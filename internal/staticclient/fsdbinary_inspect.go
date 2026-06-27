package staticclient

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	nativeFSDBinaryProbeDecoderStatus    = "native_probe_decoder_available"
	nativeFSDBinaryRowProbeDecoderStatus = "native_probe_row_decoder_available"
	maxNativeStaticTypeID                = 1000000
)

type StaticTypeInspectionOptions struct {
	ClientRoot   string
	ProbeTypeIDs []int
}

type StaticTypeInspectionResult struct {
	ParserStatus         string                  `json:"parserStatus"`
	TypeResource         StaticResourceEvidence  `json:"typeResource"`
	LocalizationResource *StaticResourceEvidence `json:"localizationResource,omitempty"`
	TypeFile             FSDBinaryFileSummary    `json:"typeFile"`
	Probes               []FSDBinaryProbe        `json:"probes,omitempty"`
	DecodedRows          []FSDBinaryDecodedRow   `json:"decodedRows,omitempty"`
}

type FSDBinaryFileSummary struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	HeaderHex string `json:"headerHex"`
}

type FSDBinaryProbe struct {
	TypeID              int     `json:"typeId"`
	LittleEndianOffsets []int64 `json:"littleEndianOffsets"`
}

type FSDBinaryDecodedRow struct {
	TypeID      int    `json:"typeId"`
	GroupID     int    `json:"groupId"`
	Name        string `json:"name,omitempty"`
	TypeNameID  int    `json:"typeNameId,omitempty"`
	WreckTypeID int    `json:"wreckTypeId,omitempty"`
	OffsetBytes int64  `json:"offsetBytes"`
	Basis       string `json:"basis"`
}

func InspectStaticClientTypes(ctx context.Context, opts StaticTypeInspectionOptions) (StaticTypeInspectionResult, error) {
	if strings.TrimSpace(opts.ClientRoot) == "" {
		return StaticTypeInspectionResult{}, errors.New("client root is required")
	}
	entries, _, err := ReadResourceIndex(opts.ClientRoot)
	if err != nil {
		return StaticTypeInspectionResult{}, err
	}
	typeEntry, ok := entries.Find("res:/staticdata/types.fsdbinary")
	if !ok {
		return StaticTypeInspectionResult{}, errors.New("types.fsdbinary resource was not found")
	}
	typeEvidence, err := ResourceEvidence(opts.ClientRoot, typeEntry)
	if err != nil {
		return StaticTypeInspectionResult{}, err
	}

	var localisationEvidence *StaticResourceEvidence
	localisationNames := map[int]string(nil)
	if locEntry, ok := entries.Find("res:/localizationfsd/localization_fsd_en-us.pickle"); ok {
		evidence, err := ResourceEvidence(opts.ClientRoot, locEntry)
		if err == nil {
			localisationEvidence = &evidence
			if data, err := os.ReadFile(evidence.Path); err == nil {
				localisationNames = DecodeLocalizationStrings(data)
			}
		}
	}

	data, err := os.ReadFile(typeEvidence.Path)
	if err != nil {
		return StaticTypeInspectionResult{}, err
	}
	headerLength := min(len(data), 32)
	probes := inspectLittleEndianInt32Offsets(ctx, data, opts.ProbeTypeIDs)
	decodedRows := decodeFSDBinaryProbeRows(ctx, data, probes, localisationNames)
	parserStatus := nativeFSDBinaryProbeDecoderStatus
	if len(decodedRows) > 0 {
		parserStatus = nativeFSDBinaryRowProbeDecoderStatus
	}
	result := StaticTypeInspectionResult{
		ParserStatus:         parserStatus,
		TypeResource:         typeEvidence,
		LocalizationResource: localisationEvidence,
		TypeFile: FSDBinaryFileSummary{
			Path:      typeEvidence.Path,
			SizeBytes: int64(len(data)),
			HeaderHex: hex.EncodeToString(data[:headerLength]),
		},
		Probes:      probes,
		DecodedRows: decodedRows,
	}
	return result, nil
}

func inspectLittleEndianInt32Offsets(ctx context.Context, data []byte, ids []int) []FSDBinaryProbe {
	ids = uniquePositiveInts(ids)
	out := make([]FSDBinaryProbe, 0, len(ids))
	for _, id := range ids {
		select {
		case <-ctx.Done():
			return out
		default:
		}
		pattern := []byte{byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24)}
		var offsets []int64
		for i := 0; i <= len(data)-4; i++ {
			if i%1048576 == 0 {
				select {
				case <-ctx.Done():
					return out
				default:
				}
			}
			if data[i] == pattern[0] && data[i+1] == pattern[1] && data[i+2] == pattern[2] && data[i+3] == pattern[3] {
				offsets = append(offsets, int64(i))
			}
		}
		out = append(out, FSDBinaryProbe{TypeID: id, LittleEndianOffsets: offsets})
	}
	return out
}

func decodeFSDBinaryProbeRows(ctx context.Context, data []byte, probes []FSDBinaryProbe, localisationNames map[int]string) []FSDBinaryDecodedRow {
	seen := make(map[string]struct{})
	var out []FSDBinaryDecodedRow
	for _, probe := range probes {
		for _, offset := range probe.LittleEndianOffsets {
			select {
			case <-ctx.Done():
				return out
			default:
			}
			row, ok := decodeFSDBinaryProbeRowAt(data, probe.TypeID, offset)
			if !ok {
				continue
			}
			if localisationNames != nil && row.TypeNameID > 0 {
				row.Name = localisationNames[row.TypeNameID]
			}
			key := strings.Join([]string{strconv.Itoa(row.TypeID), strconv.Itoa(row.GroupID), strconv.Itoa(row.TypeNameID), strconv.Itoa(row.WreckTypeID), strconv.FormatInt(row.OffsetBytes, 10)}, ":")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TypeID == out[j].TypeID {
			return out[i].OffsetBytes < out[j].OffsetBytes
		}
		return out[i].TypeID < out[j].TypeID
	})
	return out
}

func DecodeFSDBinaryTypeRows(ctx context.Context, data []byte, localisationNames map[int]string) []FSDBinaryDecodedRow {
	if len(localisationNames) == 0 || len(data) < 48 {
		return nil
	}
	seenTypeIDs := make(map[int]struct{})
	out := make([]FSDBinaryDecodedRow, 0)
	for offset := int64(32); offset+16 <= int64(len(data)); offset++ {
		if offset%1048576 == 0 {
			select {
			case <-ctx.Done():
				return out
			default:
			}
		}
		typeID := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		if typeID <= 0 || typeID > 10000000 {
			continue
		}
		if _, ok := seenTypeIDs[typeID]; ok {
			continue
		}
		row, ok := decodeFSDBinaryProbeRowAt(data, typeID, offset)
		if !ok {
			continue
		}
		name := strings.TrimSpace(localisationNames[row.TypeNameID])
		if name == "" {
			continue
		}
		row.Name = name
		if !validNativeTypeRow(data, row, localisationNames) {
			continue
		}
		seenTypeIDs[typeID] = struct{}{}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GroupID == out[j].GroupID {
			if out[i].TypeID == out[j].TypeID {
				return out[i].Name < out[j].Name
			}
			return out[i].TypeID < out[j].TypeID
		}
		return out[i].GroupID < out[j].GroupID
	})
	return out
}

func DecodeLocalizationStrings(data []byte) map[int]string {
	out := make(map[int]string)
	for offset := 0; offset+7 < len(data); offset++ {
		if data[offset] != 0x4a {
			continue
		}
		id := int(binary.LittleEndian.Uint32(data[offset+1 : offset+5]))
		if id <= 0 {
			continue
		}
		value, ok := decodePickleStringAfterBinaryInt(data[offset+5:])
		if !ok {
			continue
		}
		value = repairStaticClientText(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := out[id]; !exists {
			out[id] = value
		}
	}
	return out
}

func decodePickleStringAfterBinaryInt(data []byte) (string, bool) {
	if len(data) < 2 {
		return "", false
	}
	switch data[0] {
	case 0x8c:
		length := int(data[1])
		if len(data) < 2+length {
			return "", false
		}
		return decodeLocalisationUTF8(data[2 : 2+length])
	case 0x58:
		if len(data) < 5 {
			return "", false
		}
		length := int(binary.LittleEndian.Uint32(data[1:5]))
		if length < 0 || len(data) < 5+length {
			return "", false
		}
		return decodeLocalisationUTF8(data[5 : 5+length])
	default:
		return "", false
	}
}

func decodeLocalisationUTF8(data []byte) (string, bool) {
	if len(data) == 0 || !utf8.Valid(data) {
		return "", false
	}
	value := string(data)
	for _, r := range value {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if r < 0x20 {
			return "", false
		}
	}
	return value, true
}

func decodeFSDBinaryProbeRowAt(data []byte, typeID int, offset int64) (FSDBinaryDecodedRow, bool) {
	if typeID <= 0 || offset < 32 || offset+16 > int64(len(data)) {
		return FSDBinaryDecodedRow{}, false
	}
	if typeID > maxNativeStaticTypeID {
		return FSDBinaryDecodedRow{}, false
	}
	groupID := int(binary.LittleEndian.Uint32(data[offset-32 : offset-28]))
	typeNameID := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
	wreckTypeID := int(binary.LittleEndian.Uint32(data[offset+12 : offset+16]))
	if groupID <= 0 || groupID > 100000 || typeNameID <= 0 || typeNameID > 10000000 {
		return FSDBinaryDecodedRow{}, false
	}
	if wreckTypeID < 0 || wreckTypeID > 10000000 {
		return FSDBinaryDecodedRow{}, false
	}
	return FSDBinaryDecodedRow{
		TypeID:      typeID,
		GroupID:     groupID,
		TypeNameID:  typeNameID,
		WreckTypeID: wreckTypeID,
		OffsetBytes: offset,
		Basis:       "little-endian type row probe: groupId at typeId-32, typeNameId at typeId+4, wreckTypeId at typeId+12",
	}, true
}

func validNativeTypeRow(data []byte, row FSDBinaryDecodedRow, localisationNames map[int]string) bool {
	if row.TypeID <= 0 || row.TypeID > maxNativeStaticTypeID {
		return false
	}
	if row.TypeID < 1000 && row.TypeNameID >= 500000 {
		return false
	}
	if isLikelyBinarySentinel(row.TypeNameID) || isLikelyBinarySentinel(row.WreckTypeID) {
		return false
	}
	if isAllASCIIDigits(row.Name) {
		return false
	}
	if offsetLooksLikeNeighbourTypeRow(data, row.OffsetBytes, row.TypeNameID, localisationNames) {
		return false
	}
	return true
}

func offsetLooksLikeNeighbourTypeRow(data []byte, offset int64, typeID int, localisationNames map[int]string) bool {
	if typeID <= 0 || typeID > maxNativeStaticTypeID || offset+4 < 32 || offset+20 > int64(len(data)) {
		return false
	}
	next, ok := decodeFSDBinaryProbeRowAt(data, typeID, offset+4)
	if !ok {
		return false
	}
	if strings.TrimSpace(localisationNames[next.TypeNameID]) == "" {
		return false
	}
	return true
}

func isLikelyBinarySentinel(value int) bool {
	return value >= 65536 && value&(value-1) == 0
}

func isAllASCIIDigits(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func uniquePositiveInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}
