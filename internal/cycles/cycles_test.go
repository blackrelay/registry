package cycles

import (
	"testing"
	"time"
)

func TestFromTimeUsesCycleBoundaryInstants(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		want *int
	}{
		{
			name: "before cycle 3",
			at:   time.Date(2025, 10, 15, 14, 59, 59, 0, time.UTC),
			want: nil,
		},
		{
			name: "cycle 3 historical start is outside indexed Sui normalisation",
			at:   time.Date(2025, 10, 15, 15, 0, 0, 0, time.UTC),
			want: nil,
		},
		{
			name: "cycle 4 historical start is outside indexed Sui normalisation",
			at:   time.Date(2025, 12, 10, 9, 0, 0, 0, time.UTC),
			want: nil,
		},
		{
			name: "before cycle 5",
			at:   time.Date(2026, 3, 11, 8, 59, 59, 0, time.UTC),
			want: nil,
		},
		{
			name: "cycle 5 start",
			at:   time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC),
			want: intRef(5),
		},
		{
			name: "before cycle 6",
			at:   time.Date(2026, 6, 25, 8, 59, 59, 0, time.UTC),
			want: intRef(5),
		},
		{
			name: "cycle 6 start",
			at:   time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC),
			want: intRef(6),
		},
		{
			name: "zero time",
			at:   time.Time{},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromTime(tt.at)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil cycle, got %d", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("expected cycle %d, got %#v", *tt.want, got)
			}
		})
	}
}

func TestParseScopeDefaultsToCurrentAndSupportsArchiveOptIn(t *testing.T) {
	tests := []struct {
		name             string
		value            string
		defaultCurrent   bool
		wantCycles       []int
		wantIncludeEmpty bool
		wantAll          bool
	}{
		{
			name:             "missing value defaults to current cycle plus unlabelled rows",
			defaultCurrent:   true,
			wantCycles:       []int{6},
			wantIncludeEmpty: true,
		},
		{
			name:       "all disables cycle filtering",
			value:      "all",
			wantAll:    true,
			wantCycles: nil,
		},
		{
			name:       "current is explicit current cycle only",
			value:      "current",
			wantCycles: []int{6},
		},
		{
			name:       "comma-separated cycles are de-duplicated and sorted",
			value:      "6, 5, 6",
			wantCycles: []int{5, 6},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseScope(tt.value, tt.defaultCurrent)
			if err != nil {
				t.Fatal(err)
			}
			if got.All() != tt.wantAll {
				t.Fatalf("All() = %v, want %v", got.All(), tt.wantAll)
			}
			if got.IncludeUncycled != tt.wantIncludeEmpty {
				t.Fatalf("IncludeUncycled = %v, want %v", got.IncludeUncycled, tt.wantIncludeEmpty)
			}
			if len(got.Cycles) != len(tt.wantCycles) {
				t.Fatalf("cycles = %#v, want %#v", got.Cycles, tt.wantCycles)
			}
			for i := range got.Cycles {
				if got.Cycles[i] != tt.wantCycles[i] {
					t.Fatalf("cycles = %#v, want %#v", got.Cycles, tt.wantCycles)
				}
			}
		})
	}
}

func TestParseScopeRejectsInvalidCycles(t *testing.T) {
	for _, value := range []string{"0", "-1", "five", "5,,bad"} {
		if _, err := ParseScope(value, true); err == nil {
			t.Fatalf("ParseScope(%q) succeeded, want error", value)
		}
	}
}

func TestWindowUsesNextCycleBoundary(t *testing.T) {
	window, ok := Window(5)
	if !ok {
		t.Fatal("cycle 5 window was not found")
	}
	if !window.StartsAt.Equal(time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("cycle 5 start = %s", window.StartsAt)
	}
	if window.EndsBefore == nil || !window.EndsBefore.Equal(time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("cycle 5 end = %#v", window.EndsBefore)
	}
	current, ok := Window(6)
	if !ok {
		t.Fatal("cycle 6 window was not found")
	}
	if current.EndsBefore != nil {
		t.Fatalf("current cycle should not have an end boundary yet: %#v", current.EndsBefore)
	}
}

func intRef(value int) *int {
	return &value
}
