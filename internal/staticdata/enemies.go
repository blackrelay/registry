package staticdata

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/blackrelay/registry/internal/model"
)

type EnemyCandidate struct {
	Name       string `json:"name"`
	GroupID    int    `json:"groupId"`
	TypeID     int    `json:"typeId"`
	Confidence string `json:"confidence"`
	Basis      string `json:"basis"`
}

var reviewedEnemies = []EnemyCandidate{
	{Name: "Caird", GroupID: 5033, TypeID: 92096, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Dowser", GroupID: 5033, TypeID: 92101, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Grave Caird", GroupID: 5033, TypeID: 92271, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Grave Luthier", GroupID: 5033, TypeID: 92272, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Grave Ostler", GroupID: 5033, TypeID: 92273, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Grave Shambler", GroupID: 5033, TypeID: 92275, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Grave Wright", GroupID: 5033, TypeID: 92274, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Hellier", GroupID: 5033, TypeID: 93871, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Luthier", GroupID: 5033, TypeID: 92097, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Ostler", GroupID: 5033, TypeID: 92098, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Scrivener", GroupID: 5033, TypeID: 92102, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Shambler", GroupID: 5033, TypeID: 92100, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Wright", GroupID: 5033, TypeID: 92099, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 5033"},
	{Name: "Symbiote Enforcer", GroupID: 4963, TypeID: 91180, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4963"},
	{Name: "Symbiote Processor", GroupID: 4963, TypeID: 91179, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4963"},
	{Name: "Symbiote Scout", GroupID: 4963, TypeID: 91291, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4963"},
	{Name: "Symbiote Striker", GroupID: 4963, TypeID: 91183, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4963"},
	{Name: "Xeroti Tidofiza", GroupID: 4963, TypeID: 91011, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4963"},
	{Name: "Xeroti Tidofiza", GroupID: 4963, TypeID: 91368, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4963"},
	{Name: "Xeroti Tidofiza", GroupID: 4963, TypeID: 91369, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4963"},
	{Name: "Xeroti Tidofiza", GroupID: 4963, TypeID: 91370, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4963"},
	{Name: "Xeroti Tidofiza", GroupID: 4963, TypeID: 91371, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4963"},
	{Name: "Defective Mooneater", GroupID: 4770, TypeID: 85278, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4770"},
	{Name: "Feral Mooneater", GroupID: 4770, TypeID: 83487, Confidence: string(model.ConfidenceProbable), Basis: "confirmed enemy group 4770"},
	{Name: "Feral Mooneater", GroupID: 27, TypeID: 85702, Confidence: string(model.ConfidenceProbable), Basis: "reviewed individual enemy row; group 27 is not an enemy group"},
	{Name: "Inert Drone", GroupID: 306, TypeID: 88089, Confidence: string(model.ConfidenceProbable), Basis: "reviewed individual enemy row outside enemy groups"},
}

func ReviewedEnemies() []EnemyCandidate {
	out := append([]EnemyCandidate(nil), reviewedEnemies...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].TypeID < out[j].TypeID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func ValidateReviewedEnemies(candidates []EnemyCandidate) error {
	if len(candidates) != 26 {
		return fmt.Errorf("expected 26 reviewed enemies, got %d", len(candidates))
	}
	seenType := make(map[int]string)
	for _, candidate := range candidates {
		if candidate.Name == "" || candidate.TypeID <= 0 || candidate.GroupID <= 0 {
			return fmt.Errorf("candidate is incomplete: %#v", candidate)
		}
		if strings.EqualFold(candidate.Name, "Cairo") {
			return errors.New("cairo is not a reviewed enemy name; use Caird")
		}
		if candidate.GroupID == 27 && !(candidate.Name == "Feral Mooneater" && candidate.TypeID == 85702) {
			return fmt.Errorf("group 27 is not an enemy group; unexpected candidate %s %d", candidate.Name, candidate.TypeID)
		}
		if existing, ok := seenType[candidate.TypeID]; ok {
			return fmt.Errorf("type id %d is duplicated by %s and %s", candidate.TypeID, existing, candidate.Name)
		}
		seenType[candidate.TypeID] = candidate.Name
	}
	return nil
}

func EntityID(environment model.Environment, typeID int) string {
	return fmt.Sprintf("enemy:%s:type:%d", environment, typeID)
}

func Slug(name string, typeID int, environment model.Environment) string {
	return fmt.Sprintf("enemy-%s-%d-%s", slugify(name), typeID, environment)
}

func DisplayName(name string) string {
	return name + " [NPC]"
}

func ParseCandidatesJSON(data []byte) ([]EnemyCandidate, error) {
	var envelope struct {
		Candidates []EnemyCandidate `json:"candidates"`
		Matches    []struct {
			Name   string `json:"name"`
			TypeID int    `json:"typeId"`
			Reason string `json:"reason"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Candidates) > 0 {
		return envelope.Candidates, nil
	}
	var out []EnemyCandidate
	for _, match := range envelope.Matches {
		groupID := groupIDFromReason(match.Reason)
		if groupID == 0 {
			return nil, fmt.Errorf("candidate %s does not include a groupID in its reason", match.Name)
		}
		out = append(out, EnemyCandidate{
			Name:       match.Name,
			GroupID:    groupID,
			TypeID:     match.TypeID,
			Confidence: string(model.ConfidenceProbable),
			Basis:      "reviewed static-client extraction output",
		})
	}
	return out, nil
}

var groupIDPattern = regexp.MustCompile(`groupID=(\d+)`)

func groupIDFromReason(reason string) int {
	match := groupIDPattern.FindStringSubmatch(reason)
	if len(match) != 2 {
		return 0
	}
	var groupID int
	_, _ = fmt.Sscanf(match[1], "%d", &groupID)
	return groupID
}

var slugReplace = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	lower := strings.ToLower(value)
	slug := slugReplace.ReplaceAllString(lower, "-")
	return strings.Trim(slug, "-")
}
