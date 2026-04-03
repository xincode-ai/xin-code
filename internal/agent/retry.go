// internal/agent/retry.go
// 重试机制：指数退避 + 429/529 处理
// CC 参考：src/services/api/withRetry.ts
package agent

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"
)

// CC 对标常量
const (
	DefaultMaxRetries = 10
	BaseDelayMs       = 500
	MaxDelayMs        = 32000 // 32s cap
	Max529Retries     = 3
	JitterFraction    = 0.25 // ±25%
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries int
	OnRetry    func(attempt int, err error, delay time.Duration) // 通知调用方
}

// DefaultRetryConfig 返回 CC 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: DefaultMaxRetries,
	}
}

// APIError 包含 HTTP 状态码的 API 错误
type APIError struct {
	StatusCode int
	Message    string
	RetryAfter string // Retry-After 头值
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// WithRetry 包装一个返回 error 的操作，添加重试逻辑
// CC 参考：src/services/api/withRetry.ts
func WithRetry(
	ctx context.Context,
	cfg RetryConfig,
	operation func(ctx context.Context, attempt int) error,
) error {
	consecutive529 := 0

	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		err := operation(ctx, attempt)
		if err == nil {
			return nil
		}

		// 检查上下文取消
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 提取 API 错误信息
		apiErr, isAPI := err.(*APIError)
		if !isAPI || !IsRetryableStatusCode(apiErr.StatusCode) {
			return err // 不可重试的错误
		}

		// 529 连续计数（CC: max529Retries = 3）
		if apiErr.StatusCode == 529 {
			consecutive529++
			if consecutive529 >= Max529Retries {
				return fmt.Errorf("API 过载（连续 %d 次 529 错误）: %w", consecutive529, err)
			}
		} else {
			consecutive529 = 0
		}

		// 最后一次重试失败
		if attempt >= cfg.MaxRetries {
			return fmt.Errorf("重试 %d 次后仍失败: %w", cfg.MaxRetries, err)
		}

		// 计算退避延迟
		delay := CalcRetryDelay(attempt, apiErr.RetryAfter)

		// 通知回调
		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt, err, delay)
		}

		// 等待
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("exceeded max retries")
}

// CalcRetryDelay 计算重试延迟
// CC 公式：base * 2^(attempt-1) + jitter，cap at 32s
func CalcRetryDelay(attempt int, retryAfter string) time.Duration {
	// 指数退避
	baseMs := float64(BaseDelayMs) * math.Pow(2, float64(attempt-1))
	if baseMs > float64(MaxDelayMs) {
		baseMs = float64(MaxDelayMs)
	}

	// ±25% jitter
	jitter := baseMs * JitterFraction * (2*rand.Float64() - 1)
	delayMs := baseMs + jitter

	// Retry-After 头优先
	if retryAfter != "" {
		if seconds, err := strconv.ParseFloat(retryAfter, 64); err == nil {
			headerMs := seconds * 1000
			if headerMs > delayMs {
				delayMs = headerMs
			}
		}
	}

	return time.Duration(delayMs) * time.Millisecond
}

// IsRetryableStatusCode 判断 HTTP 状态码是否可重试
func IsRetryableStatusCode(code int) bool {
	switch code {
	case 408, 409, 429, 529:
		return true
	default:
		return code >= 500
	}
}
