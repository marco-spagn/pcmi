package middleware

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/time/rate"
)

type keyLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

var (
	rateLimiters   sync.Map
	rateLimitRPM   int
	rateLimitBurst int
)

func initRateLimitConfig() {
	if rateLimitRPM > 0 {
		return
	}
	if os.Getenv("RATE_LIMIT_DISABLED") == "true" || os.Getenv("RATE_LIMIT_DISABLED") == "1" {
		rateLimitRPM = 0
		return
	}
	rpm := 120
	if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rpm = n
		}
	}
	rateLimitRPM = rpm
	rateLimitBurst = rpm
	if b := os.Getenv("RATE_LIMIT_BURST"); b != "" {
		if n, err := strconv.Atoi(b); err == nil && n > 0 {
			rateLimitBurst = n
		}
	}
}

// RateLimitMiddleware applies per-API-key token-bucket rate limiting.
func RateLimitMiddleware() fiber.Handler {
	initRateLimitConfig()
	if rateLimitRPM == 0 {
		return func(c *fiber.Ctx) error { return c.Next() }
	}

	interval := time.Minute / time.Duration(rateLimitRPM)
	if interval <= 0 {
		interval = time.Millisecond
	}

	return func(c *fiber.Ctx) error {
		keyID, _ := c.Locals(APIKeyIDContextKey).(string)
		if keyID == "" {
			return c.Next()
		}

		lim := getLimiter(keyID, interval)
		if !lim.Allow() {
			return c.Status(429).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		}
		return c.Next()
	}
}

func getLimiter(keyID string, interval time.Duration) *rate.Limiter {
	if v, ok := rateLimiters.Load(keyID); ok {
		kl := v.(*keyLimiter)
		kl.lastSeen = time.Now()
		return kl.limiter
	}
	lim := rate.NewLimiter(rate.Every(interval), rateLimitBurst)
	kl := &keyLimiter{limiter: lim, lastSeen: time.Now()}
	actual, _ := rateLimiters.LoadOrStore(keyID, kl)
	return actual.(*keyLimiter).limiter
}
