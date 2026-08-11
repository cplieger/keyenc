# @cplieger/keyenc

> Join several untrusted strings into one key that no field's content can forge.

The TypeScript half of [cplieger/keyenc](https://github.com/cplieger/keyenc). It produces byte-identical keys to the Go half, so a key built here can be looked up against keys built there.

Programs build composite keys constantly: a cache key from a path and a version, a dedupe token from a user and a stream, a render signature from a row's fields. The usual way is a template literal, and that is correct only while no field can contain the separator.

```ts
const key = `${kind}:${host}`; // "gitea" + "git.example.com:3000"
```

When a field can contain it, two different field tuples produce the same key, and every consumer of that key silently merges two things it was built to keep apart. There is no error path: the cache returns the wrong entry, the dedupe token suppresses a distinct action as already in flight, the signature skips a repaint that was needed.

```ts
join("gitea", "git.example.com:3000"); // gitea:git.example.com\:3000
join("a:b", "c") === join("a", "b:c"); // false
```

Zero dependencies. No DOM APIs, so it runs in a browser, in Node and in Deno.

## Install

```sh
npx jsr add @cplieger/keyenc
# or
npm i @cplieger/keyenc
```

## Usage

```ts
import { isHashed, join, split } from "@cplieger/keyenc";

const key = join("streams", userID, ratingKey, audioID, subID);

// Split is join's exact inverse, for a key that gets parsed again.
const [kind, host] = split(forgeID);

// Above MAX_COMPONENT_BYTES a key becomes a fixed-size digest, which is
// one-way. Check before splitting at a site that parses its keys back.
if (isHashed(key)) return;
```

A component containing neither reserved character (`:` and `\`) is emitted verbatim, so a key whose fields are already separator-free encodes byte-identically to its template literal. Adopting this at a key that is currently safe therefore changes nothing, including for a key persisted in `localStorage`.

`split` accepts exactly what `join` emits. A key that could not have been produced by `join` throws `MalformedKeyError` rather than being normalized, and a digest throws `HashedKeyError`.

## Full documentation

The grammar, the nesting rule, the size bound, the cross-language parity contract and the API table are in the [repository README](https://github.com/cplieger/keyenc#readme).

## Disclaimer

This project is built with care and follows security best practices, but it is intended for personal / self-hosted use. No guarantees of fitness for production environments. Use at your own risk.

This project was built with AI-assisted tooling using [Claude](https://claude.com), [GPT](https://openai.com), and [Kiro](https://kiro.dev). The human maintainer defines architecture, supervises implementation, and makes all final decisions.

## License

Apache-2.0. See [LICENSE](LICENSE).
