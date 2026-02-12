// Package audit provides structured event logging for sandbox lifecycle events.
// Events are stored as JSON Lines (JSONL) files, one per sandbox.
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EventType classifies a lifecycle event.
type EventType string

const (
	EventCreate  EventType = "create"
	EventStart   EventType = "start"
	EventStop    EventType = "stop"
	EventDestroy EventType = "destroy"
	EventExec    EventType = "exec"
	EventHealth  EventType = "health"
	EventError   EventType = "error"
)

// Event represents a single audit log entry.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Type      EventType `json:"type"`
	Sandbox   string    `json:"sandbox"`
	Details   string    `json:"details,omitempty"`
}

// Logger writes and reads audit events for sandboxes.
// Events are stored in {stateDir}/sandboxes/{name}/events.jsonl.
type Logger struct {
	stateDir string
}

// NewLogger creates a new audit logger rooted at stateDir.
func NewLogger(stateDir string) *Logger {
	return &Logger{stateDir: stateDir}
}

// eventPath returns the path to the JSONL event log for a sandbox.
func (l *Logger) eventPath(sandbox string) string {
	return filepath.Join(l.stateDir, "sandboxes", sandbox+".events.jsonl")
}

// Log appends an event to the sandbox's audit log.
func (l *Logger) Log(event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	path := l.eventPath(event.Sandbox)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create audit log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open audit log: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	return nil
}

// LogEvent is a convenience method that creates and logs an event.
func (l *Logger) LogEvent(eventType EventType, sandbox, details string) error {
	return l.Log(Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Sandbox:   sandbox,
		Details:   details,
	})
}

// Events reads all events for a sandbox in chronological order.
func (l *Logger) Events(sandbox string) ([]Event, error) {
	path := l.eventPath(sandbox)

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			continue // Skip malformed lines
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("error reading audit log: %w", err)
	}

	return events, nil
}

// Remove deletes the audit log for a sandbox.
func (l *Logger) Remove(sandbox string) error {
	path := l.eventPath(sandbox)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
