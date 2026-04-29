// Package config loads and validates the application configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strconv"
)

const defaultWebPort = 8889

// Config holds the application configuration loaded from environment variables.
type Config struct {
	APIKey  string
	StoreID string
	WebPort int
}

// Load reads configuration from environment variables and validates that all
// required values are present.
func Load() (*Config, error) {
	webPort := defaultWebPort
	if p := os.Getenv("WEB_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			webPort = n
		}
	}

	cfg := &Config{
		APIKey:  os.Getenv("PRINTAGO_API_KEY"),
		StoreID: os.Getenv("PRINTAGO_STORE_ID"),
		WebPort: webPort,
	}

	if cfg.APIKey == "" {
		return nil, errors.New("PRINTAGO_API_KEY environment variable is required")
	}
	if cfg.StoreID == "" {
		return nil, errors.New("PRINTAGO_STORE_ID environment variable is required")
	}

	return cfg, nil
}
