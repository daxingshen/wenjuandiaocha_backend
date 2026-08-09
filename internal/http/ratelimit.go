// 极简内存令牌桶,按 key(IP)限频。本轮防滥用够用;分布式/持久化限频延后。
package http

import (
	"sync"
	"time"
)

type tokenBucket struct {
	tokens   float64
	last     time.Time
}

type limiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	rate     float64 // 每秒补充令牌
	capacity float64 // 桶容量(突发上限)
}

func newLimiter(ratePerSec, capacity float64) *limiter {
	return &limiter{buckets: map[string]*tokenBucket{}, rate: ratePerSec, capacity: capacity}
}

// allow 消耗一个令牌;无令牌返回 false。
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &tokenBucket{tokens: l.capacity - 1, last: now}
		return true
	}
	// 补充令牌
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// submitLimiter:提交答卷限频。每 IP 平均 1 次/秒,突发 10。
var submitLimiter = newLimiter(1, 10)
