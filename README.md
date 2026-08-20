# keyenc

[![Go Reference](https://pkg.go.dev/badge/github.com/cplieger/keyenc.svg)](https://pkg.go.dev/github.com/cplieger/keyenc)
[![npm](https://img.shields.io/npm/v/@cplieger/keyenc)](https://www.npmjs.com/package/@cplieger/keyenc)
[![JSR](https://jsr.io/badges/@cplieger/keyenc)](https://jsr.io/@cplieger/keyenc)
[![Test coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/cplieger/keyenc/badges/coverage.json)](https://github.com/cplieger/keyenc/actions/workflows/coverage.yml)
[![Mutation](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/cplieger/keyenc/badges/mutation.json)](https://github.com/cplieger/keyenc/issues?q=label%3Agremlins-tracker)
[![Mutation (TS)](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/cplieger/keyenc/badges/mutation-ts.json)](https://github.com/cplieger/keyenc/issues?q=label%3Astryker-tracker)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/13903/badge)](https://www.bestpractices.dev/projects/13903)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/cplieger/keyenc/badge)](https://scorecard.dev/viewer/?uri=github.com/cplieger/keyenc)

> Join several untrusted strings into one key that no field's content can forge. Go and TypeScript, byte-identical.

Programs build composite keys constantly: a cache key from a path and a version, a dedupe token from a user and a stream, a map key from a kind and a host. The usual way is to concatenate with a separator, and that is correct only while no field can contain the separator.

```go
key := kind + ":" + host          // "gitea" + "git.example.com:3000"
```

When a field can contain it, two different field tuples produce the same key, and every consumer of that key silently merges two things it was built to keep apart. There is no error path: the cache returns the wrong entry, the dedupe token suppresses a distinct event as already seen, the map treats two identities as one. `keyenc` makes the separator between fields always distinguishable from a separator inside one, so distinct tuples always produce distinct keys.

```go
keyenc.Join("gitea", "git.example.com:3000")   // gitea:git.example.com\:3000
keyenc.Join("a:b", "c") == keyenc.Join("a", "b:c")   // false
```

Standard library only, zero dependencies, on both sides.

## Why not just pick a separator no field can contain

That is the usual answer, and it is a bet on the whole program rather than on the key. Whether a given site is safe depends on the alphabet of every field except the last, which is not visible at the call site and not stable over time. Adding a field, reordering two, or widening one field's source from an enum to free-form upstream text each turns a correct key into a forgeable one, and none of those edits looks like it touches key encoding. `\x00` narrows the bet without closing it, and it costs you a key you can put in a URL or read in a log.

## Install

- Go: `go get github.com/cplieger/keyenc@latest`
- TS: `npx jsr add @cplieger/keyenc` or `npm i @cplieger/keyenc`

## Usage

### Building a key

```go
key := keyenc.Join("streams", userID, ratingKey, audioID, subID)
```

```ts
const key = join("streams", userID, ratingKey, audioID, subID);
```

### Reading one back

Some keys are parsed again: a URL path segment, a value read out of a persisted file. `Split` is `Join`'s exact inverse, so the site needs one grammar rather than an encoder plus a parser that can disagree with it.

```go
parts, err := keyenc.Split(key)
if err != nil {
    return fmt.Errorf("unrecognized key %q: %w", key, err)
}
kind, host := parts[0], parts[1]
```

`Split` accepts exactly what `Join` emits. A key that could not have been produced by `Join` is refused rather than normalized, so "`Split` accepted it" is a statement about where the key came from.

### Nesting

A key with an inner list nests by composition. Encode the inner sequence, then pass its result as one outer component:

```go
languages := keyenc.Join("en", "fr")
key := keyenc.Join(mediaType, mediaID, languages, videoPath)
```

The inner result is escaped as it becomes an outer component, so its separators cannot be read as outer ones at any depth. A second reserved character buys the same thing for exactly two levels and fails at three.

### Oversized keys

A caller that folds unbounded upstream data into a key can be made to allocate without limit. Above `MaxComponentBytes` (8 KiB of total raw input) `Join` returns a fixed-size SHA-256 identity instead. Components are streamed into the hash under a length prefix, so element boundaries survive hashing exactly as escaping preserves them below the bound.

Hashing is one-way. At a site that parses its keys back, check first:

```go
if keyenc.IsHashed(key) {
    return errNotRecoverable
}
```

## Adopting this in an existing key

A component containing neither reserved character is emitted verbatim, so a key whose fields are already separator-free encodes **byte-identically** to its naive concatenation:

```go
keyenc.Join("streams", "u-42", "1234", "3", "5")   // streams:u-42:1234:3:5
```

The bytes change only where the naive form was already ambiguous. So adopting `keyenc` at a key that is currently safe costs nothing: no re-key, no cache invalidation, and no migration for a persisted key. At a key that is currently forgeable the bytes do change for the inputs that were colliding, which is the point.

## API

| Symbol | Purpose |
| --- | --- |
| `Join(parts ...string) string` | Encode components as one key. Hashes above `MaxComponentBytes`. |
| `Split(key string) ([]string, error)` | Recover the exact components. `Join`'s inverse. |
| `IsHashed(key string) bool` | Whether the key is a hashed identity, whose components are gone. |
| `MaxComponentBytes` | Total raw size, in bytes, above which `Join` hashes. |
| `Separator`, `Escape` | The two reserved characters, `:` and `\`. |
| `ErrHashed`, `ErrMalformed` | Why `Split` refused. |

The TypeScript half exports the same surface as `join`, `split`, `isHashed`, `MAX_COMPONENT_BYTES`, `SEPARATOR`, `ESCAPE`, `HashedKeyError` and `MalformedKeyError`.

## Cross-language parity

The two implementations produce identical keys for identical input, which matters wherever a key built in one language is looked up against keys built in the other. The parity is pinned by a shared golden fixture rather than by inspection: `conformance/keys.json` is generated from the Go implementation and asserted by the TypeScript test suite.

Two details carry that parity and are easy to get wrong in a reimplementation. The size bound is measured in **UTF-8 bytes**, not UTF-16 code units, so a key with multibyte text crosses the threshold at the same point in both languages. And the digest hashes UTF-8 bytes with an 8-byte big-endian length prefix per component. The fixture contains a case that fails specifically when either is wrong.

To change the grammar, regenerate and land both halves in one commit:

```sh
UPDATE_GOLDEN=1 go test ./... -run TestConformanceFixture
cd web && npm test
```

## Unsupported by Design

- **No configurable separator.** Two implementations that must agree byte-for-byte cannot each carry a policy knob, and a key encoded under one separator is unreadable under another. Nesting covers what a second separator would have.
- **No key derivation, no MAC, no authentication.** The digest bounds a key's size. It is not a commitment, not constant-time, and not a substitute for HMAC. A key from this package tells you two field tuples differ; it tells you nothing about who produced them.
- **No canonicalization of components.** `Join` encodes what it is given. Case folding, Unicode normalization and trimming are the caller's decisions, and applying them here would make two implementations of "normalize" the thing that has to stay in sync.
- **Arbitrary bytes are Go-only.** A Go string is a byte sequence and a JavaScript string is a UTF-16 sequence, so the two halves agree for all valid UTF-8 and the conformance fixture covers only that. Go's own fuzz targets cover invalid UTF-8 for the Go implementation.

## Disclaimer

This project is built with care and follows security best practices, but it is intended for personal / self-hosted use. No guarantees of fitness for production environments. Use at your own risk.

This project was built with AI-assisted tooling using [Claude](https://claude.com), [GPT](https://openai.com), and [Kiro](https://kiro.dev). The human maintainer defines architecture, supervises implementation, and makes all final decisions.

## License

Apache-2.0. See [LICENSE](LICENSE).
