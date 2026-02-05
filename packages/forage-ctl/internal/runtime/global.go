package runtime

import (
	"context"
	"sync"
)

var (
	globalRuntime Runtime
	globalMu      sync.RWMutex
	initOnce      sync.Once
)

// Global returns the global runtime instance.
// It initializes the runtime on first call using auto-detection.
func Global() Runtime {
	globalMu.RLock()
	if globalRuntime != nil {
		defer globalMu.RUnlock()
		return globalRuntime
	}
	globalMu.RUnlock()

	// Initialize with auto-detection
	initOnce.Do(func() {
		rt, err := New(nil)
		if err != nil {
			// Return nil - caller should handle
			return
		}
		SetGlobal(rt)
	})

	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalRuntime
}

// SetGlobal sets the global runtime instance.
// This should be called early in main() if you want to override auto-detection.
func SetGlobal(rt Runtime) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalRuntime = rt
}

// InitGlobal initializes the global runtime with the given config.
// Returns an error if runtime creation fails.
func InitGlobal(cfg *Config) error {
	rt, err := New(cfg)
	if err != nil {
		return err
	}
	SetGlobal(rt)
	return nil
}

// Helper functions that use the global runtime

// IsRunning checks if a container is running using the global runtime
func IsRunning(name string) bool {
	rt := Global()
	if rt == nil {
		return false
	}
	running, _ := rt.IsRunning(context.Background(), name)
	return running
}

// Destroy destroys a container using the global runtime
func Destroy(name string) error {
	rt := Global()
	if rt == nil {
		return nil
	}
	return rt.Destroy(context.Background(), name)
}

// Start starts a container using the global runtime
func Start(name string) error {
	rt := Global()
	if rt == nil {
		return nil
	}
	return rt.Start(context.Background(), name)
}

// Stop stops a container using the global runtime
func Stop(name string) error {
	rt := Global()
	if rt == nil {
		return nil
	}
	return rt.Stop(context.Background(), name)
}
