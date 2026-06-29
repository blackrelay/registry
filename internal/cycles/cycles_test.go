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
			name: "cycle 5 period is outside supported public normalisation",
			at:   time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC),
			want: nil,
		},
		{
			name: "before cycle 6",
			at:   time.Date(2026, 6, 25, 8, 59, 59, 0, time.UTC),
			want: nil,
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

func TestParseScopeDefaultsToCurrentOnly(t *testing.T) {
	tests := []struct {
		name             string
		value            string
		defaultCurrent   bool
		wantCycles       []int
		wantIncludeEmpty bool
		wantAll          bool
	}{
		{
			name:           "missing value defaults to current cycle only",
			defaultCurrent: true,
			wantCycles:     []int{6},
		},
		{
			name:       "current is explicit current cycle only",
			value:      "current",
			wantCycles: []int{6},
		},
		{
			name:       "current cycle number is accepted explicitly",
			value:      "6",
			wantCycles: []int{6},
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
	for _, value := range []string{"0", "-1", "five", "5,,bad", "5", "all", "5,6"} {
		if _, err := ParseScope(value, true); err == nil {
			t.Fatalf("ParseScope(%q) succeeded, want error", value)
		}
	}
}

func TestWindowUsesCurrentCycleBoundary(t *testing.T) {
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
