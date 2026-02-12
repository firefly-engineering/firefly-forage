package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
)

// MockRuntime is a mock implementation of Runtime for testing
type MockRuntime struct {
	mu sync.RWMutex

	// Containers tracks the state of mock containers
	Containers map[string]*ContainerInfo

	// ExecResults maps container names to predefined exec results
	ExecResults map[string]*ExecResult

	// Errors allows injecting errors for specific operations
	Errors map[string]error

	// CallLog records all method calls for verification
	CallLog []MockCall

	// SandboxContainerInfo is returned by ContainerInfo()
	SandboxContainerInfoValue SandboxContainerInfo

	// GeneratedFileMounter handles MountGeneratedFile calls
	GeneratedFileMounter GeneratedFileMounter
}

// MockCall represents a recorded method call
type MockCall struct {
	Method string
	Args   []interface{}
}

// NewMockRuntime creates a new mock runtime
func NewMockRuntime() *MockRuntime {
	return &MockRuntime{
		Containers:  make(map[string]*ContainerInfo),
		ExecResults: make(map[string]*ExecResult),
		Errors:      make(map[string]error),
		CallLog:     make([]MockCall, 0),
	}
}

func (m *MockRuntime) record(method string, args ...interface{}) {
	m.CallLog = append(m.CallLog, MockCall{Method: method, Args: args})
}

// SetError sets an error to be returned for a specific operation
func (m *MockRuntime) SetError(operation string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Errors[operation] = err
}

// SetExecResult sets the result for exec operations on a container
func (m *MockRuntime) SetExecResult(name string, result *ExecResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ExecResults[name] = result
}

// AddContainer adds a container to the mock
func (m *MockRuntime) AddContainer(name string, status ContainerStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Containers[name] = &ContainerInfo{
		Name:   name,
		Status: status,
	}
}

// GetCalls returns all recorded calls
func (m *MockRuntime) GetCalls() []MockCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	calls := make([]MockCall, len(m.CallLog))
	copy(calls, m.CallLog)
	return calls
}

// GetCallsFor returns all calls for a specific method
func (m *MockRuntime) GetCallsFor(method string) []MockCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var calls []MockCall
	for _, call := range m.CallLog {
		if call.Method == method {
			calls = append(calls, call)
		}
	}
	return calls
}

// Reset clears all state
func (m *MockRuntime) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Containers = make(map[string]*ContainerInfo)
	m.ExecResults = make(map[string]*ExecResult)
	m.Errors = make(map[string]error)
	m.CallLog = make([]MockCall, 0)
}

// Name returns the runtime identifier
func (m *MockRuntime) Name() string {
	return "mock"
}

// Create creates a new container
func (m *MockRuntime) Create(ctx context.Context, opts CreateOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("Create", opts)

	if err, ok := m.Errors["Create"]; ok {
		return err
	}

	status := StatusStopped
	if opts.Start {
		status = StatusRunning
	}

	m.Containers[opts.Name] = &ContainerInfo{
		Name:   opts.Name,
		Status: status,
	}

	return nil
}

// Start starts an existing container
func (m *MockRuntime) Start(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("Start", name)

	if err, ok := m.Errors["Start"]; ok {
		return err
	}

	if container, ok := m.Containers[name]; ok {
		container.Status = StatusRunning
		return nil
	}

	return fmt.Errorf("container not found: %s", name)
}

// Stop stops a running container
func (m *MockRuntime) Stop(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("Stop", name)

	if err, ok := m.Errors["Stop"]; ok {
		return err
	}

	if container, ok := m.Containers[name]; ok {
		container.Status = StatusStopped
		return nil
	}

	return fmt.Errorf("container not found: %s", name)
}

// Destroy stops and removes a container
func (m *MockRuntime) Destroy(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("Destroy", name)

	if err, ok := m.Errors["Destroy"]; ok {
		return err
	}

	delete(m.Containers, name)
	return nil
}

// IsRunning checks if a container is currently running
func (m *MockRuntime) IsRunning(ctx context.Context, name string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.record("IsRunning", name)

	if err, ok := m.Errors["IsRunning"]; ok {
		return false, err
	}

	if container, ok := m.Containers[name]; ok {
		return container.Status == StatusRunning, nil
	}

	return false, nil
}

// Status returns detailed status of a container
func (m *MockRuntime) Status(ctx context.Context, name string) (*ContainerInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.record("Status", name)

	if err, ok := m.Errors["Status"]; ok {
		return nil, err
	}

	if container, ok := m.Containers[name]; ok {
		return container, nil
	}

	return &ContainerInfo{Name: name, Status: StatusNotFound}, nil
}

// Exec executes a command inside a container
func (m *MockRuntime) Exec(ctx context.Context, name string, command []string, opts ExecOptions) (*ExecResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.record("Exec", name, command, opts)

	if err, ok := m.Errors["Exec"]; ok {
		return nil, err
	}

	if result, ok := m.ExecResults[name]; ok {
		return result, nil
	}

	return &ExecResult{ExitCode: 0, Stdout: "", Stderr: ""}, nil
}

// ExecInteractive executes a command with an interactive TTY
func (m *MockRuntime) ExecInteractive(ctx context.Context, name string, command []string, opts ExecOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("ExecInteractive", name, command, opts)

	if err, ok := m.Errors["ExecInteractive"]; ok {
		return err
	}

	return nil
}

// List returns all containers managed by this runtime
func (m *MockRuntime) List(ctx context.Context) ([]*ContainerInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.record("List")

	if err, ok := m.Errors["List"]; ok {
		return nil, err
	}

	var containers []*ContainerInfo
	for _, container := range m.Containers {
		containers = append(containers, container)
	}

	return containers, nil
}

// ContainerInfo returns the sandbox container info for generated file paths.
func (m *MockRuntime) ContainerInfo() SandboxContainerInfo {
	if m.SandboxContainerInfoValue != (SandboxContainerInfo{}) {
		return m.SandboxContainerInfoValue
	}
	return DefaultContainerInfo()
}

// MountGeneratedFile stages a generated file for mounting into the container.
func (m *MockRuntime) MountGeneratedFile(ctx context.Context, sandboxName string, file injection.GeneratedFile) (injection.Mount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record("MountGeneratedFile", sandboxName, file)

	if err, ok := m.Errors["MountGeneratedFile"]; ok {
		return injection.Mount{}, err
	}

	if m.GeneratedFileMounter.StagingDir != "" {
		return m.GeneratedFileMounter.MountGeneratedFile(ctx, sandboxName, file)
	}

	// Default: return a simple mount without writing to disk
	return injection.Mount{
		HostPath:      fmt.Sprintf("/mock/staging/%s%s", sandboxName, file.ContainerPath),
		ContainerPath: file.ContainerPath,
		ReadOnly:      file.ReadOnly,
	}, nil
}

// Ensure MockRuntime implements Runtime and GeneratedFileRuntime
var (
	_ Runtime              = (*MockRuntime)(nil)
	_ GeneratedFileRuntime = (*MockRuntime)(nil)
)
