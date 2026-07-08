package llmgateway

import (
	"sync"
	"time"
)

// tenantRateLimiter 是按租户隔离的令牌桶限流器。
type tenantRateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	capacity float64
	refill   float64 // 每秒补充令牌数
	now      func() time.Time
}

type tokenBucket struct {
	tokens   float64
	lastFill time.Time
}

// newTenantRateLimiter 创建限流器,ratePerMinute <= 0 时不限流。
func newTenantRateLimiter(ratePerMinute int, now func() time.Time) *tenantRateLimiter {
	if now == nil {
		now = time.Now
	}
	if ratePerMinute <= 0 {
		return nil
	}
	return &tenantRateLimiter{
		buckets:  make(map[string]*tokenBucket),
		capacity: float64(ratePerMinute),
		refill:   float64(ratePerMinute) / 60,
		now:      now,
	}
}

// Allow 尝试为租户获取一个令牌。
func (l *tenantRateLimiter) Allow(tenantID string) bool {
	if l == nil {
		return true
	}
	if tenantID == "" {
		tenantID = "default"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	current := l.now()
	bucket, ok := l.buckets[tenantID]
	if !ok {
		bucket = &tokenBucket{tokens: l.capacity, lastFill: current}
		l.buckets[tenantID] = bucket
	}
	elapsed := current.Sub(bucket.lastFill).Seconds()
	if elapsed > 0 {
		bucket.tokens += elapsed * l.refill
		if bucket.tokens > l.capacity {
			bucket.tokens = l.capacity
		}
		bucket.lastFill = current
	}
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}
