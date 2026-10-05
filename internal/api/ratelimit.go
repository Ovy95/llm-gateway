package api

import (
	"sync"

	"golang.org/x/time/rate"
)

const defaultRPM = 5

func (a *API) rpmFor(key string) int {
	rpm := a.validKeys[key].RPM
	if rpm <= 0 {
		rpm = defaultRPM
	}
	return rpm
}

func (a *API) limiterFor(key string) *rate.Limiter {
	a.mu.Lock()
	defer a.mu.Unlock()

	limiter, ok := a.limiters[key]
	if !ok {
		rpm := a.rpmFor(key)
		limiter = rate.NewLimiter(rate.Limit(float64(rpm)/60.0), rpm) // burst = rpm
		a.limiters[key] = limiter
	}
	return limiter
}

// budgetMuFor returns the per-key lock that serializes a key's
// spend-check-then-record section, so two concurrent requests on the same
// key can't both read SpendUSD as under-budget before either one's Record
// lands.
func (a *API) budgetMuFor(key string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()

	bm, ok := a.budgetLocks[key]
	if !ok {
		bm = &sync.Mutex{}
		a.budgetLocks[key] = bm
	}
	return bm
}
