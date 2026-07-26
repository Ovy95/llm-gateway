package api

import "sync"

// UsageStore records and reports per-key usage. Behind an interface so the
// proxy depends on behaviour, not a concrete store — swap in Postgres later
// without touching handler logic, and fake it in tests.
type UsageStore interface {
	Record(key string, tokens int, costUSD float64)
	SpendUSD(key string) float64
}

type usage struct {
	requests int
	tokens   int
	spendUSD float64
}

type memoryStore struct {
	mu   sync.Mutex
	data map[string]usage // key -> its usage
}

func newMemoryStore() *memoryStore {
	return &memoryStore{data: make(map[string]usage)}
}

func (s *memoryStore) Record(key string, tokens int, costUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.data[key]
	current.requests++
	current.tokens += tokens
	current.spendUSD += costUSD
	s.data[key] = current
}

func (s *memoryStore) SpendUSD(key string) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[key].spendUSD
}
