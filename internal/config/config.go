package config

import (
	"errors"
	"os"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	APIKey  string
	StoreID string
}

// Load reads configuration from environment variables and validates that all
// required values are present.
func Load() (*Config, error) {
	cfg := &Config{
		APIKey:  os.Getenv("PRINTAGO_API_KEY"),
		StoreID: os.Getenv("PRINTAGO_STORE_ID"),
	}

	if cfg.APIKey == "" {
		return nil, errors.New("PRINTAGO_API_KEY environment variable is required")
	}
	if cfg.StoreID == "" {
		return nil, errors.New("PRINTAGO_STORE_ID environment variable is required")
	}

	return cfg, nil
}
