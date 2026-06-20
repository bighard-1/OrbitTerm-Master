package middleware

import (
	"net/http"
	"sync"
	"time"

	"orbitterm-server/internal/common"

	"github.com/gin-gonic/gin"
)

type rateLimitBucket struct {
	windowStart time.Time
	count       int
}

// IPRateLimit 是轻量级内存限流器，适合保护认证与管理接口免受误触或低强度暴力请求。
// 多实例部署时仍建议在 1Panel/Nginx/网关层增加统一限流。
func IPRateLimit(maxRequests int, window time.Duration) gin.HandlerFunc {
	if maxRequests <= 0 {
		maxRequests = 120
	}
	if window <= 0 {
		window = time.Minute
	}

	var mu sync.Mutex
	buckets := make(map[string]rateLimitBucket)
	lastCleanup := time.Now()

	return func(ctx *gin.Context) {
		now := time.Now()
		key := ctx.ClientIP()

		mu.Lock()
		if now.Sub(lastCleanup) >= window {
			pruneExpiredRateLimitBuckets(buckets, now, window)
			lastCleanup = now
		}
		bucket := buckets[key]
		if bucket.windowStart.IsZero() || now.Sub(bucket.windowStart) >= window {
			bucket = rateLimitBucket{windowStart: now}
		}
		bucket.count++
		buckets[key] = bucket
		allowed := bucket.count <= maxRequests
		mu.Unlock()

		if !allowed {
			common.Error(ctx, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

func pruneExpiredRateLimitBuckets(buckets map[string]rateLimitBucket, now time.Time, window time.Duration) {
	for key, bucket := range buckets {
		if bucket.windowStart.IsZero() || now.Sub(bucket.windowStart) >= window {
			delete(buckets, key)
		}
	}
}
