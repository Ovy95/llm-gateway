package api

import "golang.org/x/time/rate"

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
