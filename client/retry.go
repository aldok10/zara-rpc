// Client-side retry policy: idempotent-only, bounded attempts, exponential
// backoff with jitter.
package client

import (
	"context"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/aldok10/zara-rpc/codes"
	"github.com/aldok10/zara-rpc/status"
)

// RetryPolicy configures client-side retries for a call. Retries apply only
// to idempotent methods (GET/HEAD, or any method when Idempotent is true)
// and only to transient failures (CodeUnavailable). Each attempt is bounded
// by the call context; backoff sleeps between attempts with optional jitter.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts including the first.
	// Defaults to 3.
	MaxAttempts int
	// InitialBackoff is the sleep before the first retry. Defaults to
	// 100ms.
	InitialBackoff time.Duration
	// MaxBackoff caps the backoff growth. Defaults to 1s.
	MaxBackoff time.Duration
	// BackoffMultiplier grows the backoff between attempts. Defaults to
	// 2.0.
	BackoffMultiplier float64
	// Jitter randomizes each backoff by +/- this fraction (0..1). Defaults
	// to 0.2.
	Jitter float64
	// Idempotent marks the call idempotent even for non-GET/HEAD methods.
	// Retrying a non-idempotent call can duplicate side effects; set this
	// only when the RPC is safe to repeat.
	Idempotent bool
}

// normalize fills zero fields with the documented defaults. It mutates the
// receiver; WithRetryPolicy copies before normalizing so the caller's
// policy is untouched.
func (p *RetryPolicy) normalize() {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.InitialBackoff <= 0 {
		p.InitialBackoff = 100 * time.Millisecond
	}
	if p.MaxBackoff <= 0 {
		p.MaxBackoff = time.Second
	}
	if p.BackoffMultiplier <= 0 {
		p.BackoffMultiplier = 2.0
	}
	if p.Jitter < 0 {
		p.Jitter = 0
	}
	if p.Jitter > 1 {
		p.Jitter = 1
	}
}

// retryInvoker wraps a UnaryInvoker with the retry policy. The wrapped
// invoker is the full interceptor chain, so interceptors observe each
// attempt.
func retryInvoker(policy *RetryPolicy, next UnaryInvoker) UnaryInvoker {
	return func(ctx context.Context, method, path string, body []byte, req, resp any, cfg *clientConfig) (int64, error) {
		idempotent := policy.Idempotent || method == http.MethodGet || method == http.MethodHead
		attempts := policy.MaxAttempts
		if attempts < 1 {
			attempts = 1
		}
		backoff := policy.InitialBackoff
		var lastBytes int64
		var lastErr error
		for attempt := 0; attempt < attempts; attempt++ {
			n, err := next(ctx, method, path, body, req, resp, cfg)
			lastBytes, lastErr = n, err
			if err == nil {
				return n, nil
			}
			// Retry only idempotent calls that failed transiently, and only
			// while attempts remain.
			if !idempotent || status.Code(err) != codes.CodeUnavailable || attempt == attempts-1 {
				return n, err
			}
			wait := backoff
			if policy.Jitter > 0 {
				// +/- jitter fraction around the base backoff.
				wait = time.Duration(float64(wait) * (1 - policy.Jitter + rand.Float64()*2*policy.Jitter))
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return n, ctx.Err()
			case <-timer.C:
			}
			backoff = time.Duration(float64(backoff) * policy.BackoffMultiplier)
			if backoff > policy.MaxBackoff {
				backoff = policy.MaxBackoff
			}
		}
		return lastBytes, lastErr
	}
}