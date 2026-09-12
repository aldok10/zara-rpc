package rbac

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/aldok10/zara-rpc/runtime"
)

// StaticAuthorizer enforces a fixed RBAC policy on every request. It is
// the zara-rpc counterpart of grpc-go's authz.StaticInterceptor.
type StaticAuthorizer struct {
	policy *Policy
}

// NewStatic builds a StaticAuthorizer from a policy JSON document.
func NewStatic(policyJSON string) (*StaticAuthorizer, error) {
	p, err := parsePolicy([]byte(policyJSON))
	if err != nil {
		return nil, err
	}
	return &StaticAuthorizer{policy: p}, nil
}

// IsAuthorized evaluates the policy against the request context.
func (a *StaticAuthorizer) IsAuthorized(ctx runtime.Ctx) bool {
	return a.policy.isAuthorized(ctx)
}

// FileWatcherAuthorizer watches a policy file and hot-reloads it on a
// refresh interval, mirroring grpc-go's authz.FileWatcherInterceptor. A
// failed reload keeps the previous policy; the current policy is swapped
// atomically so in-flight requests always see a consistent policy.
type FileWatcherAuthorizer struct {
	options FileWatcherOptions
	current atomic.Pointer[StaticAuthorizer]
	last    []byte
	cancel  context.CancelFunc
}

// FileWatcherOptions configures a FileWatcherAuthorizer.
type FileWatcherOptions struct {
	// PolicyFile is the path to a JSON authorization policy.
	PolicyFile string
	// RefreshDuration is the delay between policy refreshes.
	RefreshDuration time.Duration
	// OnPolicyUpdate is invoked synchronously after each successful
	// reload with the new policy JSON.
	OnPolicyUpdate func(string)
}

// NewFileWatcher returns a FileWatcherAuthorizer watching policyFile.
func NewFileWatcher(policyFile string, refresh time.Duration) (*FileWatcherAuthorizer, error) {
	return NewFileWatcherWithOptions(FileWatcherOptions{
		PolicyFile:      policyFile,
		RefreshDuration: refresh,
	})
}

// NewFileWatcherWithOptions returns a FileWatcherAuthorizer from options.
func NewFileWatcherWithOptions(options FileWatcherOptions) (*FileWatcherAuthorizer, error) {
	if options.PolicyFile == "" {
		return nil, fmt.Errorf("authz: policy file path is empty")
	}
	if options.RefreshDuration <= 0 {
		return nil, fmt.Errorf("authz: refresh duration must be > 0")
	}
	a := &FileWatcherAuthorizer{options: options}
	if err := a.reload(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	go a.run(ctx)
	return a, nil
}

// Close stops the background reload loop.
func (a *FileWatcherAuthorizer) Close() {
	a.cancel()
}

// IsAuthorized delegates to the current policy.
func (a *FileWatcherAuthorizer) IsAuthorized(ctx runtime.Ctx) bool {
	return a.current.Load().IsAuthorized(ctx)
}

func (a *FileWatcherAuthorizer) run(ctx context.Context) {
	ticker := time.NewTicker(a.options.RefreshDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// A failed reload keeps the previous policy.
			_ = a.reload()
		}
	}
}

func (a *FileWatcherAuthorizer) reload() error {
	data, err := os.ReadFile(a.options.PolicyFile)
	if err != nil {
		return fmt.Errorf("authz: read policy file: %w", err)
	}
	if bytes.Equal(a.last, data) {
		return nil
	}
	authorizer, err := NewStatic(string(data))
	if err != nil {
		return err
	}
	a.last = data
	a.current.Store(authorizer)
	if a.options.OnPolicyUpdate != nil {
		a.options.OnPolicyUpdate(string(data))
	}
	return nil
}
