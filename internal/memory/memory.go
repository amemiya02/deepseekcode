package memory

import "time"

// Memory is a single stored fact.
type Memory struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Deleted   bool      `json:"deleted,omitempty"`
	SHA       string    `json:"sha,omitempty"`
}

// Store is the persistence + retrieval interface for long-term memory.
type Store interface {
	// Remember persists a fact and returns its ID.
	Remember(content string, tags []string) (id string, err error)
	// Recall returns memories ranked by relevance to the query.
	Recall(query string) ([]Memory, error)
	// Forget soft-deletes a memory by ID.
	Forget(id string) error
	// Close flushes and closes the store.
	Close() error
}
