package webapp

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"prods/internal/inquiries"
)

const maxTrackedRFQSources = 10000

type rfqRateLimitError struct {
	RetryAfter time.Duration
}

func (err rfqRateLimitError) Error() string {
	return fmt.Sprintf("RFQ submission rate limit reached; retry in %s", err.RetryAfter.Round(time.Second))
}

type rfqSourceWindow struct {
	expiresAt time.Time
	duration  time.Duration
	keys      map[string]struct{}
}

type rfqRateLimiter struct {
	mu        sync.Mutex
	sources   map[string]*rfqSourceWindow
	lastSweep time.Time
}

func newRFQRateLimiter() *rfqRateLimiter {
	return &rfqRateLimiter{sources: make(map[string]*rfqSourceWindow)}
}

func (limiter *rfqRateLimiter) allow(source, submissionKey string, now time.Time, limit int, window time.Duration) (bool, time.Duration) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if limiter.lastSweep.IsZero() || now.Sub(limiter.lastSweep) >= time.Minute || len(limiter.sources) >= maxTrackedRFQSources {
		for key, state := range limiter.sources {
			if !now.Before(state.expiresAt) {
				delete(limiter.sources, key)
			}
		}
		limiter.lastSweep = now
	}
	state := limiter.sources[source]
	if state == nil || !now.Before(state.expiresAt) || state.duration != window {
		if state == nil && len(limiter.sources) >= maxTrackedRFQSources {
			oldestKey := ""
			var oldest time.Time
			for key, candidate := range limiter.sources {
				if oldestKey == "" || candidate.expiresAt.Before(oldest) {
					oldestKey, oldest = key, candidate.expiresAt
				}
			}
			delete(limiter.sources, oldestKey)
		}
		state = &rfqSourceWindow{expiresAt: now.Add(window), duration: window, keys: make(map[string]struct{})}
		limiter.sources[source] = state
	}
	if _, replayCandidate := state.keys[submissionKey]; replayCandidate {
		return true, 0
	}
	if len(state.keys) >= limit {
		retryAfter := state.expiresAt.Sub(now)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter
	}
	state.keys[submissionKey] = struct{}{}
	return true, 0
}

func (s *Server) submitPublicRFQ(r *http.Request, submissionKey string, submission inquiries.Submission) (inquiries.Receipt, error) {
	receipt, found, err := s.store.LookupRFQReceipt(r.Context(), submissionKey, submission)
	if err != nil || found {
		return receipt, err
	}
	settings, err := s.store.TrafficSettings(r.Context())
	if err != nil {
		return inquiries.Receipt{}, err
	}
	allowed, retryAfter := s.rfqLimiter.allow(
		s.proxyTrust.clientSource(r), submissionKey, time.Now(), settings.RFQLimit, time.Duration(settings.RFQWindowSeconds)*time.Second,
	)
	if !allowed {
		return inquiries.Receipt{}, rfqRateLimitError{RetryAfter: retryAfter}
	}
	reservation, err := s.admitResource(r.Context(), "RFQ submission", s.config.DatabasePath, 1<<20, 0)
	if err != nil {
		return inquiries.Receipt{}, err
	}
	defer reservation.Release()
	return s.store.SubmitRFQ(r.Context(), submissionKey, submission)
}

func writeRFQRateLimit(w http.ResponseWriter, err rfqRateLimitError) {
	seconds := int64((err.RetryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", fmt.Sprintf("%d", seconds))
}
