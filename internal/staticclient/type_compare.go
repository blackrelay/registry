package staticclient

import (
	"os"
	"sort"
)

const staticTypeCompareSchema = "registry.static-client-type-compare.v1"

type StaticTypeCompareResult struct {
	SchemaVersion     string                    `json:"schemaVersion"`
	ResolvedPath      string                    `json:"resolvedPath"`
	NativePath        string                    `json:"nativePath"`
	ResolvedCount     int                       `json:"resolvedCount"`
	NativeCount       int                       `json:"nativeCount"`
	MatchedCount      int                       `json:"matchedCount"`
	SemanticallyEqual bool                      `json:"semanticallyEqual"`
	SafeToPromote     bool                      `json:"safeToPromote"`
	ResolvedOnly      []StaticTypeCompareDelta  `json:"resolvedOnly,omitempty"`
	NativeOnly        []StaticTypeCompareDelta  `json:"nativeOnly,omitempty"`
	ChangedName       []StaticTypeCompareDelta  `json:"changedName,omitempty"`
	ChangedGroup      []StaticTypeCompareDelta  `json:"changedGroup,omitempty"`
	ChangedWreckType  []StaticTypeCompareDelta  `json:"changedWreckType,omitempty"`
	DuplicateNames    []StaticTypeDuplicateName `json:"duplicateNames,omitempty"`
}

type StaticTypeCompareDelta struct {
	TypeID   int                   `json:"typeId"`
	Resolved *StaticTypeCompareRow `json:"resolved,omitempty"`
	Native   *StaticTypeCompareRow `json:"native,omitempty"`
}

type StaticTypeCompareRow struct {
	TypeID      int    `json:"typeId"`
	GroupID     int    `json:"groupId"`
	Name        string `json:"name"`
	WreckTypeID int    `json:"wreckTypeId,omitempty"`
}

type StaticTypeDuplicateName struct {
	Name    string `json:"name"`
	TypeIDs []int  `json:"typeIds"`
}

func CompareStaticTypeFiles(resolvedPath, nativePath string) (StaticTypeCompareResult, error) {
	resolvedData, err := os.ReadFile(resolvedPath)
	if err != nil {
		return StaticTypeCompareResult{}, err
	}
	nativeData, err := os.ReadFile(nativePath)
	if err != nil {
		return StaticTypeCompareResult{}, err
	}
	resolvedRows, err := decodeStaticTypeRows(resolvedData)
	if err != nil {
		return StaticTypeCompareResult{}, err
	}
	nativeRows, err := decodeStaticTypeRows(nativeData)
	if err != nil {
		return StaticTypeCompareResult{}, err
	}
	resolvedRows = normaliseStaticTypeRows(resolvedRows)
	nativeRows = normaliseStaticTypeRows(nativeRows)
	result := StaticTypeCompareResult{
		SchemaVersion: staticTypeCompareSchema,
		ResolvedPath:  resolvedPath,
		NativePath:    nativePath,
		ResolvedCount: len(resolvedRows),
		NativeCount:   len(nativeRows),
	}
	resolvedByType := staticTypeRowsByTypeID(resolvedRows)
	nativeByType := staticTypeRowsByTypeID(nativeRows)
	for typeID, resolved := range resolvedByType {
		native, ok := nativeByType[typeID]
		if !ok {
			result.ResolvedOnly = append(result.ResolvedOnly, StaticTypeCompareDelta{
				TypeID:   typeID,
				Resolved: compareRowPtr(resolved),
			})
			continue
		}
		result.MatchedCount++
		if resolved.Name != native.Name {
			result.ChangedName = append(result.ChangedName, StaticTypeCompareDelta{
				TypeID:   typeID,
				Resolved: compareRowPtr(resolved),
				Native:   compareRowPtr(native),
			})
		}
		if resolved.GroupID != native.GroupID {
			result.ChangedGroup = append(result.ChangedGroup, StaticTypeCompareDelta{
				TypeID:   typeID,
				Resolved: compareRowPtr(resolved),
				Native:   compareRowPtr(native),
			})
		}
		if resolved.WreckTypeID != native.WreckTypeID {
			result.ChangedWreckType = append(result.ChangedWreckType, StaticTypeCompareDelta{
				TypeID:   typeID,
				Resolved: compareRowPtr(resolved),
				Native:   compareRowPtr(native),
			})
		}
	}
	for typeID, native := range nativeByType {
		if _, ok := resolvedByType[typeID]; ok {
			continue
		}
		result.NativeOnly = append(result.NativeOnly, StaticTypeCompareDelta{
			TypeID: typeID,
			Native: compareRowPtr(native),
		})
	}
	sortCompareDeltas(result.ResolvedOnly)
	sortCompareDeltas(result.NativeOnly)
	sortCompareDeltas(result.ChangedName)
	sortCompareDeltas(result.ChangedGroup)
	sortCompareDeltas(result.ChangedWreckType)
	result.DuplicateNames = duplicateStaticTypeNames(append(append([]staticTypeRow{}, resolvedRows...), nativeRows...))
	result.SemanticallyEqual = len(result.ResolvedOnly) == 0 &&
		len(result.NativeOnly) == 0 &&
		len(result.ChangedName) == 0 &&
		len(result.ChangedGroup) == 0 &&
		len(result.ChangedWreckType) == 0
	result.SafeToPromote = result.SemanticallyEqual
	return result, nil
}

func staticTypeRowsByTypeID(rows []staticTypeRow) map[int]staticTypeRow {
	out := make(map[int]staticTypeRow, len(rows))
	for _, row := range rows {
		if row.TypeID <= 0 {
			continue
		}
		out[row.TypeID] = row
	}
	return out
}

func compareRowPtr(row staticTypeRow) *StaticTypeCompareRow {
	return &StaticTypeCompareRow{
		TypeID:      row.TypeID,
		GroupID:     row.GroupID,
		Name:        row.Name,
		WreckTypeID: row.WreckTypeID,
	}
}

func sortCompareDeltas(items []StaticTypeCompareDelta) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].TypeID < items[j].TypeID
	})
}

func duplicateStaticTypeNames(rows []staticTypeRow) []StaticTypeDuplicateName {
	typeIDsByName := make(map[string]map[int]struct{})
	for _, row := range rows {
		if row.Name == "" || row.TypeID <= 0 {
			continue
		}
		if typeIDsByName[row.Name] == nil {
			typeIDsByName[row.Name] = make(map[int]struct{})
		}
		typeIDsByName[row.Name][row.TypeID] = struct{}{}
	}
	out := make([]StaticTypeDuplicateName, 0)
	for name, typeSet := range typeIDsByName {
		if len(typeSet) < 2 {
			continue
		}
		item := StaticTypeDuplicateName{Name: name, TypeIDs: make([]int, 0, len(typeSet))}
		for typeID := range typeSet {
			item.TypeIDs = append(item.TypeIDs, typeID)
		}
		sort.Ints(item.TypeIDs)
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
