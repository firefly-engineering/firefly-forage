package system

import (
	"context"
	"io/fs"
	"testing"
)

func TestMockFS_ReadWriteFile(t *testing.T) {
	mockFS := NewMockFS()

	// Write a file
	content := []byte("hello world")
	err := mockFS.WriteFile("/test/file.txt", content, 0644)
	if err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	// Read it back
	data, err := mockFS.ReadFile("/test/file.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	if string(data) != "hello world" {
		t.Errorf("ReadFile = %q, want %q", string(data), "hello world")
	}
}

func TestMockFS_ReadFile_NotExists(t *testing.T) {
	mockFS := NewMockFS()

	_, err := mockFS.ReadFile("/nonexistent")
	if err != fs.ErrNotExist {
		t.Errorf("ReadFile error = %v, want fs.ErrNotExist", err)
	}
}

func TestMockFS_Stat(t *testing.T) {
	mockFS := NewMockFS()
	mockFS.AddFile("/test/file.txt", []byte("content"), 0644)
	mockFS.AddDir("/test/dir")

	// Stat file
	info, err := mockFS.Stat("/test/file.txt")
	if err != nil {
		t.Fatalf("Stat file error: %v", err)
	}
	if info.IsDir() {
		t.Error("File should not be a directory")
	}
	if info.Name() != "file.txt" {
		t.Errorf("Name = %q, want %q", info.Name(), "file.txt")
	}

	// Stat directory
	info, err = mockFS.Stat("/test/dir")
	if err != nil {
		t.Fatalf("Stat dir error: %v", err)
	}
	if !info.IsDir() {
		t.Error("Dir should be a directory")
	}
}

func TestMockFS_Exists(t *testing.T) {
	mockFS := NewMockFS()
	mockFS.AddFile("/file.txt", []byte("x"), 0644)
	mockFS.AddDir("/dir")

	if !mockFS.Exists("/file.txt") {
		t.Error("File should exist")
	}
	if !mockFS.Exists("/dir") {
		t.Error("Dir should exist")
	}
	if mockFS.Exists("/nonexistent") {
		t.Error("Nonexistent should not exist")
	}
}

func TestMockFS_IsDir(t *testing.T) {
	mockFS := NewMockFS()
	mockFS.AddFile("/file.txt", []byte("x"), 0644)
	mockFS.AddDir("/dir")

	if mockFS.IsDir("/file.txt") {
		t.Error("File should not be a directory")
	}
	if !mockFS.IsDir("/dir") {
		t.Error("Dir should be a directory")
	}
}

func TestMockFS_Remove(t *testing.T) {
	mockFS := NewMockFS()
	mockFS.AddFile("/file.txt", []byte("x"), 0644)

	if err := mockFS.Remove("/file.txt"); err != nil {
		t.Fatalf("Remove error: %v", err)
	}

	if mockFS.Exists("/file.txt") {
		t.Error("File should be removed")
	}
}

func TestMockFS_RemoveAll(t *testing.T) {
	mockFS := NewMockFS()
	mockFS.AddFile("/dir/file1.txt", []byte("x"), 0644)
	mockFS.AddFile("/dir/file2.txt", []byte("y"), 0644)
	mockFS.AddDir("/dir/subdir")

	if err := mockFS.RemoveAll("/dir"); err != nil {
		t.Fatalf("RemoveAll error: %v", err)
	}

	if mockFS.Exists("/dir/file1.txt") {
		t.Error("File1 should be removed")
	}
	if mockFS.Exists("/dir/file2.txt") {
		t.Error("File2 should be removed")
	}
}

func TestMockFS_MkdirAll(t *testing.T) {
	mockFS := NewMockFS()

	if err := mockFS.MkdirAll("/a/b/c", 0755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	if !mockFS.IsDir("/a") {
		t.Error("/a should be a directory")
	}
	if !mockFS.IsDir("/a/b") {
		t.Error("/a/b should be a directory")
	}
	if !mockFS.IsDir("/a/b/c") {
		t.Error("/a/b/c should be a directory")
	}
}

func TestMockFS_CopyFile(t *testing.T) {
	mockFS := NewMockFS()
	mockFS.AddFile("/src.txt", []byte("content"), 0644)

	if err := mockFS.CopyFile("/src.txt", "/dst.txt"); err != nil {
		t.Fatalf("CopyFile error: %v", err)
	}

	data, err := mockFS.ReadFile("/dst.txt")
	if err != nil {
		t.Fatalf("ReadFile dst error: %v", err)
	}

	if string(data) != "content" {
		t.Errorf("Dst content = %q, want %q", string(data), "content")
	}
}

func TestMockFS_ErrorInjection(t *testing.T) {
	mockFS := NewMockFS()
	mockFS.ReadFileErr = fs.ErrPermission

	_, err := mockFS.ReadFile("/anything")
	if err != fs.ErrPermission {
		t.Errorf("ReadFile error = %v, want ErrPermission", err)
	}
}

func TestMockExecutor_Execute(t *testing.T) {
	exec := NewMockExecutor()
	exec.AddResponse("echo", []byte("hello\n"), nil)

	output, err := exec.Execute(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if string(output) != "hello\n" {
		t.Errorf("Output = %q, want %q", string(output), "hello\n")
	}

	// Verify command was recorded
	cmd, ok := exec.LastCommand()
	if !ok {
		t.Fatal("No command recorded")
	}
	if cmd.Name != "echo" {
		t.Errorf("Command name = %q, want %q", cmd.Name, "echo")
	}
}

func TestMockExecutor_DefaultResponse(t *testing.T) {
	exec := NewMockExecutor()
	exec.DefaultResponse = MockResponse{Output: []byte("default"), Err: nil}

	output, err := exec.Execute(context.Background(), "unknown", "command")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if string(output) != "default" {
		t.Errorf("Output = %q, want %q", string(output), "default")
	}
}

func TestMockExecutor_Reset(t *testing.T) {
	exec := NewMockExecutor()
	exec.Execute(context.Background(), "cmd1")
	exec.Execute(context.Background(), "cmd2")

	if len(exec.Commands) != 2 {
		t.Errorf("Commands length = %d, want 2", len(exec.Commands))
	}

	exec.Reset()

	if len(exec.Commands) != 0 {
		t.Errorf("Commands length after reset = %d, want 0", len(exec.Commands))
	}
}
