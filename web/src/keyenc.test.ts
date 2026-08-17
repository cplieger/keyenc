import { readFileSync } from "node:fs";
import { join as pathJoin } from "node:path";
import { describe, expect, it } from "vitest";
import fc from "fast-check";
import {
  ESCAPE,
  HashedKeyError,
  isHashed,
  join,
  MalformedKeyError,
  MAX_COMPONENT_BYTES,
  SEPARATOR,
  split,
} from "./keyenc.js";
import { Sha256 } from "./sha256.js";

interface ConformanceCase {
  name: string;
  parts: string[];
  key: string;
}

// The fixture is a REPO-ROOT sibling of web/, which is why stryker.config.json
// sets `inPlace: true`. Stryker otherwise copies web/ into
// .stryker-tmp/sandbox-*/ where this path does not resolve, and the throw lands
// at module load — so the whole file contributes zero tests SILENTLY and Stryker
// scores what is left (4 of 49 tests, 42% instead of 86%). Read it from
// anywhere outside web/ and that config option is load-bearing; do not relocate
// or duplicate the fixture to avoid it, both halves share it on purpose.
const fixture = JSON.parse(
  readFileSync(pathJoin(import.meta.dirname, "../../conformance/keys.json"), "utf8"),
) as { note: string; cases: ConformanceCase[] };

describe("cross-language conformance", () => {
  // This is the only mechanism keeping the two implementations byte-identical.
  // A failure here is either a deliberate grammar change (regenerate with
  // `UPDATE_GOLDEN=1 go test ./... -run TestConformanceFixture` and land both
  // halves in one commit) or a real divergence between Go and TypeScript.
  it.each(fixture.cases)("matches Go for $name", ({ parts, key }) => {
    expect(join(...parts)).toBe(key);
  });

  it("covers both the escaped and hashed regimes", () => {
    const hashed = fixture.cases.filter((c) => isHashed(c.key)).length;
    expect(hashed).toBeGreaterThan(0);
    expect(fixture.cases.length - hashed).toBeGreaterThan(0);
  });

  it("round-trips every non-hashed fixture case", () => {
    for (const c of fixture.cases) {
      if (isHashed(c.key)) {
        continue;
      }
      expect(split(c.key)).toEqual(c.parts);
    }
  });
});

describe("sha256", () => {
  // The digest is hand-rolled because the call sites are synchronous, so it
  // gets its own known-answer tests on top of the conformance fixture: an
  // implementation that is wrong only for multi-block or unaligned input would
  // otherwise ride on whichever lengths the fixture happens to contain.
  it.each([
    ["", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"],
    ["abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"],
  ])("matches the known answer for %o", (input, want) => {
    const got = new Sha256().update(new TextEncoder().encode(input)).hex();
    expect(got).toBe(want);
  });

  it("agrees with crypto.subtle across block boundaries", async () => {
    // The async WebCrypto digest is the oracle the synchronous one has to
    // match; the interesting lengths are around 55/56/64/119/120 bytes, where
    // the padding branch changes.
    for (const n of [0, 1, 54, 55, 56, 57, 63, 64, 65, 119, 120, 121, 200, 1000]) {
      const bytes = new Uint8Array(n).map((_, i) => (i * 31 + 7) & 0xff);
      const want = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))]
        .map((b) => b.toString(16).padStart(2, "0"))
        .join("");
      expect(new Sha256().update(bytes).hex(), `length ${n}`).toBe(want);
    }
  });

  it("agrees with crypto.subtle when fed in arbitrary chunks", async () => {
    const bytes = new Uint8Array(300).map((_, i) => (i * 17 + 3) & 0xff);
    const want = [...new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))]
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
    for (const chunk of [1, 7, 32, 64, 100]) {
      const h = new Sha256();
      for (let i = 0; i < bytes.length; i += chunk) {
        h.update(bytes.subarray(i, i + chunk));
      }
      expect(h.hex(), `chunk ${chunk}`).toBe(want);
    }
  });
});

describe("join", () => {
  it("distinguishes a shifted boundary", () => {
    expect(join("a:b", "c")).not.toBe(join("a", "b:c"));
    expect(join("a", "b", "c")).not.toBe(join("a:b", "c"));
  });

  it("encodes separator-free components verbatim", () => {
    // The property that lets an existing key adopt keyenc without re-keying.
    expect(join("github", "github.com")).toBe("github:github.com");
    expect(join("streams", "u-42", "1234", "3", "5")).toBe("streams:u-42:1234:3:5");
  });

  it("keeps no components distinct from one empty component", () => {
    expect(join()).toBe("");
    expect(join("")).not.toBe("");
    expect(isHashed(join(""))).toBe(true);
  });

  it("hashes above the byte bound and measures UTF-8 bytes", () => {
    expect(isHashed(join("x".repeat(MAX_COMPONENT_BYTES)))).toBe(false);
    expect(isHashed(join("x".repeat(MAX_COMPONENT_BYTES + 1)))).toBe(true);
    // Two bytes per character: half the bound in characters is the whole bound
    // in bytes, so one character more must cross it. An implementation
    // measuring String.length stays raw here and diverges from Go.
    expect(isHashed(join("é".repeat(MAX_COMPONENT_BYTES / 2 + 1)))).toBe(true);
  });

  it("keeps the raw and hashed domains disjoint", () => {
    const digest = join("x".repeat(MAX_COMPONENT_BYTES + 1));
    expect(join(digest)).not.toBe(digest);
    expect(join("sha256", digest.slice("sha256:".length))).not.toBe(digest);
  });

  it("nests by composition at depth", () => {
    const inner = join("in:a", "in:b");
    const key = join("outer", inner);
    const outer = split(key);
    expect(outer).toEqual(["outer", inner]);
    expect(split(outer[1]!)).toEqual(["in:a", "in:b"]);
  });
});

describe("split", () => {
  it("refuses a hashed identity", () => {
    expect(() => split(join("x".repeat(MAX_COMPONENT_BYTES + 1)))).toThrow(HashedKeyError);
  });

  it("refuses a dangling escape", () => {
    expect(() => split("a" + ESCAPE)).toThrow(MalformedKeyError);
  });

  it("refuses an escape before an unreserved character", () => {
    // A lenient decoder would normalize this to "ab" and accept a key join can
    // never emit, so a tampered key would round-trip as if it were canonical.
    expect(() => split("a" + ESCAPE + "b")).toThrow(MalformedKeyError);
  });

  it("splits the empty key to no components", () => {
    expect(split("")).toEqual([]);
  });
});

describe("properties", () => {
  // fast-check 4 replaced stringOf with string({ unit }); the unit generator
  // is what keeps the alphabet dominated by the two reserved characters, so the
  // budget is spent on inputs the escaping exists for.
  const component = fc.string({
    unit: fc.constantFrom("a", "b", SEPARATOR, ESCAPE, "é", "🔑"),
    maxLength: 6,
  });

  it("split inverts join", () => {
    fc.assert(
      fc.property(fc.array(component, { minLength: 2, maxLength: 6 }), (parts) => {
        const key = join(...parts);
        fc.pre(!isHashed(key));
        expect(split(key)).toEqual(parts);
      }),
    );
  });

  it("moving a boundary into a component always changes the key", () => {
    fc.assert(
      fc.property(
        fc.array(component, { minLength: 2, maxLength: 5 }),
        fc.nat(),
        (parts, rawIndex) => {
          const i = rawIndex % (parts.length - 1);
          const merged = [...parts];
          merged.splice(i, 2, parts[i]! + SEPARATOR + parts[i + 1]!);
          expect(join(...merged)).not.toBe(join(...parts));
        },
      ),
    );
  });

  it("accepts only keys it could have produced", () => {
    fc.assert(
      fc.property(fc.string({ maxLength: 24 }), (key) => {
        let parts: string[];
        try {
          parts = split(key);
        } catch (e) {
          expect(e === null || e instanceof HashedKeyError || e instanceof MalformedKeyError).toBe(
            true,
          );
          return;
        }
        expect(join(...parts)).toBe(key);
      }),
    );
  });
});
