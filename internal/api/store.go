package api

import "sync"

// UsageStore records and reports per-key Usage. Behind an interface so the
// proxy depends on behaviour, not a concrete store — swap in Postgres later
// without touching handler logic, and fake it in tests.
type UsageStore interface {
	Record(key string, tokens int, costUSD float64)
	SpendUSD(key string) float64
	CurrentUsage() map[string]Usage
}

type Usage struct {
	Requests int
	Tokens   int
	SpendUSD float64
}

type memoryStore struct {
	mu   sync.Mutex
	data map[string]Usage
}

func newMemoryStore() *memoryStore {
	return &memoryStore{data: make(map[string]Usage)}
}

func (s *memoryStore) Record(key string, tokens int, costUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.data[key]
	current.Requests++
	current.Tokens += tokens
	current.SpendUSD += costUSD
	s.data[key] = current
}

func (s *memoryStore) SpendUSD(key string) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[key].SpendUSD
}

func (s *memoryStore) CurrentUsage() map[string]Usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]Usage, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}
