# Contributing to keyenc

Thanks for your interest. `keyenc` is one grammar with two implementations that must agree byte-for-byte, so most of what follows is about keeping them in step.

## Architecture

The repository is one Go module at the root and one TypeScript package under `web/`:

- `keyenc.go`, `doc.go` — the Go implementation and the package documentation. `doc.go` holds the reasoning about the grammar; keep it there rather than duplicating it in the README.
- `web/src/keyenc.ts` — the TypeScript twin.
- `web/src/sha256.ts` — a synchronous SHA-256, needed because `crypto.subtle.digest` is async and every call site is a synchronous render, dedupe or map-key path. `hex()` is non-consuming, matching Go's `hash.Hash.Sum` contract: it folds the padding into a snapshot, so it is idempotent and the stream stays usable afterwards.
- `conformance/keys.json` — the shared golden fixture that pins the two implementations against each other.
- `web/go.mod` — a sentinel module, not a real one. It stops the root module's `./...` walk at the `web/` boundary so `go test ./...` never descends into `web/node_modules`, which vendors Go files inside npm packages.

### The grammar is a contract, not an implementation detail

Two characters are reserved: `:` separates components and `\` escapes. Three properties depend on that exact choice, and a change to any of them breaks consumers rather than merely altering output:

- **A component free of both reserved characters encodes verbatim.** This is what lets an existing key adopt `keyenc` without changing its bytes, which is in turn what lets a consumer adopt it at a persisted or cross-process key without a migration. A change that escapes anything extra invalidates every persisted key in every consumer.
- **`Split` accepts exactly `Join`'s image.** The decoder refuses an escape before an unreserved character rather than normalizing it, so "`Split` accepted this" is a statement about the key's provenance. A lenient decoder would silently accept a tampered key.
- **The raw and hashed output domains are disjoint.** `Join` routes an in-bound set through the hash when its escaped form would begin with `sha256:`. Without that rule a small component set spelling a digest aliases an oversized set's identity. Note that `sha256:` contains the separator on purpose: it is a shape an ordinary two-component join can produce, so the domains have to be separated by construction.

### The two implementations must produce identical bytes

`conformance/keys.json` is generated from the Go implementation and asserted by the TypeScript suite. It is the only mechanism keeping them aligned; nothing else would catch a divergence.

To change the grammar, regenerate the fixture and update both halves **in one commit**:

```sh
UPDATE_GOLDEN=1 go test ./... -run TestConformanceFixture
cd web && npm test
```

A stale fixture fails the Go test with the regeneration command in the message. A real divergence fails the TypeScript test naming the case.

Two details carry the parity and are the likely cause of any divergence you hit:

- **The size bound is measured in UTF-8 bytes**, not UTF-16 code units. Go measures `len(s)`; the TypeScript half counts encoded bytes with `utf8Length`. Measuring `String.length` crosses the threshold at a different point for multibyte input. The fixture has a case that fails exactly this way.
- **The digest hashes UTF-8 bytes with an 8-byte big-endian length prefix per component.** The length prefix is what preserves element boundaries through hashing, so `["a", "b"]` and `["ab"]` differ in the hashed regime as they do in the escaped one.

When adding a conformance case, add it to `conformanceInputs()` in `conformance_test.go`, regenerate, and re-run the TypeScript suite. `TestConformanceCasesCoverBothRegimes` fails if the case list ever drifts entirely into one regime.

### Intentional non-features

The README's "Unsupported by Design" section is the full list, and each entry is a decision rather than a gap: no configurable separator, no MAC or key derivation, no component canonicalization, and arbitrary (non-UTF-8) bytes supported on the Go side only. A PR adding one of these needs to argue against the reasoning there first.

## Local development

`GOTOOLCHAIN=auto` is required — the module's `go` directive is ahead of some local toolchains.

```sh
# Go half
go build ./... && go vet ./...
go test -count=1 ./...
gofmt -l .                     # must print nothing
golangci-lint run ./...        # must report 0 issues

# TypeScript half
cd web
npm ci
npm test                       # vitest, includes the conformance cases
npm run typecheck              # tsc -p tsconfig.json
npx tsc -p tsconfig.tests.json # tests are type-checked too
npx eslint .
npx prettier --check src *.json *.mjs
```

Fuzzing beyond the committed seed corpus is worth a few minutes when touching the encoder or the decoder:

```sh
go test -run=XXX -fuzz=FuzzJoinSplitInjective -fuzztime=60s
go test -run=XXX -fuzz=FuzzSplitAcceptsArbitraryInput -fuzztime=60s
```

## Conventions and gotchas

- **Both halves or neither.** A change to the encoder, the decoder or the bound lands in Go and TypeScript in the same commit, with the fixture regenerated. A single-sided change passes its own suite and breaks every consumer that compares keys across the boundary.
- **Property and fuzz coverage is the point, not an extra.** The injectivity claim is the reason the package exists, so it is pinned by properties (`pgregory.net/rapid`, `fast-check`) and fuzz targets rather than only by tables. Do not rename a fuzz target that has a committed `testdata/fuzz/` corpus; the directory is keyed by the exact name.
- **Zero dependencies at runtime, both sides.** The Go module's only requirement is a test-only `pgregory.net/rapid`; the TypeScript package has no runtime dependencies. Reaching for a hashing or escaping library defeats the point.
- **The digest is not security machinery.** It bounds a key's size. It is not constant-time and must not become an authentication primitive.
- **`web/src/sha256.ts` carries an eslint exemption** for `no-non-null-assertion`, scoped in `web/eslint.config.mjs` with the reason. It applies to that file only; do not widen it.

## Publishing model

Releases are automated through `.github/workflows/release.yaml`. A release publishes the Go module as `github.com/cplieger/keyenc` and the TypeScript package to npm and JSR as `@cplieger/keyenc`, at the same version. Don't publish manually.

## Commits and PRs

Branch from `main`, keep changes focused, and open a PR. Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (parsed by git-cliff for release notes): `feat:`, `fix:`, `sec:`, and the non-releasing `chore:`/`ci:`/`docs:`/`refactor:`/`test:` types. A grammar change that alters existing keys' bytes is breaking: use `feat!:` or a `BREAKING CHANGE:` footer, and say in the body which consumers re-key.

## Conduct and security

By participating you agree to the [Code of Conduct](https://github.com/cplieger/.github/blob/main/CODE_OF_CONDUCT.md). Report security vulnerabilities through the [security policy](https://github.com/cplieger/.github/blob/main/SECURITY.md), never in a public issue.
