package system

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
	"time"
)

// MockFS implements FileSystem for testing.
type MockFS struct {
	mu    sync.RWMutex
	files map[string]*mockFile
	dirs  map[string]bool

	// Error injection
	ReadFileErr   error
	WriteFileErr  error
	RemoveErr     error
	RemoveAllErr  error
	StatErr       error
	MkdirAllErr   error
	ReadDirErr    error
	CopyFileErr   error
}

type mockFile struct {
	data []byte
	mode fs.FileMode
}

// NewMockFS creates a new MockFS with an empty filesystem.
func NewMockFS() *MockFS {
	return &MockFS{
		files: make(map[string]*mockFile),
		dirs:  make(map[string]bool),
	}
}

// AddFile adds a file to the mock filesystem.
func (m *MockFS) AddFile(path string, data []byte, mode fs.FileMode) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = &mockFile{data: data, mode: mode}
	// Ensure parent directories exist
	dir := filepath.Dir(path)
	for dir != "." && dir != "/" {
		m.dirs[dir] = true
		dir = filepath.Dir(dir)
	}
}

// AddDir adds a directory to the mock filesystem.
func (m *MockFS) AddDir(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirs[path] = true
}

// GetFile returns the contents of a file in the mock filesystem.
func (m *MockFS) GetFile(path string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[path]
	if !ok {
		return nil, false
	}
	return f.data, true
}

func (m *MockFS) ReadFile(path string) ([]byte, error) {
	if m.ReadFileErr != nil {
		return nil, m.ReadFileErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return f.data, nil
}

func (m *MockFS) WriteFile(path string, data []byte, perm fs.FileMode) error {
	if m.WriteFileErr != nil {
		return m.WriteFileErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = &mockFile{data: data, mode: perm}
	return nil
}

func (m *MockFS) Remove(path string) error {
	if m.RemoveErr != nil {
		return m.RemoveErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[path]; ok {
		delete(m.files, path)
		return nil
	}
	if _, ok := m.dirs[path]; ok {
		delete(m.dirs, path)
		return nil
	}
	return fs.ErrNotExist
}

func (m *MockFS) RemoveAll(path string) error {
	if m.RemoveAllErr != nil {
		return m.RemoveAllErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove all files and dirs with this prefix
	for p := range m.files {
		if p == path || hasPathPrefix(p, path) {
			delete(m.files, p)
		}
	}
	for p := range m.dirs {
		if p == path || hasPathPrefix(p, path) {
			delete(m.dirs, p)
		}
	}
	return nil
}

func (m *MockFS) Stat(path string) (fs.FileInfo, error) {
	if m.StatErr != nil {
		return nil, m.StatErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if f, ok := m.files[path]; ok {
		return &mockFileInfo{name: filepath.Base(path), size: int64(len(f.data)), mode: f.mode}, nil
	}
	if _, ok := m.dirs[path]; ok {
		return &mockFileInfo{name: filepath.Base(path), isDir: true, mode: fs.ModeDir | 0755}, nil
	}
	return nil, fs.ErrNotExist
}

func (m *MockFS) MkdirAll(path string, perm fs.FileMode) error {
	if m.MkdirAllErr != nil {
		return m.MkdirAllErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create all directories in the path
	current := path
	for current != "." && current != "/" {
		m.dirs[current] = true
		current = filepath.Dir(current)
	}
	return nil
}

func (m *MockFS) Exists(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, fileOk := m.files[path]
	_, dirOk := m.dirs[path]
	return fileOk || dirOk
}

func (m *MockFS) IsDir(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.dirs[path]
	return ok
}

func (m *MockFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if m.ReadDirErr != nil {
		return nil, m.ReadDirErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.dirs[path]; !ok {
		// Check if it's the root or a path that has children
		hasChildren := false
		for p := range m.files {
			if hasPathPrefix(p, path) {
				hasChildren = true
				break
			}
		}
		if !hasChildren {
			return nil, fs.ErrNotExist
		}
	}

	entries := make(map[string]fs.DirEntry)

	// Find direct children
	for p, f := range m.files {
		if dir := filepath.Dir(p); dir == path {
			name := filepath.Base(p)
			entries[name] = &mockDirEntry{name: name, mode: f.mode}
		}
	}
	for p := range m.dirs {
		if dir := filepath.Dir(p); dir == path {
			name := filepath.Base(p)
			entries[name] = &mockDirEntry{name: name, isDir: true, mode: fs.ModeDir | 0755}
		}
	}

	result := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, e)
	}
	return result, nil
}

func (m *MockFS) CopyFile(src, dst string) error {
	if m.CopyFileErr != nil {
		return m.CopyFileErr
	}
	data, err := m.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := m.Stat(src)
	if err != nil {
		return err
	}
	return m.WriteFile(dst, data, info.Mode())
}

// hasPathPrefix checks if path has the given prefix as a path component.
func hasPathPrefix(path, prefix string) bool {
	if len(path) <= len(prefix) {
		return false
	}
	return path[:len(prefix)] == prefix && path[len(prefix)] == '/'
}

// mockFileInfo implements fs.FileInfo for testing.
type mockFileInfo struct {
	name  string
	size  int64
	mode  fs.FileMode
	isDir bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() fs.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() interface{}   { return nil }

// mockDirEntry implements fs.DirEntry for testing.
type mockDirEntry struct {
	name  string
	mode  fs.FileMode
	isDir bool
}

func (m *mockDirEntry) Name() string               { return m.name }
func (m *mockDirEntry) IsDir() bool                { return m.isDir }
func (m *mockDirEntry) Type() fs.FileMode          { return m.mode.Type() }
func (m *mockDirEntry) Info() (fs.FileInfo, error) { return &mockFileInfo{name: m.name, mode: m.mode, isDir: m.isDir}, nil }

// MockExecutor implements CommandExecutor for testing.
type MockExecutor struct {
	mu sync.Mutex

	// Commands records all executed commands for verification.
	Commands []MockCommand

	// Responses maps command patterns to responses.
	// Key format: "command arg1 arg2..."
	Responses map[string]MockResponse

	// DefaultResponse is used when no matching response is found.
	DefaultResponse MockResponse

	// InteractiveErr is returned by ExecuteInteractive if set.
	InteractiveErr error

	// ReplaceProcessErr is returned by ReplaceProcess if set.
	ReplaceProcessErr error
}

// MockCommand records an executed command.
type MockCommand struct {
	Name  string
	Args  []string
	Stdin string
}

// MockResponse defines the response for a command.
type MockResponse struct {
	Output []byte
	Err    error
}

// NewMockExecutor creates a new MockExecutor.
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		Commands:  make([]MockCommand, 0),
		Responses: make(map[string]MockResponse),
	}
}

// AddResponse adds a response for a specific command pattern.
func (m *MockExecutor) AddResponse(pattern string, output []byte, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses[pattern] = MockResponse{Output: output, Err: err}
}

func (m *MockExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Commands = append(m.Commands, MockCommand{Name: name, Args: args})

	// Look for matching response
	key := name
	if len(args) > 0 {
		key = name + " " + args[0]
	}

	if resp, ok := m.Responses[key]; ok {
		return resp.Output, resp.Err
	}
	if resp, ok := m.Responses[name]; ok {
		return resp.Output, resp.Err
	}

	return m.DefaultResponse.Output, m.DefaultResponse.Err
}

func (m *MockExecutor) ExecuteWithStdin(ctx context.Context, stdin string, name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Commands = append(m.Commands, MockCommand{Name: name, Args: args, Stdin: stdin})

	key := name
	if len(args) > 0 {
		key = name + " " + args[0]
	}

	if resp, ok := m.Responses[key]; ok {
		return resp.Output, resp.Err
	}
	if resp, ok := m.Responses[name]; ok {
		return resp.Output, resp.Err
	}

	return m.DefaultResponse.Output, m.DefaultResponse.Err
}

func (m *MockExecutor) ExecuteInteractive(ctx context.Context, name string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Commands = append(m.Commands, MockCommand{Name: name, Args: args})

	if m.InteractiveErr != nil {
		return m.InteractiveErr
	}
	return nil
}

func (m *MockExecutor) ReplaceProcess(name string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Commands = append(m.Commands, MockCommand{Name: name, Args: args})

	if m.ReplaceProcessErr != nil {
		return m.ReplaceProcessErr
	}
	// In tests, we can't actually replace the process, so just return an error
	// that indicates this was called
	return errors.New("mock: ReplaceProcess called (would exec in real implementation)")
}

// LastCommand returns the most recently executed command.
func (m *MockExecutor) LastCommand() (MockCommand, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Commands) == 0 {
		return MockCommand{}, false
	}
	return m.Commands[len(m.Commands)-1], true
}

// Reset clears all recorded commands.
func (m *MockExecutor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Commands = make([]MockCommand, 0)
}
