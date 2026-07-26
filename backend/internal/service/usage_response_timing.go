package service

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type usageResponseTimingContextKey struct{}

// UsageResponseTiming records upstream response observations for one gateway
// forwarding attempt. Downstream response delivery is intentionally excluded.
type UsageResponseTiming struct {
	mu                  sync.Mutex
	startedAt           time.Time
	firstObservedAt     time.Time
	lastObservedAt      time.Time
	upstreamFirstByteMs *int
}

// UpstreamFirstByteMsHeader carries the upstream time to first byte as the
// forwarding gateway measured it. Our own observation starts earlier and ends
// later: it also spans the hop plus whatever the gateway buffered before
// releasing the stream, so the reported value is preferred when present.
const UpstreamFirstByteMsHeader = "x-aether-ttfb-ms"

func NewUsageResponseTiming(startedAt time.Time) *UsageResponseTiming {
	return &UsageResponseTiming{startedAt: startedAt}
}

func WithUsageResponseTiming(ctx context.Context, timing *UsageResponseTiming) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if timing == nil {
		return ctx
	}
	return context.WithValue(ctx, usageResponseTimingContextKey{}, timing)
}

func UsageResponseTimingFromContext(ctx context.Context) *UsageResponseTiming {
	if ctx == nil {
		return nil
	}
	timing, _ := ctx.Value(usageResponseTimingContextKey{}).(*UsageResponseTiming)
	return timing
}

// BeginUpstreamResponse resets observations when an internal retry starts a
// new upstream response while preserving the logical forwarding start time.
func (t *UsageResponseTiming) BeginUpstreamResponse() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.firstObservedAt = time.Time{}
	t.lastObservedAt = time.Time{}
	// A retry reports its own upstream timing; the previous attempt's value must
	// not survive into the attempt that actually served the request.
	t.upstreamFirstByteMs = nil
	t.mu.Unlock()
}

func (t *UsageResponseTiming) ObserveUpstreamRead(observedAt time.Time) {
	if t == nil || observedAt.IsZero() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.startedAt.IsZero() || observedAt.Before(t.startedAt) {
		return
	}
	if t.firstObservedAt.IsZero() {
		t.firstObservedAt = observedAt
	}
	t.lastObservedAt = observedAt
}

// ObserveUpstreamReportedFirstByte records the upstream gateway's own TTFB when
// it reports one. Reading a response header costs nothing extra: the headers are
// already in hand at the single point where the upstream body is instrumented.
func (t *UsageResponseTiming) ObserveUpstreamReportedFirstByte(header http.Header) {
	if t == nil || header == nil {
		return
	}
	raw := strings.TrimSpace(header.Get(UpstreamFirstByteMsHeader))
	if raw == "" {
		return
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.upstreamFirstByteMs = &value
}

// FirstByteMs prefers the upstream gateway's own measurement when it reports
// one. Our local observation spans the hop plus whatever the upstream buffered
// before releasing the stream, so it overstates upstream latency by an amount
// that varies with response shape rather than with the upstream.
func (t *UsageResponseTiming) FirstByteMs() *int {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.upstreamFirstByteMs != nil {
		value := *t.upstreamFirstByteMs
		return &value
	}
	if t.startedAt.IsZero() || t.firstObservedAt.IsZero() {
		return nil
	}
	value := int(t.firstObservedAt.Sub(t.startedAt).Milliseconds())
	return &value
}

func (t *UsageResponseTiming) UpstreamDuration() (time.Duration, bool) {
	if t == nil {
		return 0, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.startedAt.IsZero() || t.lastObservedAt.IsZero() {
		return 0, false
	}
	duration := t.lastObservedAt.Sub(t.startedAt)
	if duration < 0 {
		return 0, false
	}
	return duration, true
}

type usageResponseTimingBody struct {
	io.ReadCloser
	timing *UsageResponseTiming
}

func (b *usageResponseTimingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.timing.ObserveUpstreamRead(time.Now())
	}
	return n, err
}

// WrapUsageResponseTimingBody instruments the raw upstream body. Callers
// should wrap before decompression so compressed header reads establish TTFB.
func WrapUsageResponseTimingBody(ctx context.Context, body io.ReadCloser) io.ReadCloser {
	if body == nil {
		return nil
	}
	timing := UsageResponseTimingFromContext(ctx)
	if timing == nil {
		return body
	}
	timing.BeginUpstreamResponse()
	return &usageResponseTimingBody{ReadCloser: body, timing: timing}
}

// ObserveUpstreamReportedTiming adopts the upstream gateway's own latency
// numbers for this attempt. Call it after WrapUsageResponseTimingBody, which
// clears any value carried over from a previous attempt.
func ObserveUpstreamReportedTiming(ctx context.Context, header http.Header) {
	UsageResponseTimingFromContext(ctx).ObserveUpstreamReportedFirstByte(header)
}
