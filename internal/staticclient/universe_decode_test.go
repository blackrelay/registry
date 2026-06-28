package staticclient

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/blackrelay/registry/internal/artefacts"
	"github.com/blackrelay/registry/internal/db"
	"github.com/blackrelay/registry/internal/model"
)

func TestDecodeStaticClientUniverseFilesWritesImportableStaticUniverse(t *testing.T) {
	root := t.TempDir()
	clientRoot := filepath.Join(root, "stillness")
	if err := os.MkdirAll(clientRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	var indexLines []string
	addResource := func(resourcePath, hashPath string, data []byte) {
		t.Helper()
		path := filepath.Join(root, "ResFiles", filepath.FromSlash(hashPath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		indexLines = append(indexLines, resourcePath+","+hashPath+",hash,0,0")
	}
	addResource("res:/staticdata/regions.static", "aa/regions", staticDict([]staticFixtureRow{
		{key: 10000012, value: staticRegionValue(10000012, 1001, 1, 2, 3)},
	}))
	addResource("res:/staticdata/constellations.static", "aa/constellations", staticDict([]staticFixtureRow{
		{key: 20000068, value: staticConstellationValue(20000068, 10000012, 2001, 4, 5, 6)},
	}))
	addResource("res:/staticdata/systems.static", "aa/systems", staticDict([]staticFixtureRow{
		{key: 30001001, value: staticSystemValue(30001001, 20000068, 10000012, 3001, 7, 8, 9)},
		{key: 30001002, value: staticSystemValue(30001002, 20000068, 10000012, 3002, 10, 11, 12)},
	}))
	addResource("res:/staticdata/jumps.static", "aa/jumps", staticJumpList([]staticJumpFixtureRow{
		{jumpID: 3535, stargateID: 60000001, fromSystemID: 30001001, toSystemID: 30001002},
	}))
	addResource("res:/localizationfsd/localization_fsd_en-us.pickle", "aa/localisation", staticLocalisation(map[int]string{
		1001: "Inner Stone Cluster",
		2001: "Inner First",
		3001: "NN0-Y-D5",
		3002: "6RG-Y-T4",
	}))
	if err := os.WriteFile(filepath.Join(clientRoot, "resfileindex.txt"), []byte(strings.Join(indexLines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(root, "decoded-universe")
	result, err := DecodeStaticClientUniverseFiles(context.Background(), StaticUniverseDecodeOptions{
		ClientRoot:  clientRoot,
		OutputDir:   outputDir,
		Environment: model.EnvironmentStillness,
		ClientBuild: "fixture-build",
		PatchLabel:  "fixture-patch",
		Now:         func() time.Time { return time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SystemCount != 2 || result.RegionCount != 1 || result.ConstellationCount != 1 || result.JumpCount != 1 {
		t.Fatalf("unexpected decode counts: %#v", result)
	}
	if len(result.Resources) != 5 {
		t.Fatalf("expected five evidence resources, got %d", len(result.Resources))
	}

	store := db.NewMemoryStore()
	importResult, err := ImportUniverse(context.Background(), store, artefacts.LocalStore{Root: filepath.Join(root, "artefacts")}, outputDir, UniverseOptions{
		Environment:     model.EnvironmentStillness,
		AllowedRootDirs: []string{root},
	})
	if err != nil {
		t.Fatal(err)
	}
	if importResult.SystemsImported != 2 || importResult.RoutesImported != 1 {
		t.Fatalf("unexpected import counts: %#v", importResult)
	}
	if got := store.Entities["system:stillness:30001001"].Name; got != "NN0-Y-D5" {
		t.Fatalf("system name was not resolved from localisation: %q", got)
	}
	if got := store.Entities["route:stillness:30001001:30001002"].Name; got != "NN0-Y-D5 to 6RG-Y-T4" {
		t.Fatalf("route name was not built from decoded system names: %q", got)
	}
}

func TestDecodeStaticDictEntriesRejectsInvalidFooter(t *testing.T) {
	if _, err := decodeStaticDictEntries([]byte{1, 2, 3, 4}); err == nil {
		t.Fatal("expected invalid footer to be rejected")
	}
}

func TestStaticUniverseRowNameUsesNativeVariableNameBeforeNumericLocalisation(t *testing.T) {
	row := staticSystemValue(30001001, 20000068, 10000012, 970062, 7, 8, 9)
	row = append(row, staticSystemVariableData("ILC-7R7")...)

	got := staticUniverseRowName(row, 56, 11, 2, map[int]string{970062: "30090267"}, 970062, "System 30001001")

	if got != "ILC-7R7" {
		t.Fatalf("expected native variable name, got %q", got)
	}
}

func TestStaticUniverseRowNameUsesNativeVariableNameWithExtraOffset(t *testing.T) {
	row := staticSystemValue(30000192, 20000014, 10000006, 969253, 7, 8, 9)
	row = append(row, staticSystemVariableDataWithOffsetCount("L.J33.TF5", 11)...)

	got := staticUniverseRowName(row, 56, 11, 2, map[int]string{969253: "30089458"}, 969253, "System 30000192")

	if got != "L.J33.TF5" {
		t.Fatalf("expected native variable name with extra offset, got %q", got)
	}
}

func TestStaticUniverseNameRejectsNumericLocalisationPlaceholder(t *testing.T) {
	got := staticUniverseName(map[int]string{970062: "30090267"}, 970062, "System 30001001")

	if got != "System 30001001" {
		t.Fatalf("expected fallback for numeric localisation placeholder, got %q", got)
	}
}

type staticFixtureRow struct {
	key   int
	value []byte
}

type staticJumpFixtureRow struct {
	jumpID       int
	stargateID   int
	fromSystemID int
	toSystemID   int
}

func staticDict(rows []staticFixtureRow) []byte {
	data := []byte{0, 0, 0, 0}
	offsets := make([]int, 0, len(rows))
	for _, row := range rows {
		offsets = append(offsets, len(data)-4)
		data = append(data, row.value...)
	}
	footerStart := len(data)
	for index, row := range rows {
		appendInt32(&data, row.key)
		appendInt32(&data, offsets[index])
	}
	appendInt32(&data, len(data)-footerStart+4)
	return data
}

func staticRegionValue(regionID, nameID int, x, y, z float64) []byte {
	value := make([]byte, 44)
	putInt32(value, 0, regionID)
	putInt32(value, 4, nameID)
	putVector3(value, 8, x, y, z)
	putFloat32(value, 32, 1)
	putInt32(value, 36, 42)
	putInt32(value, 40, 7)
	return value
}

func staticConstellationValue(constellationID, regionID, nameID int, x, y, z float64) []byte {
	value := make([]byte, 36)
	putInt32(value, 0, constellationID)
	putInt32(value, 4, regionID)
	putInt32(value, 8, nameID)
	putVector3(value, 12, x, y, z)
	return value
}

func staticSystemValue(systemID, constellationID, regionID, nameID int, x, y, z float64) []byte {
	value := make([]byte, 56)
	putInt32(value, 0, systemID)
	putFloat32(value, 4, -1)
	putFloat32(value, 8, 2)
	putFloat32(value, 12, 3)
	putInt32(value, 16, constellationID)
	putInt32(value, 20, regionID)
	putInt32(value, 24, nameID)
	putVector3(value, 28, x, y, z)
	putFloat32(value, 52, -1)
	return value
}

func staticSystemVariableData(name string) []byte {
	return staticSystemVariableDataWithOffsetCount(name, 10)
}

func staticSystemVariableDataWithOffsetCount(name string, offsetCount int) []byte {
	securityClass := staticStringSegment("D1")
	habitableZone := make([]byte, 12)
	putInt32(habitableZone, 0, 2)
	putFloat32(habitableZone, 4, 1)
	putFloat32(habitableZone, 8, 2)
	nameSegment := staticStringSegment(name)
	factionID := make([]byte, 4)
	putInt32(factionID, 0, 45031)
	sunTypeID := make([]byte, 4)
	putInt32(sunTypeID, 0, 1243)
	sunFlareGraphicID := make([]byte, 4)
	wormholeClassID := make([]byte, 4)
	visualEffect := make([]byte, 4)
	neighbours := make([]byte, 4)

	segments := [][]byte{
		securityClass,
		habitableZone,
		nameSegment,
		factionID,
		sunTypeID,
		sunFlareGraphicID,
		wormholeClassID,
		visualEffect,
		neighbours,
	}
	offsets := []int{0, 0}
	total := 0
	for _, segment := range segments {
		total += len(segment)
		offsets = append(offsets, total)
	}
	out := make([]byte, 4)
	putInt32(out, 0, 29)
	for _, offset := range offsets[:offsetCount] {
		appendInt32(&out, offset)
	}
	for _, segment := range segments {
		out = append(out, segment...)
	}
	return out
}

func staticStringSegment(value string) []byte {
	data := make([]byte, 4)
	putInt32(data, 0, len(value))
	data = append(data, []byte(value)...)
	return data
}

func staticJumpList(rows []staticJumpFixtureRow) []byte {
	data := make([]byte, 4)
	putInt32(data, 0, len(rows))
	for _, row := range rows {
		record := make([]byte, 65)
		putInt32(record, 0, row.jumpID)
		putInt32(record, 4, row.stargateID)
		putInt32(record, 8, row.fromSystemID)
		putInt32(record, 12, row.toSystemID)
		record[16] = 0
		putVector3(record, 17, 1, 2, 3)
		putVector3(record, 41, 4, 5, 6)
		data = append(data, record...)
	}
	return data
}

func staticLocalisation(names map[int]string) []byte {
	var data []byte
	ids := make([]int, 0, len(names))
	for id := range names {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		data = append(data, 0x4a)
		appendInt32(&data, id)
		name := []byte(names[id])
		if len(name) > 255 {
			panic("test localisation value is too long")
		}
		data = append(data, 0x8c, byte(len(name)))
		data = append(data, name...)
	}
	return data
}

func appendInt32(data *[]byte, value int) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(value))
	*data = append(*data, buf[:]...)
}

func putInt32(data []byte, offset int, value int) {
	binary.LittleEndian.PutUint32(data[offset:offset+4], uint32(value))
}

func putFloat32(data []byte, offset int, value float32) {
	binary.LittleEndian.PutUint32(data[offset:offset+4], math.Float32bits(value))
}

func putFloat64(data []byte, offset int, value float64) {
	binary.LittleEndian.PutUint64(data[offset:offset+8], math.Float64bits(value))
}

func putVector3(data []byte, offset int, x, y, z float64) {
	putFloat64(data, offset, x)
	putFloat64(data, offset+8, y)
	putFloat64(data, offset+16, z)
}
