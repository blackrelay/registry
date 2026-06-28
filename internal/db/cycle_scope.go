package db

import (
	"time"

	"github.com/blackrelay/registry/internal/cycles"
)

func cycleInScope(cycle *int, cycles []int, includeUncycled bool) bool {
	if len(cycles) == 0 {
		return true
	}
	if cycle == nil {
		return includeUncycled
	}
	for _, candidate := range cycles {
		if candidate == *cycle {
			return true
		}
	}
	return false
}

func effectiveEventCycles(query EventQuery) []int {
	if len(query.Cycles) > 0 {
		return query.Cycles
	}
	if query.Cycle != nil {
		return []int{*query.Cycle}
	}
	return nil
}

func timeCycleInScope(value time.Time, cycleValues []int, includeUncycled bool) bool {
	if value.IsZero() {
		return cycleInScope(nil, cycleValues, includeUncycled)
	}
	cycle := cycles.FromTime(value)
	if cycle == nil && len(cycleValues) > 0 {
		return false
	}
	return cycleInScope(cycle, cycleValues, includeUncycled)
}
