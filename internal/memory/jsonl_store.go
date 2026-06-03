package memory

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"
)

// JSONLStore is a JSONL-backed Store implementation.
// Each line is a JSON-encoded Memory record. Deletes are soft
// (Deleted:true appended) and compacted on reload.
type JSONLStore struct {
	mu      sync.RWMutex
	path    string
	records map[string]*Memory // keyed by ID, deleted entries removed
	index   *BM25Index
}

// NewJSONLStore opens (or creates) the JSONL file at path and
// loads existing records into the in-memory index.
func NewJSONLStore(path string) (*JSONLStore, error) {
	s := &JSONLStore{
		path:    path,
		records: make(map[string]*Memory),
		index:   NewBM25Index(),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *JSONLStore) load() error {
	f, err := os.Open(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var m Memory
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue // skip malformed lines
		}
		if m.Deleted {
			delete(s.records, m.ID)
			s.index.Remove(m.ID)
		} else {
			s.records[m.ID] = &m
			s.index.Add(m.ID, m.Content)
		}
	}
	return sc.Err()
}

func (s *JSONLStore) append(m *Memory) error {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Remember stores a fact. It deduplicates by SHA and reconciles
// near-duplicate content in-place (see reconcile.go).
func (s *JSONLStore) Remember(content string, tags []string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sha := ContentSHA(content)

	// SHA dedup — exact same text already stored.
	for _, m := range s.records {
		if m.SHA == sha {
			return m.ID, nil
		}
	}

	// Mem0-style reconciliation — near-duplicate update in place.
	if existing := s.findNearDuplicate(content); existing != nil {
		existing.Content = content
		existing.Tags = mergeTags(existing.Tags, tags)
		existing.UpdatedAt = time.Now()
		existing.SHA = sha
		s.index.Add(existing.ID, content)
		return existing.ID, s.append(existing)
	}

	now := time.Now()
	m := &Memory{
		ID:        newID(),
		Content:   content,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
		SHA:       sha,
	}
	s.records[m.ID] = m
	s.index.Add(m.ID, content)
	return m.ID, s.append(m)
}

// Recall returns memories ranked by BM25 relevance.
func (s *JSONLStore) Recall(query string) ([]Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.index.Search(query, 10)
	out := make([]Memory, 0, len(ids))
	for _, id := range ids {
		if m, ok := s.records[id]; ok {
			out = append(out, *m)
		}
	}
	return out, nil
}

// Forget soft-deletes a memory by ID.
func (s *JSONLStore) Forget(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.records[id]
	if !ok {
		return fmt.Errorf("memory %q not found", id)
	}
	// Snapshot the record and mark deleted in the snapshot only.
	// Mutate the live state only after the append succeeds so that
	// a disk-full or other write error does not silently lose the record.
	snapshot := *m
	snapshot.Deleted = true
	snapshot.UpdatedAt = time.Now()
	if err := s.append(&snapshot); err != nil {
		return err
	}
	m.Deleted = true
	m.UpdatedAt = snapshot.UpdatedAt
	delete(s.records, id)
	s.index.Remove(id)
	return nil
}

// Close is a no-op for JSONL (writes are immediate).
func (s *JSONLStore) Close() error { return nil }
