package repository

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAttachUsageResponseTimingObservesBodyReads(t *testing.T) {
	startedAt := time.Now().Add(-100 * time.Millisecond)
	timing := service.NewUsageResponseTiming(startedAt)
	req, err := http.NewRequest(
		http.MethodGet,
		"https://example.com",
		nil,
	)
	require.NoError(t, err)
	req = req.WithContext(
		service.WithUsageResponseTiming(req.Context(), timing),
	)
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewBufferString("response")),
	}

	attachUsageResponseTiming(req, resp)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	firstByteMs := timing.FirstByteMs()
	require.NotNil(t, firstByteMs)
	duration, ok := timing.UpstreamDuration()
	require.True(t, ok)
	require.GreaterOrEqual(
		t,
		duration.Milliseconds(),
		int64(*firstByteMs),
	)
}

func TestAttachUsageResponseTimingRunsBeforeDecompression(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte("response"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	timing := service.NewUsageResponseTiming(time.Now().Add(-time.Second))
	req, err := http.NewRequest(
		http.MethodGet,
		"https://example.com",
		nil,
	)
	require.NoError(t, err)
	req = req.WithContext(
		service.WithUsageResponseTiming(req.Context(), timing),
	)
	resp := &http.Response{
		Header: http.Header{
			"Content-Encoding": []string{"gzip"},
		},
		Body: io.NopCloser(bytes.NewReader(compressed.Bytes())),
	}

	attachUsageResponseTiming(req, resp)
	decompressResponseBody(resp)

	require.NotNil(t, timing.FirstByteMs())
}
