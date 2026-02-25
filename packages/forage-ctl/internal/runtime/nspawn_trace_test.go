package runtime

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTestTracer returns a nixOutputTracer wired to an in-memory span recorder.
// Call flush + ended() to retrieve the recorded span and its events.
func newTestTracer(t *testing.T) (*nixOutputTracer, func() sdktrace.ReadOnlySpan) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	_, span := tp.Tracer("test").Start(t.Context(), "test-span")

	tracer := newNixOutputTracer(span)

	ended := func() sdktrace.ReadOnlySpan {
		t.Helper()
		tracer.Flush()
		span.End()
		spans := rec.Ended()
		if len(spans) != 1 {
			t.Fatalf("expected 1 span, got %d", len(spans))
		}
		return spans[0]
	}
	return tracer, ended
}

func TestNixOutputTracer_EvalStart(t *testing.T) {
	tr, ended := newTestTracer(t)
	tr.Write([]byte("evaluating derivation 'foo'\n"))

	s := ended()
	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "nix.eval.start" {
		t.Errorf("expected event name nix.eval.start, got %s", events[0].Name)
	}
}

func TestNixOutputTracer_BuildPlan(t *testing.T) {
	tr, ended := newTestTracer(t)
	tr.Write([]byte("these 42 derivations will be built:\n"))

	s := ended()
	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "nix.build.plan" {
		t.Errorf("expected nix.build.plan, got %s", events[0].Name)
	}
	assertAttr(t, events[0].Attributes, "derivation.count", int64(42))
}

func TestNixOutputTracer_BuildDerivation(t *testing.T) {
	tr, ended := newTestTracer(t)
	tr.Write([]byte("building '/nix/store/abc123-foo.drv'\n"))

	s := ended()
	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "nix.build.derivation" {
		t.Errorf("expected nix.build.derivation, got %s", events[0].Name)
	}
	assertAttr(t, events[0].Attributes, "derivation.path", "/nix/store/abc123-foo.drv")
}

func TestNixOutputTracer_CopyPath(t *testing.T) {
	tr, ended := newTestTracer(t)
	tr.Write([]byte("copying path '/nix/store/xyz-bar' to remote\n"))

	s := ended()
	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "nix.copy" {
		t.Errorf("expected nix.copy, got %s", events[0].Name)
	}
	assertAttr(t, events[0].Attributes, "store.path", "/nix/store/xyz-bar")
}

func TestNixOutputTracer_FetchPath(t *testing.T) {
	tr, ended := newTestTracer(t)
	tr.Write([]byte("fetching path '/nix/store/qrs-baz'...\n"))

	s := ended()
	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "nix.fetch" {
		t.Errorf("expected nix.fetch, got %s", events[0].Name)
	}
	assertAttr(t, events[0].Attributes, "store.path", "/nix/store/qrs-baz")
}

func TestNixOutputTracer_UnrelatedLines(t *testing.T) {
	tr, ended := newTestTracer(t)
	tr.Write([]byte("some random output\nanother line\n"))

	s := ended()
	if len(s.Events()) != 0 {
		t.Errorf("expected 0 events for unrelated lines, got %d", len(s.Events()))
	}
}

func TestNixOutputTracer_MultipleLines(t *testing.T) {
	tr, ended := newTestTracer(t)
	tr.Write([]byte("evaluating derivation\nthese 5 derivations will be built:\nbuilding '/nix/store/a-b'\n"))

	s := ended()
	events := s.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Name != "nix.eval.start" {
		t.Errorf("event[0]: expected nix.eval.start, got %s", events[0].Name)
	}
	if events[1].Name != "nix.build.plan" {
		t.Errorf("event[1]: expected nix.build.plan, got %s", events[1].Name)
	}
	if events[2].Name != "nix.build.derivation" {
		t.Errorf("event[2]: expected nix.build.derivation, got %s", events[2].Name)
	}
}

func TestNixOutputTracer_SplitWrites(t *testing.T) {
	tr, ended := newTestTracer(t)
	// Line split across two Write calls
	tr.Write([]byte("building '/nix"))
	tr.Write([]byte("/store/abc-foo'\n"))

	s := ended()
	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Name != "nix.build.derivation" {
		t.Errorf("expected nix.build.derivation, got %s", events[0].Name)
	}
}

func TestNixOutputTracer_FlushPartialLine(t *testing.T) {
	tr, ended := newTestTracer(t)
	// No trailing newline — only emitted on Flush
	tr.Write([]byte("building '/nix/store/partial-drv'"))

	s := ended()
	events := s.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event after flush, got %d", len(events))
	}
	if events[0].Name != "nix.build.derivation" {
		t.Errorf("expected nix.build.derivation, got %s", events[0].Name)
	}
}

func TestNixOutputTracer_WriteReturnsFullLength(t *testing.T) {
	tr, _ := newTestTracer(t)
	input := []byte("hello world\n")
	n, err := tr.Write(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(input) {
		t.Errorf("expected n=%d, got %d", len(input), n)
	}
}

// assertAttr checks that the given attribute key has the expected value.
func assertAttr(t *testing.T, attrs []attribute.KeyValue, key string, want any) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			switch w := want.(type) {
			case string:
				if a.Value.AsString() != w {
					t.Errorf("attr %s: expected %q, got %q", key, w, a.Value.AsString())
				}
			case int64:
				if a.Value.AsInt64() != w {
					t.Errorf("attr %s: expected %d, got %d", key, w, a.Value.AsInt64())
				}
			}
			return
		}
	}
	t.Errorf("attribute %s not found", key)
}
