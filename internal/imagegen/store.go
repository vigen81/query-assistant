package imagegen

import (
	"fmt"
	"sync"
	"time"

	"gitlab.smartbet.am/golang/query-assistant/internal/models"
)

// GenerationStore is a thread-safe, in-memory registry of banner generation jobs.
// For a production deployment this can be replaced with a Redis/DB-backed store
// by implementing the same interface.
type GenerationStore struct {
	mu      sync.RWMutex
	entries map[string]*models.BannerGenerationResult
}

// NewGenerationStore creates an empty store.
func NewGenerationStore() *GenerationStore {
	return &GenerationStore{
		entries: make(map[string]*models.BannerGenerationResult),
	}
}

// Create inserts a new (in-progress) generation entry.
func (s *GenerationStore) Create(result *models.BannerGenerationResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[result.GenerationID] = result
}

// Get retrieves a generation entry by ID.
func (s *GenerationStore) Get(id string) (*models.BannerGenerationResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.entries[id]
	if !ok {
		return nil, fmt.Errorf("generation %q not found", id)
	}
	// Return a shallow copy so callers can't mutate the stored pointer.
	cp := *r
	if len(r.Variants) > 0 {
		cp.Variants = make([]models.BannerVariant, len(r.Variants))
		copy(cp.Variants, r.Variants)
	}
	return &cp, nil
}

// MarkCompleted transitions a generation to the "completed" state.
func (s *GenerationStore) MarkCompleted(id string, variants []models.BannerVariant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.entries[id]; ok {
		r.Status = models.GenerationStatusCompleted
		r.Variants = variants
		r.UpdatedAt = time.Now()
	}
}

// MarkFailed transitions a generation to the "failed" state.
func (s *GenerationStore) MarkFailed(id, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.entries[id]; ok {
		r.Status = models.GenerationStatusFailed
		r.FailureReason = reason
		r.UpdatedAt = time.Now()
	}
}
