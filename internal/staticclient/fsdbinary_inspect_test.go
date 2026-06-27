package staticclient

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInspectStaticClientTypesRecordsBinaryEvidenceAndProbeOffsets(t *testing.T) {
	root := t.TempDir()
	typePath := filepath.Join(filepath.Dir(root), "ResFiles", "3c", "types-file")
	locPath := filepath.Join(filepath.Dir(root), "ResFiles", "2c", "localisation-file")
	if err := os.MkdirAll(filepath.Dir(typePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(locPath), 0o755); err != nil {
		t.Fatal(err)
	}

	typeBytes := make([]byte, 128)
	copy(typeBytes[:16], []byte("0123456789abcdef"))
	binary.LittleEndian.PutUint32(typeBytes[40:44], 5033)
	binary.LittleEndian.PutUint32(typeBytes[72:76], 92096)
	binary.LittleEndian.PutUint32(typeBytes[76:80], 1035497)
	binary.LittleEndian.PutUint32(typeBytes[84:88], 81610)
	if err := os.WriteFile(typePath, typeBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(locPath, []byte("pickle-ish localisation"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte(strings.Join([]string{
		"res:/staticdata/types.fsdbinary,3c/types-file,hash,128,64",
		"res:/localizationfsd/localization_fsd_en-us.pickle,2c/localisation-file,hash,23,23",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := InspectStaticClientTypes(context.Background(), StaticTypeInspectionOptions{
		ClientRoot:   root,
		ProbeTypeIDs: []int{92096, 5130},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ParserStatus != "native_probe_row_decoder_available" {
		t.Fatalf("unexpected parser status: %s", result.ParserStatus)
	}
	if result.TypeResource.ResourcePath != "res:/staticdata/types.fsdbinary" || result.TypeResource.SHA256 == "" {
		t.Fatalf("type resource evidence was not recorded: %#v", result.TypeResource)
	}
	if result.LocalizationResource == nil || result.LocalizationResource.ResourcePath != "res:/localizationfsd/localization_fsd_en-us.pickle" {
		t.Fatalf("localisation resource evidence was not recorded: %#v", result.LocalizationResource)
	}
	if result.TypeFile.HeaderHex != "3031323334353637383961626364656600000000000000000000000000000000" {
		t.Fatalf("unexpected header hex: %s", result.TypeFile.HeaderHex)
	}
	if len(result.Probes) != 2 {
		t.Fatalf("expected two probes, got %#v", result.Probes)
	}
	if result.Probes[0].TypeID != 5130 || len(result.Probes[0].LittleEndianOffsets) != 0 {
		t.Fatalf("unexpected 5130 probe: %#v", result.Probes[0])
	}
	if result.Probes[1].TypeID != 92096 || len(result.Probes[1].LittleEndianOffsets) != 1 || result.Probes[1].LittleEndianOffsets[0] != 72 {
		t.Fatalf("unexpected 92096 probe: %#v", result.Probes[1])
	}
	if len(result.DecodedRows) != 1 {
		t.Fatalf("expected one decoded row probe, got %#v", result.DecodedRows)
	}
	row := result.DecodedRows[0]
	if row.TypeID != 92096 || row.GroupID != 5033 || row.TypeNameID != 1035497 || row.WreckTypeID != 81610 || row.OffsetBytes != 72 {
		t.Fatalf("unexpected decoded row probe: %#v", row)
	}
}

func TestDecodeLocalizationStringsMapsPickleBinaryIntegersToStrings(t *testing.T) {
	data := []byte{
		0x4a, 0xe9, 0xcc, 0x0f, 0x00,
		0x8c, 0x05, 'C', 'a', 'i', 'r', 'd',
		0x94, 0x4e, 0x4e, 0x87, 0x94,
		0x4a, 0xff, 0xee, 0x0f, 0x00,
		0x8c, 0x06, 'M', 'y', 'c', 'e', 'n', 'a',
	}

	got := DecodeLocalizationStrings(data)

	if got[1035497] != "Caird" {
		t.Fatalf("expected Caird localisation, got %#v", got)
	}
	if got[1044223] != "Mycena" {
		t.Fatalf("expected Mycena localisation, got %#v", got)
	}
}

func TestDecodeFSDBinaryTypeRowsScansLocalisationBackedRows(t *testing.T) {
	typeBytes := make([]byte, 192)
	writeNativeTypeProbeRow(typeBytes, 72, 5033, 92096, 1035497, 81610)
	writeNativeTypeProbeRow(typeBytes, 128, 5130, 94167, 1044223, 81610)
	writeNativeTypeProbeRow(typeBytes, 176, 5130, 94167, 1044223, 81610)

	rows := DecodeFSDBinaryTypeRows(context.Background(), typeBytes, map[int]string{
		1035497: "Caird",
		1044223: "Mycena",
	})

	if len(rows) != 2 {
		t.Fatalf("expected two deduplicated native rows, got %#v", rows)
	}
	if rows[0].TypeID != 92096 || rows[0].GroupID != 5033 || rows[0].Name != "Caird" || rows[0].OffsetBytes != 72 {
		t.Fatalf("unexpected first native row: %#v", rows[0])
	}
	if rows[1].TypeID != 94167 || rows[1].GroupID != 5130 || rows[1].Name != "Mycena" || rows[1].OffsetBytes != 128 {
		t.Fatalf("unexpected second native row: %#v", rows[1])
	}
}

func TestDecodeFSDBinaryTypeRowsRejectsNeighbourAndSentinelFalsePositives(t *testing.T) {
	typeBytes := make([]byte, 560)
	binary.LittleEndian.PutUint32(typeBytes[36:40], 11160)
	binary.LittleEndian.PutUint32(typeBytes[68:72], 1)
	writeNativeTypeProbeRow(typeBytes, 72, 5033, 92096, 1035497, 81610)
	writeNativeTypeProbeRow(typeBytes, 160, 27, 28, 524288, 1048576)
	writeNativeTypeProbeRow(typeBytes, 240, 10, 256, 573295, 0)
	writeNativeTypeProbeRow(typeBytes, 320, 1928, 86517, 888888, 0)
	writeNativeTypeProbeRow(typeBytes, 400, 1, 1016413, 1000424, 0)
	writeNativeTypeProbeRow(typeBytes, 480, 4830, 95457, 1047430, 0)

	rows := DecodeFSDBinaryTypeRows(context.Background(), typeBytes, map[int]string{
		92096:   "False neighbour name",
		1035497: "Caird",
		524288:  "Tormentor Ironblood Serenity Only SKIN (30 Days)",
		573295:  "'Saikadori' Facial Augmentation Crate",
		888888:  "20002303",
		1000424: "30120629",
		1047430: "Char_OmoPromisedLands",
	})

	if len(rows) != 2 {
		t.Fatalf("expected only validated native rows, got %#v", rows)
	}
	byTypeID := make(map[int]FSDBinaryDecodedRow, len(rows))
	for _, row := range rows {
		byTypeID[row.TypeID] = row
	}
	if byTypeID[92096].Name != "Caird" {
		t.Fatalf("known-good row was not retained: %#v", rows)
	}
	if byTypeID[95457].Name != "Char_OmoPromisedLands" {
		t.Fatalf("plausible new row was not retained: %#v", rows)
	}
}

func TestDecodeStaticClientTypeFileWritesStableNativeArtefact(t *testing.T) {
	root := t.TempDir()
	typePath := filepath.Join(filepath.Dir(root), "ResFiles", "3c", "types-file")
	locPath := filepath.Join(filepath.Dir(root), "ResFiles", "2c", "localisation-file")
	if err := os.MkdirAll(filepath.Dir(typePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(locPath), 0o755); err != nil {
		t.Fatal(err)
	}
	typeBytes := make([]byte, 192)
	writeNativeTypeProbeRow(typeBytes, 72, 5033, 92096, 1035497, 81610)
	writeNativeTypeProbeRow(typeBytes, 128, 5130, 94167, 1044223, 81610)
	if err := os.WriteFile(typePath, typeBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	localisation := []byte{
		0x4a, 0xe9, 0xcc, 0x0f, 0x00,
		0x8c, 0x05, 'C', 'a', 'i', 'r', 'd',
		0x4a, 0xff, 0xee, 0x0f, 0x00,
		0x8c, 0x06, 'M', 'y', 'c', 'e', 'n', 'a',
	}
	if err := os.WriteFile(locPath, localisation, 0o644); err != nil {
		t.Fatal(err)
	}
	index := strings.Join([]string{
		"res:/staticdata/types.fsdbinary,3c/types-file,hash,192,96",
		"res:/localizationfsd/localization_fsd_en-us.pickle,2c/localisation-file,hash,25,25",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "resfileindex.txt"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "decoded-types.json")

	result, err := DecodeStaticClientTypeFile(context.Background(), StaticTypeDecodeOptions{
		ClientRoot:  root,
		OutputPath:  out,
		Environment: "stillness",
		Now: func() time.Time {
			return time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != "registry.static-client-type-decode.v1" || result.RowCount != 2 || result.OutputPath != out {
		t.Fatalf("unexpected decode result: %#v", result)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"schemaVersion": "registry.static-client-type-decode.v1"`,
		`"decoderStatus": "native_localisation_backed_type_rows"`,
		`"resourcePath": "res:/staticdata/types.fsdbinary"`,
		`"typeId": 92096`,
		`"name": "Caird"`,
		`"offsetBytes": 72`,
		`"typeId": 94167`,
		`"name": "Mycena"`,
		`"offsetBytes": 128`,
	} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("decoded artefact missing %q:\n%s", expected, string(data))
		}
	}
}
