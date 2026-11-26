// Package config loads and validates the application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultWebPort             = 8889
	defaultQueueWeightHalfLife = time.Hour
	defaultQueuePollInterval   = 15 * time.Second
	defaultSmartQueueStateFile = "/data/smart-queue.json"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	APIKey              string
	StoreID             string
	WebPort             int
	QueueWeightHalfLife time.Duration
	QueuePollInterval   time.Duration
	SmartQueueStateFile string
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

	stateFile := defaultSmartQueueStateFile
	if p := os.Getenv("SMART_QUEUE_STATE_FILE"); p != "" {
		stateFile = p
	}

	cfg := &Config{
		APIKey:              os.Getenv("PRINTAGO_API_KEY"),
		StoreID:             os.Getenv("PRINTAGO_STORE_ID"),
		WebPort:             webPort,
		SmartQueueStateFile: stateFile,
	}

	if cfg.APIKey == "" {
		return nil, errors.New("PRINTAGO_API_KEY environment variable is required")
	}
	if cfg.StoreID == "" {
		return nil, errors.New("PRINTAGO_STORE_ID environment variable is required")
	}

	halfLife, err := positiveDuration("QUEUE_WEIGHT_HALF_LIFE", defaultQueueWeightHalfLife)
	if err != nil {
		return nil, err
	}
	poll, err := positiveDuration("QUEUE_POLL_INTERVAL", defaultQueuePollInterval)
	if err != nil {
		return nil, err
	}
	cfg.QueueWeightHalfLife = halfLife
	cfg.QueuePollInterval = poll

	return cfg, nil
}

func positiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return d, nil
}
