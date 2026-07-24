package api

import "golang.org/x/time/rate"

func (a *API) limiterFor(key string) *rate.Limiter {
	a.mu.Lock()
	defer a.mu.Unlock()

	limiter, ok := a.limiters[key]
	if !ok {
		limiter = rate.NewLimiter(rate.Limit(5.0/60.0), 5) // 5 req/min, burst 5
		a.limiters[key] = limiter
	}
	return limiter
}
