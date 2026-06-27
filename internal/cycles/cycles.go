package cycles

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Boundary struct {
	Cycle    int
	StartsAt time.Time
	Notes    string
}

type Scope struct {
	Cycles          []int
	IncludeUncycled bool
}

type TimeWindow struct {
	Cycle      int
	StartsAt   time.Time
	EndsBefore *time.Time
}

var boundaries = []Boundary{
	{
		Cycle:    5,
		StartsAt: time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC),
		Notes:    "Cycle 5 began at 2026-03-11T09:00:00Z.",
	},
	{
		Cycle:    6,
		StartsAt: time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC),
		Notes:    "Cycle 6 began at 2026-06-25T09:00:00Z.",
	},
}

func FromTime(value time.Time) *int {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	var cycle *int
	for _, boundary := range boundaries {
		if value.Before(boundary.StartsAt) {
			break
		}
		item := boundary.Cycle
		cycle = &item
	}
	return cycle
}

func Current() int {
	if len(boundaries) == 0 {
		return 0
	}
	return boundaries[len(boundaries)-1].Cycle
}

func ParseScope(value string, defaultCurrent bool) (Scope, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if defaultCurrent {
			return Scope{Cycles: []int{Current()}, IncludeUncycled: true}, nil
		}
		return Scope{}, nil
	}
	switch strings.ToLower(value) {
	case "all":
		return Scope{}, nil
	case "current":
		return Scope{Cycles: []int{Current()}}, nil
	}
	seen := make(map[int]struct{})
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return Scope{}, fmt.Errorf("invalid cycle %q", value)
		}
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed <= 0 {
			return Scope{}, fmt.Errorf("invalid cycle %q", part)
		}
		seen[parsed] = struct{}{}
	}
	cycles := make([]int, 0, len(seen))
	for cycle := range seen {
		cycles = append(cycles, cycle)
	}
	sort.Ints(cycles)
	return Scope{Cycles: cycles}, nil
}

func (s Scope) All() bool {
	return len(s.Cycles) == 0
}

func (s Scope) Contains(cycle *int) bool {
	if len(s.Cycles) == 0 {
		return true
	}
	if cycle == nil {
		return s.IncludeUncycled
	}
	for _, candidate := range s.Cycles {
		if candidate == *cycle {
			return true
		}
	}
	return false
}

func Window(cycle int) (TimeWindow, bool) {
	for i, boundary := range boundaries {
		if boundary.Cycle != cycle {
			continue
		}
		window := TimeWindow{Cycle: cycle, StartsAt: boundary.StartsAt}
		if i+1 < len(boundaries) {
			end := boundaries[i+1].StartsAt
			window.EndsBefore = &end
		}
		return window, true
	}
	return TimeWindow{}, false
}

func Boundaries() []Boundary {
	out := make([]Boundary, len(boundaries))
	copy(out, boundaries)
	return out
}
