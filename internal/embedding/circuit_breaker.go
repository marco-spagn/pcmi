package embedding

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/marco-spagn/pcmi/internal/metrics"
	"golang.org/x/time/rate"
)

// ErrCircuitOpen is returned when the embedding circuit breaker is open.
var ErrCircuitOpen = errors.New("embedding circuit breaker open")

const (
	defaultCBMaxFailures    uint32 = 5
	defaultCBBaseTimeout           = 30 * time.Second
	defaultCBMaxTimeout            = 5 * time.Minute
	defaultCBRateLimit             = 10.0
	defaultCBHalfOpenMax      uint32 = 1
)

// CircuitBreakerConfig tunes the embedding circuit breaker and outbound rate limit.
type CircuitBreakerConfig struct {
	MaxConsecutiveFailures uint32
	BaseOpenTimeout        time.Duration
	MaxOpenTimeout         time.Duration
	RateLimitPerSec        float64
	HalfOpenMaxRequests    uint32
}

// DefaultCircuitBreakerConfig returns production-oriented defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxConsecutiveFailures: defaultCBMaxFailures,
		BaseOpenTimeout:        defaultCBBaseTimeout,
		MaxOpenTimeout:         defaultCBMaxTimeout,
		RateLimitPerSec:        defaultCBRateLimit,
		HalfOpenMaxRequests:    defaultCBHalfOpenMax,
	}
}

// OpenBackoffDuration returns exponential backoff with multiplicative jitter for
// how long the circuit stays open before probing half-open. attempt 0 uses base;
// each subsequent open doubles the base (capped at max), then scales by 0.75–1.25.
func OpenBackoffDuration(attempt uint32, base, max time.Duration) time.Duration {
	if base <= 0 {
		base = defaultCBBaseTimeout
	}
	if max <= 0 {
		max = defaultCBMaxTimeout
	}
	shift := attempt
	if shift > 16 {
		shift = 16
	}
	d := base * time.Duration(1<<shift)
	if d > max {
		d = max
	}
	jitter := 0.75 + rand.Float64()*0.5
	out := time.Duration(float64(d) * jitter)
	if out > max {
		return max
	}
	return out
}

// CircuitBreakerProvider wraps a Provider with gobreaker, rate limiting, and metrics.
type CircuitBreakerProvider struct {
	inner Provider
	cfg   CircuitBreakerConfig

	mu       sync.RWMutex
	cb       *gobreaker.CircuitBreaker[[]float32]
	limiter  *rate.Limiter
	openGen  uint32
	openHold time.Time
}

// WrapWithCircuitBreaker returns inner wrapped with circuit breaker and rate limit.
func WrapWithCircuitBreaker(inner Provider, cfg CircuitBreakerConfig) Provider {
	if inner == nil {
		return nil
	}
	p := &CircuitBreakerProvider{
		inner:   inner,
		cfg:     cfg,
		limiter: rate.NewLimiter(rate.Limit(cfg.RateLimitPerSec), int(cfg.RateLimitPerSec)+1),
	}
	p.cb = p.newBreaker()
	metrics.SetEmbeddingCircuitState(metrics.EmbeddingCircuitClosed)
	return p
}

func (p *CircuitBreakerProvider) newBreaker() *gobreaker.CircuitBreaker[[]float32] {
	maxFail := p.cfg.MaxConsecutiveFailures
	if maxFail == 0 {
		maxFail = defaultCBMaxFailures
	}
	halfOpenMax := p.cfg.HalfOpenMaxRequests
	if halfOpenMax == 0 {
		halfOpenMax = defaultCBHalfOpenMax
	}
	timeout := p.cfg.BaseOpenTimeout
	if timeout <= 0 {
		timeout = defaultCBBaseTimeout
	}

	return gobreaker.NewCircuitBreaker[[]float32](gobreaker.Settings{
		Name:        "embedding",
		MaxRequests: halfOpenMax,
		Timeout:     timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= maxFail
		},
		OnStateChange: func(_ string, from, to gobreaker.State) {
			p.onStateChange(from, to)
		},
	})
}

func (p *CircuitBreakerProvider) onStateChange(from, to gobreaker.State) {
	switch to {
	case gobreaker.StateClosed:
		p.mu.Lock()
		p.openGen = 0
		p.openHold = time.Time{}
		p.mu.Unlock()
		metrics.SetEmbeddingCircuitState(metrics.EmbeddingCircuitClosed)
	case gobreaker.StateHalfOpen:
		p.mu.Lock()
		p.openHold = time.Time{}
		p.mu.Unlock()
		metrics.SetEmbeddingCircuitState(metrics.EmbeddingCircuitHalfOpen)
	case gobreaker.StateOpen:
		p.mu.Lock()
		p.openGen++
		hold := OpenBackoffDuration(p.openGen-1, p.cfg.BaseOpenTimeout, p.cfg.MaxOpenTimeout)
		p.openHold = time.Now().Add(hold)
		p.mu.Unlock()
		metrics.SetEmbeddingCircuitState(metrics.EmbeddingCircuitOpen)
		_ = from
	}
}

func (p *CircuitBreakerProvider) circuitBlocked() bool {
	if p.cb.State() != gobreaker.StateOpen {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return !p.openHold.IsZero() && time.Now().Before(p.openHold)
}

// Generate calls the inner provider through the circuit breaker and rate limiter.
func (p *CircuitBreakerProvider) Generate(ctx context.Context, text string) ([]float32, error) {
	if err := p.limiter.Wait(ctx); err != nil {
		metrics.IncEmbeddingRequest(metrics.EmbeddingResultRateLimited)
		return nil, fmt.Errorf("embedding rate limit: %w", err)
	}

	if p.circuitBlocked() {
		metrics.IncEmbeddingRequest(metrics.EmbeddingResultCircuitOpen)
		return nil, ErrCircuitOpen
	}

	start := time.Now()
	vec, err := p.cb.Execute(func() ([]float32, error) {
		return p.inner.Generate(ctx, text)
	})
	metrics.ObserveEmbeddingLatency(time.Since(start).Seconds())

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			metrics.IncEmbeddingRequest(metrics.EmbeddingResultCircuitOpen)
			return nil, ErrCircuitOpen
		}
		metrics.IncEmbeddingRequest(metrics.EmbeddingResultError)
		return nil, err
	}
	metrics.IncEmbeddingRequest(metrics.EmbeddingResultSuccess)
	return vec, nil
}

// State exposes the current gobreaker state (for tests).
func (p *CircuitBreakerProvider) State() gobreaker.State {
	return p.cb.State()
}

// OpenGeneration returns how many times the circuit has opened (for tests).
func (p *CircuitBreakerProvider) OpenGeneration() uint32 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.openGen
}
