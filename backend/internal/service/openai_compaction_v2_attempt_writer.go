package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultOpenAICompactionV2AttemptMaxBytes int64 = 32 * 1024 * 1024

var errOpenAICompactionV2AttemptTooLarge = errors.New("remote compaction v2 response too large")

// openAICompactionV2AttemptWriter isolates a single account attempt. Headers,
// status, and body become visible to the real client only after validation.
type openAICompactionV2AttemptWriter struct {
	parent   gin.ResponseWriter
	header   http.Header
	status   int
	size     int
	maxBytes int64
	buf      bytes.Buffer
	err      error
	parentMu sync.Mutex
}

func newOpenAICompactionV2AttemptWriter(parent gin.ResponseWriter, maxBytes int64) *openAICompactionV2AttemptWriter {
	if maxBytes <= 0 || maxBytes > defaultOpenAICompactionV2AttemptMaxBytes {
		maxBytes = defaultOpenAICompactionV2AttemptMaxBytes
	}
	header := make(http.Header)
	if parent != nil {
		header = parent.Header().Clone()
	}
	return &openAICompactionV2AttemptWriter{
		parent:   parent,
		header:   header,
		status:   http.StatusOK,
		size:     -1,
		maxBytes: maxBytes,
	}
}

func (w *openAICompactionV2AttemptWriter) Header() http.Header {
	return w.header
}

func (w *openAICompactionV2AttemptWriter) WriteHeader(code int) {
	if code <= 0 || w.Written() {
		return
	}
	w.status = code
}

func (w *openAICompactionV2AttemptWriter) WriteHeaderNow() {
	if !w.Written() {
		w.size = 0
	}
}

func (w *openAICompactionV2AttemptWriter) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	w.size += len(data)
	if w.err != nil {
		return len(data), nil
	}
	remaining := w.maxBytes - int64(w.buf.Len())
	if int64(len(data)) > remaining {
		if remaining > 0 {
			_, _ = w.buf.Write(data[:remaining])
		}
		w.err = errOpenAICompactionV2AttemptTooLarge
		return len(data), nil
	}
	_, _ = w.buf.Write(data)
	return len(data), nil
}

func (w *openAICompactionV2AttemptWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *openAICompactionV2AttemptWriter) Status() int {
	return w.status
}

func (w *openAICompactionV2AttemptWriter) Size() int {
	return w.size
}

func (w *openAICompactionV2AttemptWriter) Written() bool {
	return w.size >= 0
}

func (w *openAICompactionV2AttemptWriter) Flush() {
	w.WriteHeaderNow()
}

func (w *openAICompactionV2AttemptWriter) CloseNotify() <-chan bool {
	if w.parent != nil {
		return w.parent.CloseNotify()
	}
	ch := make(chan bool)
	close(ch)
	return ch
}

func (w *openAICompactionV2AttemptWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("buffered remote compaction response cannot be hijacked")
}

func (w *openAICompactionV2AttemptWriter) Pusher() http.Pusher {
	if w.parent == nil {
		return nil
	}
	return w.parent.Pusher()
}

func (w *openAICompactionV2AttemptWriter) Body() []byte {
	return w.buf.Bytes()
}

func (w *openAICompactionV2AttemptWriter) Err() error {
	return w.err
}

func (w *openAICompactionV2AttemptWriter) ParentWritten() bool {
	if w == nil || w.parent == nil {
		return false
	}
	w.parentMu.Lock()
	defer w.parentMu.Unlock()
	return w.parent.Written()
}

func (w *openAICompactionV2AttemptWriter) Commit() error {
	if w.parent == nil {
		return errors.New("remote compaction response writer has no parent")
	}
	if w.err != nil {
		return w.err
	}
	w.parentMu.Lock()
	defer w.parentMu.Unlock()
	if !w.parent.Written() {
		parentHeader := w.parent.Header()
		for key := range parentHeader {
			parentHeader.Del(key)
		}
		for key, values := range w.header {
			for _, value := range values {
				parentHeader.Add(key, value)
			}
		}
		w.parent.WriteHeader(w.status)
	}
	if !w.Written() {
		return nil
	}
	if w.buf.Len() == 0 {
		w.parent.WriteHeaderNow()
		return nil
	}
	_, err := io.Copy(w.parent, bytes.NewReader(w.buf.Bytes()))
	return err
}

func (w *openAICompactionV2AttemptWriter) writeKeepalive() error {
	if w == nil || w.parent == nil {
		return errors.New("remote compaction response writer has no parent")
	}
	w.parentMu.Lock()
	defer w.parentMu.Unlock()
	if !w.parent.Written() {
		header := w.parent.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		header.Set("X-Accel-Buffering", "no")
	}
	if _, err := io.WriteString(w.parent, ":\n\n"); err != nil {
		return err
	}
	if flusher, ok := w.parent.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (w *openAICompactionV2AttemptWriter) startKeepalive(ctx context.Context, interval time.Duration) func() {
	if w == nil || w.parent == nil || interval <= 0 {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				if err := w.writeKeepalive(); err != nil {
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(stop) })
		<-done
	}
}

var _ gin.ResponseWriter = (*openAICompactionV2AttemptWriter)(nil)
