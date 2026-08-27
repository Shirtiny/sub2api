package service

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildRequestControlRequestSnapshotCapturesBodyContextAndMultiValueHeaders(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6-sol","input":[{"role":"user","content":"complete diagnostic prompt"}]}`)
	snapshot := buildRequestControlRequestSnapshot(RequestControlCheckInput{
		RequestMethod: "POST",
		RequestHost:   "www.cafecode.work",
		RequestPath:   "/v1/responses",
		RequestQuery:  "debug=1&api_key=secret-query-key",
		ClientIP:      "203.0.113.8",
		RemoteAddr:    "10.0.0.2:4321",
		ContentLength: int64(len(body)),
		MetadataHeaders: http.Header{
			"Authorization": {"Bearer secret"},
			"Cookie":        {"session=secret"},
			"X-Debug":       {"first", "second"},
		},
		Body: body,
	})

	require.True(t, snapshot.Available)
	require.Equal(t, "POST", snapshot.Method)
	require.Equal(t, "www.cafecode.work", snapshot.Host)
	require.Equal(t, "/v1/responses", snapshot.Path)
	require.Equal(t, "api_key=%5Bredacted%5D&debug=1", snapshot.RawQuery)
	require.NotContains(t, snapshot.RawQuery, "secret-query-key")
	require.Equal(t, "203.0.113.8", snapshot.ClientIP)
	require.Equal(t, "10.0.0.2:4321", snapshot.RemoteAddr)
	require.Equal(t, []string{"[redacted]"}, snapshot.Headers["authorization"])
	require.Equal(t, []string{"[redacted]"}, snapshot.Headers["cookie"])
	require.Equal(t, []string{"first", "second"}, snapshot.Headers["x-debug"])
	require.Equal(t, string(body), snapshot.Body)
	require.Equal(t, len(body), snapshot.BodyBytes)
	require.Equal(t, len(body), snapshot.BodyCapturedBytes)
	require.False(t, snapshot.BodyTruncated)
	require.Equal(t, "full", snapshot.BodyCaptureMode)
	digest := sha256.Sum256(body)
	require.Equal(t, hex.EncodeToString(digest[:]), snapshot.BodySHA256)
}

func TestBuildRequestControlRequestSnapshotKeepsHeadTailAndFullHashWhenLarge(t *testing.T) {
	head := strings.Repeat("H", requestControlSnapshotMaxBodyBytes/2)
	middle := strings.Repeat("M", 1024)
	tail := strings.Repeat("T", requestControlSnapshotMaxBodyBytes/2)
	body := []byte(head + middle + tail)

	snapshot := buildRequestControlRequestSnapshot(RequestControlCheckInput{Body: body})

	require.True(t, snapshot.BodyTruncated)
	require.Equal(t, "head_tail", snapshot.BodyCaptureMode)
	require.Equal(t, len(body), snapshot.BodyBytes)
	require.Equal(t, requestControlSnapshotMaxBodyBytes, snapshot.BodyCapturedBytes)
	require.True(t, strings.HasPrefix(snapshot.Body, strings.Repeat("H", 128)))
	require.True(t, strings.HasSuffix(snapshot.Body, strings.Repeat("T", 128)))
	require.Contains(t, snapshot.Body, "request-control omitted")
	require.NotContains(t, snapshot.Body, strings.Repeat("M", 128))
	digest := sha256.Sum256(body)
	require.Equal(t, hex.EncodeToString(digest[:]), snapshot.BodySHA256)
}

func TestBuildRequestControlRequestSnapshotRedactsCredentialsButKeepsPrompt(t *testing.T) {
	body := []byte(`{"model":"gpt-5","input":[{"role":"user","content":"keep this diagnostic prompt"}],"metadata":{"api_key":"secret-key","accessToken":"secret-token","password":"secret-password"}}`)
	snapshot := buildRequestControlRequestSnapshot(RequestControlCheckInput{Body: body})
	require.Contains(t, snapshot.Body, "keep this diagnostic prompt")
	require.NotContains(t, snapshot.Body, "secret-key")
	require.NotContains(t, snapshot.Body, "secret-token")
	require.NotContains(t, snapshot.Body, "secret-password")
	require.Contains(t, snapshot.Body, "\"***\"")
	require.Equal(t, len(snapshot.Body), snapshot.BodyCapturedBytes)
}

func TestBuildRequestControlRequestSnapshotLargeCredentialOutsideCaptureDoesNotTriggerFullScan(t *testing.T) {
	body := []byte(strings.Repeat("A", requestControlSnapshotMaxBodyBytes/2) +
		`{"access_token":"secret-middle-token"}` + strings.Repeat("B", requestControlSnapshotMaxBodyBytes) +
		strings.Repeat("C", requestControlSnapshotMaxBodyBytes/2))
	snapshot := buildRequestControlRequestSnapshot(RequestControlCheckInput{Body: body})
	require.True(t, snapshot.BodyTruncated)
	require.Equal(t, requestControlSnapshotMaxBodyBytes, snapshot.BodyCapturedBytes)
	require.NotContains(t, snapshot.Body, "secret-middle-token")
}

func TestBuildRequestControlRequestSnapshotReappliesLimitAfterRedaction(t *testing.T) {
	body := []byte(`{"input":"` + strings.Repeat("<", 100*1024) + `","password":"secret-password"}`)
	require.Less(t, len(body), requestControlSnapshotMaxBodyBytes)
	snapshot := buildRequestControlRequestSnapshot(RequestControlCheckInput{Body: body})
	require.True(t, snapshot.BodyTruncated)
	require.LessOrEqual(t, len(snapshot.Body), requestControlSnapshotMaxBodyBytes)
	require.Equal(t, len(snapshot.Body), snapshot.BodyCapturedBytes)
	require.NotContains(t, snapshot.Body, "secret-password")
}

func TestBuildRequestControlRequestSnapshotBoundsHeaders(t *testing.T) {
	headers := make(http.Header)
	for i := range requestControlSnapshotMaxHeaders + 10 {
		for range requestControlSnapshotMaxHeaderValues + 2 {
			headers.Add("X-Test-"+strconv.Itoa(i), strings.Repeat("v", requestControlSnapshotMaxHeaderRunes+10))
		}
	}
	snapshot := buildRequestControlRequestSnapshot(RequestControlCheckInput{MetadataHeaders: headers})
	require.LessOrEqual(t, len(snapshot.Headers), requestControlSnapshotMaxHeaders+1)
	require.Equal(t, []string{"true"}, snapshot.Headers["x-request-control-headers-truncated"])
	for key, values := range snapshot.Headers {
		if key == "x-request-control-headers-truncated" {
			continue
		}
		require.LessOrEqual(t, len(values), requestControlSnapshotMaxHeaderValues)
		for _, value := range values {
			require.LessOrEqual(t, len(value), requestControlSnapshotMaxHeaderRunes)
		}
	}
}
