package api

import "golang.org/x/time/rate"

const (
	requestsPerMinute = 5
	burstSize         = 5
	retryAfterSeconds = 60 / requestsPerMinute // = 12s, the bucket's refill interval
	defaultRPM        = 5
)

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
		limiter = rate.NewLimiter(rate.Limit(float64(requestsPerMinute)/60.0), burstSize)
		a.limiters[key] = limiter
	}
	return limiter
}
