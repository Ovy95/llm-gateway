package api

import (
	"context"

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

// keyMutex is a mutex whose Lock can be aborted by a context. A plain
// sync.Mutex has no way to give up on a wait, so a request queued behind a
// slow call on the same key would keep its goroutine (and the underlying
// connection) blocked for the full wait even after its client disconnected.
// The zero value is not usable; construct with newKeyMutex.
type keyMutex chan struct{}

func newKeyMutex() keyMutex {
	m := make(keyMutex, 1)
	m <- struct{}{}
	return m
}

// Lock blocks until the mutex is free or ctx is done, whichever happens
// first. On success (nil error) the caller owns the mutex and must call
// Unlock exactly once; on error the mutex was not acquired and must not be
// unlocked.
func (m keyMutex) Lock(ctx context.Context) error {
	select {
	case <-m:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m keyMutex) Unlock() {
	m <- struct{}{}
}

// budgetMuFor returns the per-key lock that serialises a key's
// spend-check-then-record section, so two concurrent requests on the same
// key can't both read SpendUSD as under-budget before either one's Record
// lands.
func (a *API) budgetMuFor(key string) keyMutex {
	a.mu.Lock()
	defer a.mu.Unlock()

	bm, ok := a.budgetLocks[key]
	if !ok {
		bm = newKeyMutex()
		a.budgetLocks[key] = bm
	}
	return bm
}
