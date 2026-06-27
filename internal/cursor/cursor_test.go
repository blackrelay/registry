package cursor

import (
	"testing"
	"time"
)

func TestEncodeDecodeKeysetCursor(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	encoded, err := Encode(Keyset{Time: now, ID: "event:abc:1"})
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if !decoded.Time.Equal(now) || decoded.ID != "event:abc:1" {
		t.Fatalf("decoded cursor mismatch: %#v", decoded)
	}
}

func TestDecodeRejectsInvalidCursor(t *testing.T) {
	if _, err := Decode("not base64"); err == nil {
		t.Fatal("Decode accepted an invalid cursor")
	}
}
