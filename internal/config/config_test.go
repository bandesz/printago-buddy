package config_test

import (
	"testing"
	"time"

	"github.com/bandesz/printago-buddy/internal/config"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PRINTAGO_API_KEY", "test-key")
	t.Setenv("PRINTAGO_STORE_ID", "test-store")
}

func clearQueueEnv(t *testing.T) {
	t.Helper()
	t.Setenv("QUEUE_POLL_INTERVAL", "")
	t.Setenv("SMART_QUEUE_STATE_FILE", "")
}

func TestLoad_success(t *testing.T) {
	t.Setenv("PRINTAGO_API_KEY", "test-key")
	t.Setenv("PRINTAGO_STORE_ID", "test-store")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-key")
	}
	if cfg.StoreID != "test-store" {
		t.Errorf("StoreID = %q, want %q", cfg.StoreID, "test-store")
	}
}

func TestLoad_missingAPIKey(t *testing.T) {
	t.Setenv("PRINTAGO_API_KEY", "")
	t.Setenv("PRINTAGO_STORE_ID", "test-store")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when API key is missing, got nil")
	}
}

func TestLoad_missingStoreID(t *testing.T) {
	t.Setenv("PRINTAGO_API_KEY", "test-key")
	t.Setenv("PRINTAGO_STORE_ID", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when store ID is missing, got nil")
	}
}

func TestPoll_defaultsTo15s(t *testing.T) {
	setRequiredEnv(t)
	clearQueueEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.QueuePollInterval != 15*time.Second {
		t.Errorf("QueuePollInterval = %v, want 15s", cfg.QueuePollInterval)
	}
}

func TestPoll_keeps5s(t *testing.T) {
	setRequiredEnv(t)
	clearQueueEnv(t)
	t.Setenv("QUEUE_POLL_INTERVAL", "5s")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.QueuePollInterval != 5*time.Second {
		t.Errorf("QueuePollInterval = %v, want 5s", cfg.QueuePollInterval)
	}
}

func TestPoll_rejectsInvalid(t *testing.T) {
	for _, value := range []string{"0", "-1s", "abc"} {
		t.Run(value, func(t *testing.T) {
			setRequiredEnv(t)
			clearQueueEnv(t)
			t.Setenv("QUEUE_POLL_INTERVAL", value)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("expected error for QUEUE_POLL_INTERVAL=%q", value)
			}
		})
	}
}

func TestStateFile_defaultsToDataPath(t *testing.T) {
	setRequiredEnv(t)
	clearQueueEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SmartQueueStateFile != "/data/smart-queue.json" {
		t.Errorf("SmartQueueStateFile = %q, want %q", cfg.SmartQueueStateFile, "/data/smart-queue.json")
	}
}
