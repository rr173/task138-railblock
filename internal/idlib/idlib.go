// Package idlib generates short, deterministic identifiers for interlocking
// entities. In production IDs are UUID-derived prefixes; in the self-check the
// caller passes explicit prefixes so assertions can name a specific switch or
// route without parsing random strings.
package idlib

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New returns a new 12-char hex identifier. It is unique enough for a single
// station's elements and short enough to read in test output.
func New(prefix string) string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read only fails if the system CSPRNG is unavailable; fall back
		// to a panic so a misconfigured host fails loudly rather than emitting
		// duplicate zero IDs.
		panic("crypto/rand unavailable: " + err.Error())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

// Prefixed returns id if it is non-empty, otherwise a fresh prefixed ID. The
// self-check uses this to pass explicit, deterministic IDs.
func Prefixed(prefix, id string) string {
	if id != "" {
		return id
	}
	return New(prefix)
}

// SeqID renders a sequence number into a stable ID like "SW-0007". Used by
// test fixtures that want predictable IDs.
func SeqID(prefix string, n int) string {
	return fmt.Sprintf("%s-%04d", prefix, n)
}
