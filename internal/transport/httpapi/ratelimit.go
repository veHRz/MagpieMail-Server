package httpapi

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/veHRz/MagpieMail-Server/internal/config"
)

// limiter keeps one token bucket per key (client address or user), in memory.
// Several server instances would each count separately: sharing the buckets
// needs a common store (see ADR 0025).
type limiter struct {
	rate  rate.Limit
	burst int
	// idle is how long an unused bucket is kept. It is never shorter than the
	// time a bucket takes to refill, so forgetting one never grants extra tokens.
	idle time.Duration
	now  func() time.Time

	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	tokens   *rate.Limiter
	lastSeen time.Time
}

const sweepInterval = time.Minute

func newLimiter(cfg config.LimitConfig) *limiter {
	refill := time.Duration(float64(cfg.Burst) / cfg.Rate * float64(time.Second))
	return &limiter{
		rate:    rate.Limit(cfg.Rate),
		burst:   cfg.Burst,
		idle:    max(10*time.Minute, refill),
		now:     time.Now,
		buckets: make(map[string]*bucket),
	}
}

// take consumes a token for key. When none is left it returns false and how
// long until the next one.
func (l *limiter) take(key string) (time.Duration, bool) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sweep(now)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: rate.NewLimiter(l.rate, l.burst)}
		l.buckets[key] = b
	}
	b.lastSeen = now

	reservation := b.tokens.ReserveN(now, 1)
	if delay := reservation.DelayFrom(now); delay > 0 {
		reservation.CancelAt(now)
		return delay, false
	}
	return 0, true
}

// sweep forgets idle buckets, at most once per sweepInterval.
func (l *limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	for key, b := range l.buckets {
		if now.Sub(b.lastSeen) > l.idle {
			delete(l.buckets, key)
		}
	}
	l.lastSweep = now
}

// rateLimit rejects requests whose bucket is empty with 429 and a Retry-After
// header. key returns false for requests the limiter does not apply to.
func rateLimit(l *limiter, key func(*http.Request) (string, bool)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k, ok := key(r)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			if wait, allowed := l.take(k); !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(wait.Seconds())))))
				writeProblem(w, r, http.StatusTooManyRequests, detailRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// probes are polled by orchestrators and never rate limited.
var probes = map[string]bool{"/healthz": true, "/readyz": true}
