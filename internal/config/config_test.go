package config_test

import (
	"testing"

	"github.com/bandesz/printago-buddy/internal/config"
)

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
