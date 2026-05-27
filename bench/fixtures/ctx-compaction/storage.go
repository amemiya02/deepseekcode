package main

import "fmt"

// Storage defines persistence interface.
type Storage interface {
	Save(key, value string) error
	Load(key string) (string, error)
	Delete(key string) error
}

// MemoryStorage implements Storage with in-memory map.
type MemoryStorage struct {
	data map[string]string
}

// NewMemoryStorage creates a new MemoryStorage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{data: make(map[string]string)}
}

func (m *MemoryStorage) Save(key, value string) error {
	m.data[key] = value
	return nil
}

func (m *MemoryStorage) Load(key string) (string, error) {
	v, ok := m.data[key]
	if !ok {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return v, nil
}

func (m *MemoryStorage) Delete(key string) error {
	delete(m.data, key)
	return nil
}
