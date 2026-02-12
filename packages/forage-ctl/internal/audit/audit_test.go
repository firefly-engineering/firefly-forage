package audit

import (
	"testing"
	"time"
)

func TestLogger_LogAndEvents(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	// Log some events
	now := time.Now().Truncate(time.Millisecond)

	events := []Event{
		{Timestamp: now, Type: EventCreate, Sandbox: "test-sandbox", Details: "template=claude"},
		{Timestamp: now.Add(time.Second), Type: EventStart, Sandbox: "test-sandbox"},
		{Timestamp: now.Add(2 * time.Second), Type: EventHealth, Sandbox: "test-sandbox", Details: "healthy"},
		{Timestamp: now.Add(3 * time.Second), Type: EventStop, Sandbox: "test-sandbox"},
	}

	for _, e := range events {
		if err := logger.Log(e); err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	// Read them back
	result, err := logger.Events("test-sandbox")
	if err != nil {
		t.Fatalf("Events failed: %v", err)
	}

	if len(result) != len(events) {
		t.Fatalf("got %d events, want %d", len(result), len(events))
	}

	for i, e := range result {
		if e.Type != events[i].Type {
			t.Errorf("event %d: type = %q, want %q", i, e.Type, events[i].Type)
		}
		if e.Sandbox != events[i].Sandbox {
			t.Errorf("event %d: sandbox = %q, want %q", i, e.Sandbox, events[i].Sandbox)
		}
		if e.Details != events[i].Details {
			t.Errorf("event %d: details = %q, want %q", i, e.Details, events[i].Details)
		}
	}
}

func TestLogger_EventsEmpty(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	result, err := logger.Events("nonexistent")
	if err != nil {
		t.Fatalf("Events failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("got %d events, want 0", len(result))
	}
}

func TestLogger_LogEvent(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	if err := logger.LogEvent(EventCreate, "my-sandbox", "template=test"); err != nil {
		t.Fatalf("LogEvent failed: %v", err)
	}

	events, err := logger.Events("my-sandbox")
	if err != nil {
		t.Fatalf("Events failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	e := events[0]
	if e.Type != EventCreate {
		t.Errorf("type = %q, want %q", e.Type, EventCreate)
	}
	if e.Sandbox != "my-sandbox" {
		t.Errorf("sandbox = %q, want %q", e.Sandbox, "my-sandbox")
	}
	if e.Details != "template=test" {
		t.Errorf("details = %q, want %q", e.Details, "template=test")
	}
	if e.Timestamp.IsZero() {
		t.Error("timestamp should be set automatically")
	}
}

func TestLogger_Remove(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	logger.LogEvent(EventCreate, "removable", "")

	if err := logger.Remove("removable"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	events, err := logger.Events("removable")
	if err != nil {
		t.Fatalf("Events failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d events after remove, want 0", len(events))
	}
}

func TestLogger_RemoveNonexistent(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	// Should not error
	if err := logger.Remove("nonexistent"); err != nil {
		t.Errorf("Remove should not error for nonexistent: %v", err)
	}
}

func TestLogger_EventOrder(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	base := time.Now()
	for i := 0; i < 5; i++ {
		logger.Log(Event{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Type:      EventExec,
			Sandbox:   "order-test",
			Details:   string(rune('A' + i)),
		})
	}

	events, _ := logger.Events("order-test")
	if len(events) != 5 {
		t.Fatalf("got %d events, want 5", len(events))
	}

	// Events should be in chronological order (append-only)
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Errorf("event %d timestamp before event %d", i, i-1)
		}
	}
}
