package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	chatgptCodexAlphaSearchURL   = "https://chatgpt.com/backend-api/codex/alpha/search"
	openAIPlatformAlphaSearchURL = "https://api.openai.com/v1/alpha/search"
)

// ForwardAlphaSearch proxies Codex standalone web search without binding the
// evolving alpha request or response schema.
// Only a 2xx upstream response returns WebSearchCalls=1; passthrough errors are
// never billed.
func (s *OpenAIGatewayService) ForwardAlphaSearch(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
	if s == nil || c == nil || account == nil {
		return nil, fmt.Errorf("service, context, and account are required")
	}
	modelResult := gjson.GetBytes(body, "model")
	requestedModel := strings.TrimSpace(modelResult.String())
	if modelResult.Type != gjson.String || requestedModel == "" {
		return nil, fmt.Errorf("model is required")
	}

	upstreamModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(requestedModel))
	if upstreamModel != "" && upstreamModel != requestedModel {
		body = ReplaceModelInBody(body, upstreamModel)
	}
	body, err := sanitizeOpenAIAlphaSearchBody(body)
	if err != nil {
		return nil, fmt.Errorf("sanitize alpha search request body: %w", err)
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	req, err := s.buildOpenAIAlphaSearchRequest(ctx, c, account, body, token)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, fmt.Errorf("read alpha search response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		upstreamMessage := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMessage, respBody) ||
			isOpenAIAlphaSearchEndpointUnsupported(account, resp.StatusCode) {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			if shouldApplyOpenAIAlphaSearchAccountErrorSideEffects(resp.StatusCode) {
				s.handleFailoverSideEffects(ctx, resp, account, upstreamModel)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
	}

	s.UpdateCodexUsageSnapshotFromHeaders(ctx, account.ID, resp.Header)
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	if disposition := strings.TrimSpace(resp.Header.Get("x-aether-upstream-disposition")); disposition != "" {
		c.Header("x-aether-upstream-disposition", disposition)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, respBody)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil
	}
	return &OpenAIForwardResult{
		RequestID:       strings.TrimSpace(resp.Header.Get("x-request-id")),
		Model:           requestedModel,
		UpstreamModel:   upstreamModel,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(upstreamStart),
		WebSearchCalls:  1,
	}, nil
}

func (s *OpenAIGatewayService) buildOpenAIAlphaSearchRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string) (*http.Request, error) {
	req, err := s.buildUpstreamRequestOpenAIPassthrough(ctx, c, account, body, token)
	if err != nil {
		return nil, err
	}

	targetURL, err := s.openAIAlphaSearchURL(account)
	if err != nil {
		return nil, err
	}
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("parse alpha search URL: %w", err)
	}
	if c != nil && c.Request != nil && c.Request.URL != nil {
		query := parsedURL.Query()
		for key, values := range c.Request.URL.Query() {
			for _, value := range values {
				query.Add(key, value)
			}
		}
		parsedURL.RawQuery = query.Encode()
	}
	req.URL = parsedURL
	req.Method = http.MethodPost
	req.Host = parsedURL.Host
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if account.Type == AccountTypeOAuth {
		req.Host = "chatgpt.com"
	}
	if version := openAIAlphaSearchInboundHeader(c, "Version"); version != "" {
		req.Header.Set("Version", version)
	} else if account.Type == AccountTypeOAuth {
		req.Header.Set("Version", codexCLIVersion)
	}
	if account.Type == AccountTypeOAuth {
		if originator := openAIAlphaSearchInboundHeader(c, "Originator"); originator != "" {
			req.Header.Set("Originator", originator)
		} else {
			req.Header.Set("Originator", "codex_cli_rs")
		}
		if turnMetadata := openAIAlphaSearchInboundHeader(c, "X-Codex-Turn-Metadata"); turnMetadata != "" {
			req.Header.Set("X-Codex-Turn-Metadata", turnMetadata)
		}
		if customUA := account.GetOpenAIUserAgent(); customUA != "" {
			req.Header.Set("User-Agent", customUA)
		} else if userAgent := openAIAlphaSearchInboundHeader(c, "User-Agent"); userAgent != "" {
			req.Header.Set("User-Agent", userAgent)
		} else {
			req.Header.Set("User-Agent", codexCLIUserAgent)
		}
	}
	stripOpenAIAlphaSearchResponsesHeaders(req.Header)
	return req, nil
}

func openAIAlphaSearchInboundHeader(c *gin.Context, key string) string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.GetHeader(key))
}

func stripOpenAIAlphaSearchResponsesHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	for _, key := range []string{
		"OpenAI-Beta",
		"Session_ID",
		"Conversation_ID",
		"X-Codex-Beta-Features",
		"X-Codex-Turn-State",
		"X-OpenAI-Internal-Codex-Responses-Lite",
	} {
		headers.Del(key)
	}
}

var openAIAlphaSearchUnsupportedBodyFields = [...]string{
	"prompt_cache_key",
	"prompt_cache_retention",
}

func sanitizeOpenAIAlphaSearchBody(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil || obj == nil {
		return body, nil
	}
	changed := false
	for _, field := range openAIAlphaSearchUnsupportedBodyFields {
		if _, ok := obj[field]; ok {
			delete(obj, field)
			changed = true
		}
	}
	if !changed {
		return body, nil
	}
	return json.Marshal(obj)
}

// AlphaSearchRequiresBoundAffinity reports whether the request references a
// prior search result by opaque ref_id. Absolute HTTP(S) URLs are stateless,
// while opaque references must stay on the account that created them.
func AlphaSearchRequiresBoundAffinity(body []byte) bool {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		// The handler already rejects ordinary invalid JSON. If this stricter
		// decoder still cannot inspect a body (for example, pathological depth),
		// fail closed and prevent cross-account replay.
		return true
	}

	// Use an iterative walk so deeply nested, forward-compatible request fields
	// cannot bypass the affinity guard or cause recursion proportional to input.
	pending := []any{root}
	for len(pending) > 0 {
		last := len(pending) - 1
		value := pending[last]
		pending = pending[:last]
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if strings.EqualFold(key, "ref_id") {
					if refID, ok := child.(string); ok {
						refID = strings.TrimSpace(refID)
						if refID != "" && !alphaSearchRefIsAbsoluteHTTPURL(refID) {
							return true
						}
					}
				}
				pending = append(pending, child)
			}
		case []any:
			pending = append(pending, typed...)
		}
	}
	return false
}

func alphaSearchRefIsAbsoluteHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) && parsed.Host != ""
}

func isOpenAIAlphaSearchEndpointUnsupported(account *Account, statusCode int) bool {
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
	}
	return statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed
}

func shouldApplyOpenAIAlphaSearchAccountErrorSideEffects(statusCode int) bool {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusNotFound, http.StatusMethodNotAllowed:
		return false
	default:
		return true
	}
}

func (s *OpenAIGatewayService) openAIAlphaSearchURL(account *Account) (string, error) {
	if account == nil {
		return "", fmt.Errorf("account is required")
	}
	switch account.Type {
	case AccountTypeOAuth:
		return chatgptCodexAlphaSearchURL, nil
	case AccountTypeAPIKey:
		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			return openAIPlatformAlphaSearchURL, nil
		}
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return "", err
		}
		return buildOpenAIAlphaSearchEndpointURL(validatedURL), nil
	default:
		return "", fmt.Errorf("unsupported OpenAI account type: %s", account.Type)
	}
}

func buildOpenAIAlphaSearchEndpointURL(base string) string {
	parsedBase, err := url.Parse(base)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" {
		return buildOpenAIEndpointURL(base, "/v1/alpha/search")
	}
	baseQuery := parsedBase.RawQuery
	parsedBase.RawQuery = ""
	parsedBase.ForceQuery = false
	parsedBase.Fragment = ""

	target, err := url.Parse(buildOpenAIEndpointURL(parsedBase.String(), "/v1/alpha/search"))
	if err != nil {
		return buildOpenAIEndpointURL(base, "/v1/alpha/search")
	}
	target.RawQuery = baseQuery
	return target.String()
}
