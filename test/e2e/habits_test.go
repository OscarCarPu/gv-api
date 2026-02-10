package e2e

import (
	"testing"
)

func TestE2E_InsertLogAndVerifyDailyView(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	truncateTables(t)
	client := NewAPIClient(t)

	desc := "E2E test habit"
	habit := client.CreateHabit(t, CreateHabitRequest{Name: "Test Weight", Description: &desc})
	t.Logf("Created habit with ID %d", habit.ID)

	testDate := "2025-01-31"
	testValue := float32(64.22)
	client.LogHabit(t, LogRequest{HabitID: habit.ID, Date: testDate, Value: testValue})

	habits := client.GetDailyView(t, testDate)

	var found *HabitWithLog
	for i, h := range habits {
		if h.ID == habit.ID {
			found = &habits[i]
			break
		}
	}

	if found == nil {
		t.Fatalf("habit with ID %d not found in daily view", habit.ID)
	}
	if found.LogValue == nil {
		t.Fatal("got nil log value, want value")
	}
	if *found.LogValue != testValue {
		t.Errorf("got log value %f, want %f", *found.LogValue, testValue)
	}
}
