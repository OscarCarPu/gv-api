package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type CreateHabitRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type CreateHabitResponse struct {
	ID          int32   `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type HabitWithLog struct {
	ID          int32    `json:"id"`
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	LogValue    *float32 `json:"log_value"`
}

type LogRequest struct {
	HabitID int32  `json:"habit_id"`
	Date    string `json:"date"`
	Value   float32 `json:"value"`
}

func TestE2E_InsertLogAndVerifyDailyView(t *testing.T) {
	truncateTables(t)

	baseURL := getBaseURL(t)
	client := &http.Client{Timeout: 10 * time.Second}

	// Setup: Create a habit via API
	t.Log("Creating habit")
	desc := "E2E test habit"
	createReq := CreateHabitRequest{Name: "Test Weight", Description: &desc}
	createBody, _ := json.Marshal(createReq)

	resp, err := client.Post(baseURL+"/habits", "application/json", bytes.NewBuffer(createBody))
	if err != nil {
		t.Fatalf("failed to create habit: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}

	var habit CreateHabitResponse
	if err := json.NewDecoder(resp.Body).Decode(&habit); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}
	t.Logf("Created habit with ID %d", habit.ID)

	// Test: Insert a log for the created habit
	testDate := "2025-01-31"
	testValue := float32(64.22)

	t.Log("Inserting log")
	logReq := LogRequest{HabitID: habit.ID, Date: testDate, Value: testValue}
	logBody, _ := json.Marshal(logReq)

	resp, err = client.Post(baseURL+"/habits/log", "application/json", bytes.NewBuffer(logBody))
	if err != nil {
		t.Fatalf("failed to insert log: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify: Check the daily view
	t.Log("Verifying daily view")
	resp, err = client.Get(fmt.Sprintf("%s/habits?date=%s", baseURL, testDate))
	if err != nil {
		t.Fatalf("failed to get daily view: %v", err)
	}
	defer resp.Body.Close()

	var habits []HabitWithLog
	if err := json.NewDecoder(resp.Body).Decode(&habits); err != nil {
		t.Fatalf("failed to decode daily view: %v", err)
	}

	t.Log("Verifying log value")
	var foundHabit *HabitWithLog
	for i, h := range habits {
		if h.ID == habit.ID {
			foundHabit = &habits[i]
			break
		}
	}

	if foundHabit == nil {
		t.Fatalf("failed to find habit with ID %d", habit.ID)
	}

	if foundHabit.LogValue == nil {
		t.Fatal("expected log value, got nil")
	}

	if *foundHabit.LogValue != testValue {
		t.Errorf("expected log value %f, got %f", testValue, *foundHabit.LogValue)
	}

	t.Log("Test passed")
}
