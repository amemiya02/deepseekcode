package main

import "time"

// Config holds application configuration.
type Config struct {
	DatabaseURL     string
	MaxRetries      int
	Timeout         time.Duration
	EnableTelemetry bool
	LogLevel        string
}

// LoadConfig returns a default configuration.
func LoadConfig() *Config {
	return &Config{
		DatabaseURL:     "postgres://localhost:5432/app",
		MaxRetries:      3,
		Timeout:         30 * time.Second,
		EnableTelemetry: false,
		LogLevel:        "info",
	}
}
