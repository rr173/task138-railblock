package idlib

import (
	"strings"
	"testing"
)

// TestNewHasPrefix checks the prefixed shape.
func TestNewHasPrefix(t *testing.T) {
	id := New("SW")
	if !strings.HasPrefix(id, "SW-") {
		t.Fatalf("id %s should have SW- prefix", id)
	}
	if len(id) <= len("SW-") {
		t.Fatalf("id %s too short", id)
	}
}

// TestNewIsUnique checks two generated ids differ.
func TestNewIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := New("SG")
		if seen[id] {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = true
	}
}

// TestPrefixedPassesThrough: an explicit id is returned unchanged.
func TestPrefixedPassesThrough(t *testing.T) {
	if got := Prefixed("SW", "custom-1"); got != "custom-1" {
		t.Fatalf("explicit id: got %s want custom-1", got)
	}
	if got := Prefixed("SW", ""); !strings.HasPrefix(got, "SW-") {
		t.Fatalf("empty id should generate prefixed, got %s", got)
	}
}

// TestSeqIDFormat checks the zero-padded shape.
func TestSeqIDFormat(t *testing.T) {
	if got := SeqID("RT", 7); got != "RT-0007" {
		t.Fatalf("SeqID(7) = %s want RT-0007", got)
	}
	if got := SeqID("RT", 1234); got != "RT-1234" {
		t.Fatalf("SeqID(1234) = %s want RT-1234", got)
	}
}
