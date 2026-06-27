package sui

import "testing"

func TestEventKindForMoveTypeNormalisesVersionedEventNames(t *testing.T) {
	parts := moveTypeParts{
		PackageID: "0x8b8a46ed766fa1358ce7c5c51f6a164b13d627a63e45343f69ed0ba0446c1aa1",
		Module:    "inventory",
		TypeName:  "ItemDepositedEventV2",
	}
	if got := eventKindForMoveType(parts); got != "inventory.item.deposited.v2" {
		t.Fatalf("unexpected event kind %q", got)
	}
}
