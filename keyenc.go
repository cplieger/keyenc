package keyenc

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
)

// MaxComponentBytes is the total raw size of a component set above which
// [Join] returns a fixed-size hashed identity instead of an escaped join.
//
// The threshold is measured on the RAW components, not on the escaped output,
// so a set's representation never depends on how many separators escaping had
// to double. A delimiter-heavy but honest set whose escaped form is twice this
// bound still encodes as an escaped join; only genuinely oversized input
// crosses over. That keeps the shape of an honest key independent of its
// content, which matters wherever a key is persisted or compared across runs.
const MaxComponentBytes = 8 << 10

// Separator is the character [Join] places between components and [Split]
// reads as a component boundary. It is exported so a caller can assert its own
// field alphabet against the grammar, not so it can be changed.
const Separator = ':'

// Escape is the character [Join] prefixes to a literal [Separator] or [Escape]
// inside a component.
const Escape = '\\'

// hashedPrefix marks a hashed component identity. The raw encodings exclude it
// (see [Join]) so the two output domains cannot collide: without the rule, a
// small component set literally spelling "sha256:<hex>" would alias the hashed
// identity of a different, oversized set. The prefix contains a [Separator] on
// purpose - "sha256" plus a hex digest is a shape an ordinary two-component
// join can produce, so the domains must be separated by construction rather
// than by hoping no caller joins that pair.
const hashedPrefix = "sha256:"

// hashedHexLen is the digest length of the hashed identity, in hex characters.
const hashedHexLen = sha256.Size * 2

var (
	// ErrHashed reports that a key is a hashed identity and therefore carries
	// no recoverable components. Hashing is one-way: the components are gone,
	// not merely encoded. A site that both builds oversized keys and parses
	// them back has to keep the fields it needs somewhere else.
	ErrHashed = errors.New("keyenc: key is a hashed identity and cannot be split")

	// ErrMalformed reports that a key was not produced by [Join]: it ends in a
	// dangling escape, or an escape precedes a character that never needs
	// escaping. Split's accepted language is exactly Join's image, so a
	// tampered or hand-built key is refused rather than silently normalized -
	// which matters wherever a key arrives from a persisted file, a URL
	// segment or client storage.
	ErrMalformed = errors.New("keyenc: key was not produced by Join")
)

// partEscaper escapes the two reserved characters. Escape is listed first so a
// component's literal backslashes are doubled before any separator escape
// introduces new ones, which is what keeps the mapping injective.
var partEscaper = strings.NewReplacer(
	string(Escape), string(Escape)+string(Escape),
	string(Separator), string(Escape)+string(Separator),
)

// Join encodes parts as one key such that no part's content can forge a
// different split: each part is escaped before the separators are inserted, so
// Join("a:b", "c") and Join("a", "b:c") differ, where a naive concatenation
// makes them identical. A part containing neither reserved character is
// emitted verbatim, so a key built from separator-free fields is
// byte-identical to its naive concatenation.
//
// Above [MaxComponentBytes] of total raw input it returns the set's hashed
// identity instead (see [MaxComponentBytes] and [IsHashed]). An in-bound set
// whose escaped join would itself begin with the hashed-identity prefix is
// also routed through the hash, which is what keeps the raw and hashed output
// domains disjoint and therefore keeps Join injective across the size
// boundary.
//
// Two degenerate cases are worth naming because they are the only places the
// output is not simply an escaped join. No parts at all encodes as the empty
// string. A single empty part is routed through the hash, because escaping
// preserves element boundaries only once the join emits a byte: without the
// special case, Join() and Join("") would share the empty encoding and alias
// two distinct component sets.
func Join(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 && parts[0] == "" {
		return hashParts(parts)
	}
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	if total <= MaxComponentBytes {
		if joined := escapeJoin(parts); !strings.HasPrefix(joined, hashedPrefix) {
			return joined
		}
	}
	return hashParts(parts)
}

// Split recovers the exact components [Join] encoded. It is Join's inverse for
// every in-bound key: Split(Join(parts...)) equals parts, and Join(Split(key))
// equals key.
//
// Its accepted language is exactly Join's image. A key carrying an escape
// before anything other than a reserved character, or a trailing dangling
// escape, is refused with [ErrMalformed] rather than normalized, so "Split
// accepted it" is a statement about the key's provenance. A hashed identity is
// refused with [ErrHashed], whose components are not recoverable. The empty key
// splits to no components, mirroring Join's zero-part case.
//
// Split exists so a site that parses its keys back has one grammar rather than
// an encoder plus a separately-maintained parser that can disagree with it.
func Split(key string) ([]string, error) {
	if key == "" {
		return nil, nil
	}
	if strings.HasPrefix(key, hashedPrefix) {
		return nil, ErrHashed
	}
	parts := make([]string, 0, strings.Count(key, string(Separator))+1)
	var cur strings.Builder
	escaped := false
	for i := range len(key) {
		c := key[i]
		switch {
		case escaped:
			if c != Escape && c != Separator {
				return nil, ErrMalformed
			}
			cur.WriteByte(c)
			escaped = false
		case c == Escape:
			escaped = true
		case c == Separator:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if escaped {
		return nil, ErrMalformed
	}
	return append(parts, cur.String()), nil
}

// IsHashed reports whether key is a hashed identity, i.e. whether [Join]
// reduced an oversized component set rather than encoding it. Call it before
// [Split] at a site that must parse its keys back; a key is otherwise
// indistinguishable from a raw one only to a caller that does not look.
func IsHashed(key string) bool {
	rest, ok := strings.CutPrefix(key, hashedPrefix)
	if !ok || len(rest) != hashedHexLen {
		return false
	}
	_, err := hex.DecodeString(rest)
	return err == nil
}

// escapeJoin escapes each part before joining, so element boundaries survive
// in the encoding: a part containing a separator is escaped while the joining
// separators stay raw. Parts free of both reserved characters stay
// byte-identical to their naive concatenation.
func escapeJoin(parts []string) string {
	escaped := make([]string, len(parts))
	for i, p := range parts {
		escaped[i] = partEscaper.Replace(p)
	}
	return strings.Join(escaped, string(Separator))
}

// hashParts streams each component into SHA-256 under a length prefix and
// returns the fixed-size "sha256:<hex>" identity. The length prefix is what
// preserves element boundaries through hashing - ["a", "b"] and ["ab"] hash
// differently - so the hashed regime is as injective as the escaped one. The
// components are never joined into a single allocation, which is the point of
// having a bound at all.
func hashParts(parts []string) string {
	h := sha256.New()
	var lenBuf [8]byte
	for _, p := range parts {
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(p)))
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write([]byte(p))
	}
	return hashedPrefix + hex.EncodeToString(h.Sum(nil))
}
