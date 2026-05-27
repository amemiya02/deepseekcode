package main

import (
	"fmt"
	"strings"
)

// Service processes data with configurable behavior.
type Service struct {
	cfg     *Config
	storage Storage
	cache   map[string]string
}

// NewService creates a new Service.
func NewService(cfg *Config) *Service {
	return &Service{
		cfg:     cfg,
		storage: NewMemoryStorage(),
		cache:   make(map[string]string),
	}
}

// Process handles input and returns output.
func (s *Service) Process(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("empty input")
	}
	if cached, ok := s.cache[input]; ok {
		return cached, nil
	}
	transformed := strings.ToUpper(input)
	if err := s.storage.Save(input, transformed); err != nil {
		return "", fmt.Errorf("save: %w", err)
	}
	s.cache[input] = transformed
	return transformed, nil
}
