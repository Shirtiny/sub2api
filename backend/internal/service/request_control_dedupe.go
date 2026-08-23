package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

var requestControlMetadataHashIgnoredHeaders = map[string]struct{}{
	"accept-encoding":          {},
	"authorization":            {},
	"cdn-loop":                 {},
	"cf-connecting-ip":         {},
	"cf-ray":                   {},
	"cf-ipcountry":             {},
	"cf-visitor":               {},
	"content-length":           {},
	"cookie":                   {},
	"connection":               {},
	"forwarded":                {},
	"host":                     {},
	"keep-alive":               {},
	"proxy-connection":         {},
	"sec-websocket-extensions": {},
	"sec-websocket-key":        {},
	"sec-websocket-version":    {},
	"te":                       {},
	"trailer":                  {},
	"transfer-encoding":        {},
	"upgrade":                  {},
	"via":                      {},
	"x-client-request-id":      {},
	"x-forwarded-host":         {},
	"x-forwarded-for":          {},
	"x-forwarded-port":         {},
	"x-forwarded-proto":        {},
	"x-real-ip":                {},
}

func requestControlDedupHashes(input RequestControlCheckInput) (string, string) {
	_, body := buildRequestControlMetadata(input)
	return requestControlDedupHeaderHash(input), requestControlMetadataHash(body)
}

func requestControlDedupHeaderHash(input RequestControlCheckInput) string {
	// Remove transport noise before applying the metadata field/size bounds so
	// volatile proxy headers cannot consume the capture budget and hide stable
	// client identity fields.
	source := input.MetadataHeaders
	if source == nil {
		source = input.Headers
	}
	filtered := make(http.Header, len(source))
	for key, values := range source {
		if _, ignored := requestControlMetadataHashIgnoredHeaders[strings.ToLower(strings.TrimSpace(key))]; ignored {
			continue
		}
		filtered[key] = append([]string(nil), values...)
	}
	return requestControlMetadataHash(requestControlHeaderMetadata(filtered))
}

func requestControlMetadataHash(metadata any) string {
	raw, _ := json.Marshal(metadata)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
