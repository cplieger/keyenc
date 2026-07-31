// Package keyenc joins several untrusted strings into one key that no
// component's content can forge.
//
// A composite key is normally built by concatenating fields with a separator:
//
//	key := userID + ":" + ratingKey + ":" + streamID
//
// That is correct only while no field can contain the separator. When one can,
// two distinct field tuples produce one key, and every consumer of that key
// silently merges two things it was built to keep apart. The failure has no
// error path: a cache returns the wrong entry, a dedupe token suppresses a
// distinct event as already seen, a singleflight gate hands one caller another
// caller's result, a map treats two identities as one.
//
// Whether a given site is exposed depends on the alphabet of every field
// except the last, which is a property of the whole program rather than of the
// key. Adding a field, reordering two, or widening one field's source from an
// enum to free-form upstream text can each turn a correct key into a forgeable
// one, and none of those edits looks like it touches key encoding.
//
// # The grammar
//
// [Join] escapes each component before joining, so the separator that appears
// between components is always distinguishable from a separator inside one:
//
//	Join("a:b", "c") == `a\:b:c`
//	Join("a", "b:c") == `a:b\:c`
//	Join("a", "b", "c") == "a:b:c"
//
// Two characters are reserved: ':' separates components and '\' escapes. Both
// are escaped element-wise, '\' first, which keeps the mapping injective. A
// component containing neither is emitted verbatim, so a key whose fields are
// already separator-free encodes byte-identically to its naive concatenation.
// That property is what lets an existing key move to this package without
// changing its bytes: the encoding changes only where the naive form was
// already ambiguous.
//
// [Split] is the exact inverse below the size bound, so a key that must be
// parsed back into its fields round-trips instead of needing a second,
// separately-maintained parser.
//
// # Nesting
//
// A key with an inner list nests by composition rather than by a second
// separator. Encode the inner sequence, then pass its result as one outer
// component:
//
//	inner := keyenc.Join(languages...)
//	key := keyenc.Join(mediaType, mediaID, inner, videoPath)
//
// The inner result is escaped as it becomes an outer component, so its
// separators cannot be read as outer ones at any depth. A second reserved
// character would buy the same thing for exactly two levels and fail at three.
//
// # Bounding
//
// Escaping grows a key in proportion to its input, so a caller that folds
// unbounded upstream data into a key can be made to allocate without limit
// (CWE-400). When the components' total raw size exceeds [MaxComponentBytes],
// [Join] returns a fixed-size SHA-256 identity instead. Components are
// streamed into the hash under a length prefix, so element boundaries survive
// hashing exactly as escaping preserves them below the bound: ["a:b"] and
// ["a", "b"] hash differently.
//
// The hashed identity carries a "sha256:" prefix and the raw encodings exclude
// that prefix, so the two output domains stay disjoint and no in-bound key can
// spell an oversized one's identity. Hashing is one-way, so [Split] refuses a
// hashed key rather than guessing; call [IsHashed] first at a site that must
// parse its keys back.
//
// Standard library only, zero dependencies. A TypeScript twin publishes as
// @cplieger/keyenc and produces byte-identical keys, pinned by the conformance
// fixture in conformance/.
package keyenc
