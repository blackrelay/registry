package staticclient

import (
	"context"
	"encoding/binary"
	"testing"
)

func TestDecodeFSDBinaryMaterialRequirementRowsReadsDictionaryRows(t *testing.T) {
	data := make([]byte, 160)
	writeFSDDictionaryHeader(data, 1, []uint64{48})
	binary.LittleEndian.PutUint64(data[72:80], 1)
	binary.LittleEndian.PutUint64(data[80:88], 87434)
	binary.LittleEndian.PutUint64(data[88:96], 72)
	binary.LittleEndian.PutUint64(data[96:104], 2)
	binary.LittleEndian.PutUint32(data[104:108], 77801)
	binary.LittleEndian.PutUint32(data[108:112], 82)
	binary.LittleEndian.PutUint32(data[112:116], 77802)
	binary.LittleEndian.PutUint32(data[116:120], 7)

	rows := DecodeFSDBinaryMaterialRequirementRows(data, staticTypeNamesFixture())

	if len(rows) != 1 {
		t.Fatalf("expected one material requirement row, got %#v", rows)
	}
	row := rows[0]
	if row.TypeID != 87434 || row.Name != "Desiccated Flesh" || row.OffsetBytes != 96 {
		t.Fatalf("unexpected material row metadata: %#v", row)
	}
	if len(row.Materials) != 2 || row.Materials[0].TypeID != 77801 || row.Materials[0].Quantity != 82 || row.Materials[1].TypeID != 77802 || row.Materials[1].Quantity != 7 {
		t.Fatalf("material pairs were not decoded deterministically: %#v", row.Materials)
	}
}

func TestDecodeFSDBinaryMaterialRequirementRowsRejectsUnvalidatedRows(t *testing.T) {
	data := make([]byte, 144)
	writeFSDDictionaryHeader(data, 1, []uint64{48})
	binary.LittleEndian.PutUint64(data[72:80], 1)
	binary.LittleEndian.PutUint64(data[80:88], 87434)
	binary.LittleEndian.PutUint64(data[88:96], 72)
	binary.LittleEndian.PutUint64(data[96:104], 2)
	binary.LittleEndian.PutUint32(data[104:108], 77801)
	binary.LittleEndian.PutUint32(data[108:112], 82)
	binary.LittleEndian.PutUint32(data[112:116], 42)
	binary.LittleEndian.PutUint32(data[116:120], 7)

	rows := DecodeFSDBinaryMaterialRequirementRows(data, staticTypeNamesFixture())

	if len(rows) != 0 {
		t.Fatalf("unknown material type ids should reject the row: %#v", rows)
	}
}

func TestDecodeFSDBinaryBlueprintRowsBuildsCandidateRecipes(t *testing.T) {
	data := make([]byte, 192)
	writeFSDDictionaryHeader(data, 1, []uint64{48})
	binary.LittleEndian.PutUint64(data[72:80], 1)
	binary.LittleEndian.PutUint64(data[80:88], 1056)
	binary.LittleEndian.PutUint64(data[88:96], 80)

	rowOffset := 104
	binary.LittleEndian.PutUint32(data[rowOffset:rowOffset+4], 0)
	binary.LittleEndian.PutUint32(data[rowOffset+4:rowOffset+8], 82654)
	binary.LittleEndian.PutUint32(data[rowOffset+8:rowOffset+12], 207)
	binary.LittleEndian.PutUint32(data[rowOffset+12:rowOffset+16], 8)
	binary.LittleEndian.PutUint32(data[rowOffset+16:rowOffset+20], 0)
	binary.LittleEndian.PutUint32(data[rowOffset+20:rowOffset+24], 3)
	binary.LittleEndian.PutUint32(data[rowOffset+24:rowOffset+28], 78516)
	binary.LittleEndian.PutUint32(data[rowOffset+28:rowOffset+32], 60)
	binary.LittleEndian.PutUint32(data[rowOffset+32:rowOffset+36], 83899)
	binary.LittleEndian.PutUint32(data[rowOffset+36:rowOffset+40], 1)
	binary.LittleEndian.PutUint32(data[rowOffset+40:rowOffset+44], 82654)
	binary.LittleEndian.PutUint32(data[rowOffset+44:rowOffset+48], 1)

	rows := DecodeFSDBinaryBlueprintRows(context.Background(), data, staticTypeNamesFixture())

	if len(rows) != 1 {
		t.Fatalf("expected one blueprint row, got %#v", rows)
	}
	row := rows[0]
	if row.BlueprintID != 1056 || row.PrimaryTypeID != 82654 || row.PrimaryName != "Reinforced Shield Generator II" || row.RunTimeSeconds != 207 {
		t.Fatalf("unexpected blueprint row metadata: %#v", row)
	}
	if len(row.Outputs) != 1 || row.Outputs[0].TypeID != 82654 || row.Outputs[0].Quantity != 1 {
		t.Fatalf("primary output was not decoded: %#v", row.Outputs)
	}
	if len(row.Inputs) != 2 || row.Inputs[0].TypeID != 78516 || row.Inputs[0].Quantity != 60 || row.Inputs[1].TypeID != 83899 || row.Inputs[1].Quantity != 1 {
		t.Fatalf("input candidates were not decoded: %#v", row.Inputs)
	}
}

func TestDecodeFSDBinaryBlueprintRowsRejectsRowsWithoutTrustedPrimaryType(t *testing.T) {
	data := make([]byte, 160)
	writeFSDDictionaryHeader(data, 1, []uint64{48})
	binary.LittleEndian.PutUint64(data[72:80], 1)
	binary.LittleEndian.PutUint64(data[80:88], 1056)
	binary.LittleEndian.PutUint64(data[88:96], 80)

	rowOffset := 104
	binary.LittleEndian.PutUint32(data[rowOffset:rowOffset+4], 0)
	binary.LittleEndian.PutUint32(data[rowOffset+4:rowOffset+8], 999999)
	binary.LittleEndian.PutUint32(data[rowOffset+8:rowOffset+12], 207)
	binary.LittleEndian.PutUint32(data[rowOffset+12:rowOffset+16], 8)

	rows := DecodeFSDBinaryBlueprintRows(context.Background(), data, staticTypeNamesFixture())

	if len(rows) != 0 {
		t.Fatalf("unknown primary type id should reject blueprint candidates: %#v", rows)
	}
}

func writeFSDDictionaryHeader(data []byte, rowCount uint64, bucketOffsets []uint64) {
	binary.LittleEndian.PutUint64(data[24:32], uint64(len(data)-32))
	binary.LittleEndian.PutUint64(data[32:40], 24)
	binary.LittleEndian.PutUint64(data[40:48], rowCount)
	binary.LittleEndian.PutUint64(data[48:56], uint64(len(bucketOffsets)))
	for index, offset := range bucketOffsets {
		binary.LittleEndian.PutUint64(data[56+index*8:64+index*8], offset)
	}
}

func staticTypeNamesFixture() map[int]string {
	return map[int]string{
		77801: "Nickel-Iron Veins",
		77802: "Light Metals",
		78516: "EU-40 Fuel",
		82654: "Reinforced Shield Generator II",
		83899: "Catalytic Dust",
		87434: "Desiccated Flesh",
	}
}
