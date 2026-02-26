package runtime

import (
	"regexp"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// nixOutputTracer is an io.Writer that intercepts nix/extra-container stderr
// output line-by-line, emitting OTel span events for known progress patterns
// while forwarding all bytes to an underlying writer.
type nixOutputTracer struct {
	span trace.Span
	// buf accumulates bytes until a newline is seen.
	buf []byte
}

// Patterns matched against each stderr line.
var (
	reEvaluating = regexp.MustCompile(`evaluating derivation`)
	reBuildPlan  = regexp.MustCompile(`these (\d+) derivations will be built`)
	reBuildDrv   = regexp.MustCompile(`building '(/nix/store/[^']+)'`)
	reCopyPath   = regexp.MustCompile(`copying path '(/nix/store/[^']+)'`)
	reFetchPath  = regexp.MustCompile(`fetching path '(/nix/store/[^']+)'`)
)

func newNixOutputTracer(span trace.Span) *nixOutputTracer {
	return &nixOutputTracer{span: span}
}

// Write implements io.Writer. It buffers input and processes complete lines.
func (t *nixOutputTracer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)

	for {
		idx := -1
		for i, b := range t.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := t.buf[:idx]
		t.processLine(line)
		t.buf = t.buf[idx+1:]
	}

	return len(p), nil
}

// Flush processes any remaining partial line in the buffer.
func (t *nixOutputTracer) Flush() {
	if len(t.buf) > 0 {
		t.processLine(t.buf)
		t.buf = nil
	}
}

func (t *nixOutputTracer) processLine(line []byte) {
	s := string(line)

	if reEvaluating.MatchString(s) {
		t.span.AddEvent("nix.eval.start")
		return
	}

	if m := reBuildPlan.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		t.span.AddEvent("nix.build.plan", trace.WithAttributes(
			attribute.Int("derivation.count", n),
		))
		return
	}

	if m := reBuildDrv.FindStringSubmatch(s); m != nil {
		t.span.AddEvent("nix.build.derivation", trace.WithAttributes(
			attribute.String("derivation.path", m[1]),
		))
		return
	}

	if m := reCopyPath.FindStringSubmatch(s); m != nil {
		t.span.AddEvent("nix.copy", trace.WithAttributes(
			attribute.String("store.path", m[1]),
		))
		return
	}

	if m := reFetchPath.FindStringSubmatch(s); m != nil {
		t.span.AddEvent("nix.fetch", trace.WithAttributes(
			attribute.String("store.path", m[1]),
		))
		return
	}
}
