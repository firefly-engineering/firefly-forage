package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestSetup_TextOutput(t *testing.T) {
	var buf bytes.Buffer
	Setup(false, false, &buf)

	Info("test message", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected 'test message' in output, got: %s", output)
	}
}

func TestSetup_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	Setup(false, true, &buf)

	Info("test message", "key", "value")

	output := buf.String()
	// JSON output should contain braces
	if !strings.Contains(output, "{") {
		t.Errorf("Expected JSON output, got: %s", output)
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected 'test message' in output, got: %s", output)
	}
}

func TestSetup_VerboseMode(t *testing.T) {
	var buf bytes.Buffer
	Setup(true, false, &buf)

	if !Verbose {
		t.Error("Verbose flag should be true after Setup(true, ...)")
	}

	Debug("debug message")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Errorf("Debug message should appear in verbose mode, got: %s", output)
	}
}

func TestSetup_NonVerboseMode(t *testing.T) {
	var buf bytes.Buffer
	Setup(false, false, &buf)

	if Verbose {
		t.Error("Verbose flag should be false after Setup(false, ...)")
	}

	Debug("debug message")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Errorf("Debug message should NOT appear in non-verbose mode, got: %s", output)
	}
}

func TestDebug(t *testing.T) {
	var buf bytes.Buffer
	Setup(true, false, &buf)

	Debug("debug test", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "debug test") {
		t.Errorf("Expected 'debug test' in output, got: %s", output)
	}
}

func TestInfo(t *testing.T) {
	var buf bytes.Buffer
	Setup(false, false, &buf)

	Info("info test", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "info test") {
		t.Errorf("Expected 'info test' in output, got: %s", output)
	}
}

func TestWarn(t *testing.T) {
	var buf bytes.Buffer
	Setup(false, false, &buf)

	Warn("warn test", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "warn test") {
		t.Errorf("Expected 'warn test' in output, got: %s", output)
	}
}

func TestError(t *testing.T) {
	var buf bytes.Buffer
	Setup(false, false, &buf)

	Error("error test", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "error test") {
		t.Errorf("Expected 'error test' in output, got: %s", output)
	}
}

func TestWith(t *testing.T) {
	var buf bytes.Buffer
	Setup(false, false, &buf)

	logger := With("component", "test")
	if logger == nil {
		t.Error("With() returned nil")
	}

	logger.Info("with test")

	output := buf.String()
	if !strings.Contains(output, "with test") {
		t.Errorf("Expected 'with test' in output, got: %s", output)
	}
	if !strings.Contains(output, "component") {
		t.Errorf("Expected 'component' in output, got: %s", output)
	}
}

func TestSetup_NilWriter(t *testing.T) {
	// Should not panic with nil writer
	Setup(false, false, nil)

	// Logger should still work (writes to stderr)
	if Logger == nil {
		t.Error("Logger should not be nil after Setup with nil writer")
	}
}
