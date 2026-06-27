package staticdata

import (
	"testing"

	"github.com/blackrelay/registry/internal/model"
)

func TestReviewedEnemiesPreserveKnownBoundaries(t *testing.T) {
	enemies := ReviewedEnemies()
	if err := ValidateReviewedEnemies(enemies); err != nil {
		t.Fatalf("reviewed enemies failed validation: %v", err)
	}
	group27 := 0
	xeroti := 0
	for _, enemy := range enemies {
		if enemy.Name == "Cairo" {
			t.Fatal("Cairo must not appear in reviewed enemy candidates")
		}
		if enemy.GroupID == 27 {
			group27++
			if enemy.Name != "Feral Mooneater" || enemy.TypeID != 85702 {
				t.Fatalf("unexpected group 27 enemy: %#v", enemy)
			}
		}
		if enemy.Name == "Xeroti Tidofiza" {
			xeroti++
		}
	}
	if group27 != 1 {
		t.Fatalf("expected one reviewed group 27 exception, got %d", group27)
	}
	if xeroti != 5 {
		t.Fatalf("expected five distinct Xeroti Tidofiza type rows, got %d", xeroti)
	}
}

func TestEnemyIdentityAndDisplay(t *testing.T) {
	if got := EntityID(model.EnvironmentStillness, 92096); got != "enemy:stillness:type:92096" {
		t.Fatalf("unexpected entity id %q", got)
	}
	if got := DisplayName("Caird"); got != "Caird [NPC]" {
		t.Fatalf("unexpected display name %q", got)
	}
	if got := Slug("Grave Ostler", 92273, model.EnvironmentStillness); got != "enemy-grave-ostler-92273-stillness" {
		t.Fatalf("unexpected slug %q", got)
	}
}
