package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type openAIWSClientFrameConn struct {
	conn *coderws.Conn
}

type OpenAIWSClientEnvelope = openaiwsv2.ClientEnvelope

// ParseOpenAIWSClientEnvelope validates one client text event without copying
// its request body. Callers receive only bounded routing metadata.
func ParseOpenAIWSClientEnvelope(payload []byte) (OpenAIWSClientEnvelope, error) {
	return openaiwsv2.ParseClientEnvelope(payload)
}

func validateOpenAIWSPassthroughSessionModel(envelope OpenAIWSClientEnvelope, frozenModel string) error {
	if envelope.Type != "session.update" || !envelope.HasSessionModel || envelope.SessionModel == frozenModel {
		return nil
	}
	return NewOpenAIWSClientCloseError(
		coderws.StatusPolicyViolation,
		"changing model requires a new websocket connection",
		errors.New("session.update attempted to change the websocket model"),
	)
}

func transformOpenAIWSPassthroughSessionModel(
	payload []byte,
	envelope OpenAIWSClientEnvelope,
	upstreamModel string,
) ([]byte, error) {
	if envelope.Type != "session.update" || !envelope.HasSessionModel || envelope.SessionModel == upstreamModel {
		return payload, nil
	}
	return sjson.SetBytes(payload, "session.model", upstreamModel)
}

func transformOpenAIWSPassthroughResponseModel(payload []byte, currentModel string, upstreamModel string) ([]byte, error) {
	if len(payload) == 0 || strings.TrimSpace(upstreamModel) == "" {
		return nil, errors.New("websocket upstream model is unavailable")
	}
	if strings.TrimSpace(currentModel) == strings.TrimSpace(upstreamModel) {
		return payload, nil
	}
	return sjson.SetBytes(payload, "model", upstreamModel)
}

func sameOpenAIWSPayload(payload, updated []byte) bool {
	if len(payload) != len(updated) {
		return false
	}
	if len(payload) == 0 {
		return true
	}
	return &payload[0] == &updated[0]
}

// openAIWSPolicyEnforcingFrameConn wraps a client-side FrameConn and runs
// every client→upstream frame through the OpenAI Fast Policy. It is the
// passthrough-relay equivalent of the parseClientPayload integration in the
// ingress session path. filter returns:
//   - newPayload, nil, nil: forward the (possibly mutated) payload
//   - _, *OpenAIFastBlockedError, nil: block — the wrapper sends an error
//     event via onBlock and surfaces a transport-level error so the relay
//     stops reading from the client.
//   - _, _, err: a transport error other than block.
type openAIWSPolicyEnforcingFrameConn struct {
	inner   openaiwsv2.FrameConn
	filter  func(msgType coderws.MessageType, payload []byte) ([]byte, *OpenAIFastBlockedError, error)
	onBlock func(blocked *OpenAIFastBlockedError)
}

var _ openaiwsv2.FrameConn = (*openAIWSPolicyEnforcingFrameConn)(nil)

func (c *openAIWSPolicyEnforcingFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.inner == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	}
	msgType, payload, err := c.inner.ReadFrame(ctx)
	if err != nil {
		return msgType, payload, err
	}
	if c.filter == nil {
		return msgType, payload, nil
	}
	updated, blocked, filterErr := c.filter(msgType, payload)
	if filterErr != nil {
		return msgType, payload, filterErr
	}
	if blocked != nil {
		if c.onBlock != nil {
			c.onBlock(blocked)
		}
		return msgType, nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, blocked.Message, blocked)
	}
	return msgType, updated, nil
}

func (c *openAIWSPolicyEnforcingFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.inner == nil {
		return errOpenAIWSConnClosed
	}
	return c.inner.WriteFrame(ctx, msgType, payload)
}

func (c *openAIWSPolicyEnforcingFrameConn) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.Close()
}

// openAIWSPassthroughPolicyModelForFrame returns the upstream-perspective
// model name that should be passed to evaluateOpenAIFastPolicy for a single
// passthrough WS frame. Mirrors the HTTP-side normalization
// (account.GetMappedModel + normalizeOpenAIModelForUpstream) so the WS path
// matches model whitelists identically.
func openAIWSPassthroughPolicyModelForFrame(account *Account, payload []byte) string {
	if account == nil || len(payload) == 0 {
		return ""
	}
	original := strings.TrimSpace(gjson.GetBytes(payload, "model").String())
	if original == "" {
		return ""
	}
	return normalizeOpenAIModelForUpstream(account, account.GetMappedModel(original))
}

// openAIWSPassthroughPolicyModelFromSessionFrame returns the upstream model
// derived from a session.update frame's session.model field. Returns "" when
// the frame is not a session.update event or carries no session.model. Used
// by the per-frame policy filter (client→upstream direction) to keep
// capturedSessionModel in sync with the session-level model the client may
// rotate mid-session.
//
// Realtime / Responses WS lets the client change the session model after
// the WS handshake via:
//
//	{"type":"session.update","session":{"model":"gpt-5.5", ...}}
//
// If we only capture the model from the very first frame, a client can ship
// gpt-4o on the first response.create (whitelisted as pass), then
// session.update to gpt-5.5, then send response.create without "model" so
// the per-frame resolver returns "" and the stale capturedSessionModel falls
// back to gpt-4o — defeating the gpt-5.5 fast-policy filter.
func openAIWSPassthroughPolicyModelFromSessionFrame(account *Account, payload []byte) string {
	if account == nil || len(payload) == 0 {
		return ""
	}
	frameType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if frameType != "session.update" {
		return ""
	}
	original := strings.TrimSpace(gjson.GetBytes(payload, "session.model").String())
	if original == "" {
		return ""
	}
	return normalizeOpenAIModelForUpstream(account, account.GetMappedModel(original))
}

type openAIWSPassthroughUsageMeta struct {
	serviceTier     atomic.Pointer[string]
	reasoningEffort atomic.Pointer[string]

	// 仅在 client->upstream filter goroutine 中读写；Load 侧通过上方原子指针同步。
	sessionRequestModel string
}

func newOpenAIWSPassthroughUsageMeta(initialRequestModel string, firstFrame []byte) *openAIWSPassthroughUsageMeta {
	meta := &openAIWSPassthroughUsageMeta{
		sessionRequestModel: strings.TrimSpace(initialRequestModel),
	}
	if meta.sessionRequestModel == "" {
		meta.sessionRequestModel = openAIWSPassthroughRequestModelForFrame(firstFrame)
	}
	return meta
}

func newOpenAIWSPassthroughUsageMetaFromEnvelope(initialRequestModel string, envelope OpenAIWSClientEnvelope) *openAIWSPassthroughUsageMeta {
	requestModel := strings.TrimSpace(initialRequestModel)
	if requestModel == "" {
		requestModel = envelope.Model
	}
	return &openAIWSPassthroughUsageMeta{sessionRequestModel: requestModel}
}

func openAIWSServiceTierFromEnvelope(envelope OpenAIWSClientEnvelope) *string {
	if !envelope.HasServiceTier {
		return nil
	}
	return normalizeOpenAIServiceTier(envelope.ServiceTier)
}

func openAIWSReasoningEffortFromEnvelope(envelope OpenAIWSClientEnvelope, requestedModel string) *string {
	if envelope.HasReasoningEffort {
		if normalized := normalizeOpenAIReasoningEffort(envelope.ReasoningEffort); normalized != "" {
			return &normalized
		}
		return nil
	}
	value := deriveOpenAIReasoningEffortFromModel(requestedModel)
	if value == "" {
		return nil
	}
	return &value
}

func (m *openAIWSPassthroughUsageMeta) initFromEnvelope(envelope OpenAIWSClientEnvelope) {
	if m == nil {
		return
	}
	m.serviceTier.Store(openAIWSServiceTierFromEnvelope(envelope))
	m.reasoningEffort.Store(openAIWSReasoningEffortFromEnvelope(envelope, m.sessionRequestModel))
}

func (m *openAIWSPassthroughUsageMeta) initFromFirstFrame(policyOutput []byte) {
	if m == nil {
		return
	}
	m.serviceTier.Store(extractOpenAIServiceTierFromBody(policyOutput))
	m.reasoningEffort.Store(extractOpenAIReasoningEffortFromBody(policyOutput, m.sessionRequestModel))
}

func (m *openAIWSPassthroughUsageMeta) updateSessionRequestModel(payload []byte) {
	if m == nil {
		return
	}
	if model := openAIWSPassthroughRequestModelFromSessionFrame(payload); model != "" {
		m.sessionRequestModel = model
	}
}

func (m *openAIWSPassthroughUsageMeta) requestModelForFrame(payload []byte) string {
	if m == nil {
		return openAIWSPassthroughRequestModelForFrame(payload)
	}
	if model := openAIWSPassthroughRequestModelForFrame(payload); model != "" {
		return model
	}
	return m.sessionRequestModel
}

func (m *openAIWSPassthroughUsageMeta) requestModelForEnvelope(envelope OpenAIWSClientEnvelope) string {
	if envelope.Model != "" {
		return envelope.Model
	}
	if m == nil {
		return ""
	}
	return m.sessionRequestModel
}

func (m *openAIWSPassthroughUsageMeta) updateFromResponseCreate(policyOutput []byte, requestModelForFrame string) {
	if m == nil {
		return
	}
	m.serviceTier.Store(extractOpenAIServiceTierFromBody(policyOutput))
	m.reasoningEffort.Store(extractOpenAIReasoningEffortFromBody(policyOutput, requestModelForFrame))
}

func (m *openAIWSPassthroughUsageMeta) updateFromEnvelope(envelope OpenAIWSClientEnvelope, requestModelForFrame string) {
	if m == nil {
		return
	}
	m.serviceTier.Store(openAIWSServiceTierFromEnvelope(envelope))
	m.reasoningEffort.Store(openAIWSReasoningEffortFromEnvelope(envelope, requestModelForFrame))
}

func openAIWSPassthroughRequestModelForFrame(payload []byte) string {
	if len(payload) == 0 || strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "response.create" {
		return ""
	}
	return strings.TrimSpace(gjson.GetBytes(payload, "model").String())
}

func openAIWSPassthroughRequestModelFromSessionFrame(payload []byte) string {
	if len(payload) == 0 || strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "session.update" {
		return ""
	}
	return strings.TrimSpace(gjson.GetBytes(payload, "session.model").String())
}

const openaiWSV2PassthroughModeFields = "ws_mode=passthrough ws_router=v2"

var _ openaiwsv2.FrameConn = (*openAIWSClientFrameConn)(nil)

func (c *openAIWSClientFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.conn == nil {
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.conn.Read(ctx)
}

func (c *openAIWSClientFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.conn == nil {
		return errOpenAIWSConnClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.conn.Write(ctx, msgType, payload)
}

func (c *openAIWSClientFrameConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	_ = c.conn.Close(coderws.StatusNormalClosure, "")
	_ = c.conn.CloseNow()
	return nil
}

func (s *OpenAIGatewayService) proxyResponsesWebSocketV2Passthrough(
	ctx context.Context,
	c *gin.Context,
	clientConn *coderws.Conn,
	account *Account,
	token string,
	firstClientMessage []byte,
	hooks *OpenAIWSIngressHooks,
	wsDecision OpenAIWSProtocolDecision,
) error {
	if s == nil {
		return errors.New("service is nil")
	}
	if clientConn == nil {
		return errors.New("client websocket is nil")
	}
	if account == nil {
		return errors.New("account is nil")
	}
	if strings.TrimSpace(token) == "" {
		return errors.New("token is empty")
	}
	if wsDecision.AetherCapability.Effective {
		if sizeErr := validateAetherWSClientPayload(firstClientMessage); sizeErr != nil {
			return wrapOpenAIWSIngressTurnError(
				"write_upstream",
				NewOpenAIWSClientCloseError(coderws.StatusMessageTooBig, "websocket request payload is too large", sizeErr),
				false,
			)
		}
	}
	firstEnvelope := OpenAIWSClientEnvelope{}
	var envelopeErr error
	if hooks != nil && hooks.HasValidatedFirstEnvelope {
		firstEnvelope = hooks.ValidatedFirstEnvelope
	} else {
		firstEnvelope, envelopeErr = ParseOpenAIWSClientEnvelope(firstClientMessage)
	}
	if envelopeErr != nil || firstEnvelope.Type != "response.create" {
		if envelopeErr == nil {
			envelopeErr = errors.New("first websocket event must be response.create")
		}
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", envelopeErr)
	}
	requestModel := firstEnvelope.Model
	requestPreviousResponseID := firstEnvelope.PreviousResponseID
	logOpenAIWSV2Passthrough(
		"relay_start account_id=%d model=%s previous_response_id=%s first_message_type=%s first_message_bytes=%d",
		account.ID,
		truncateOpenAIWSLogValue(requestModel, openAIWSLogValueMaxLen),
		truncateOpenAIWSLogValue(requestPreviousResponseID, openAIWSIDValueMaxLen),
		openaiwsv2RelayMessageTypeName(coderws.MessageText),
		len(firstClientMessage),
	)

	// Apply OpenAI Fast Policy on the first response.create frame. The relay's
	// admitted-response callback applies the same policy to later turns.
	//
	// We capture the session-level model from the first frame here so the
	// later-turn callback can fall back to it when a follow-up frame
	// omits "model" — Realtime clients are allowed to send response.create
	// without re-stating the model, in which case the upstream uses the model
	// negotiated at session.update time. Without this fallback, an empty
	// model would miss any admin-configured model whitelist and be silently
	// passed through, defeating that policy on every frame after the first.
	initialRequestModel := ""
	routedRequestModel := requestModel
	if hooks != nil {
		initialRequestModel = hooks.InitialRequestModel
		if candidate := strings.TrimSpace(hooks.RoutedRequestModel); candidate != "" {
			routedRequestModel = candidate
		}
	}
	originalRequestModel := strings.TrimSpace(initialRequestModel)
	if originalRequestModel == "" {
		originalRequestModel = requestModel
	}
	frozenRequestModel := normalizeOpenAIModelForUpstream(account, account.GetMappedModel(routedRequestModel))
	if frozenRequestModel == "" {
		return NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"websocket upstream model is unavailable",
			errors.New("account model mapping resolved to an empty model"),
		)
	}
	capturedSessionModel := frozenRequestModel
	usageMeta := newOpenAIWSPassthroughUsageMetaFromEnvelope(initialRequestModel, firstEnvelope)
	validatedFirstPayload := firstClientMessage
	firstPolicyDecision := openAIWSFastPolicyDecision{}
	firstPolicyDeferred := wsDecision.AetherCapability.Effective && (hooks == nil || hooks.TransformRequest == nil)
	updatedFirst := firstClientMessage
	var blocked *OpenAIFastBlockedError
	var policyErr error
	if firstPolicyDeferred {
		firstPolicyDecision, blocked = s.evaluateOpenAIFastPolicyForWS(
			ctx,
			account,
			frozenRequestModel,
			firstEnvelope.ServiceTier,
			firstEnvelope.HasServiceTier,
		)
	} else {
		updatedFirst, blocked, policyErr = s.applyValidatedOpenAIFastPolicyToWSResponseCreateWithTier(
			ctx,
			account,
			frozenRequestModel,
			firstClientMessage,
			firstEnvelope.ServiceTier,
			firstEnvelope.HasServiceTier,
		)
	}
	if policyErr != nil {
		return fmt.Errorf("%w: apply openai fast policy on first ws frame: %w", ErrOpenAIWSLocalAdmission, policyErr)
	}
	if blocked != nil {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
		// coder/websocket@v1.8.14 Conn.Write is synchronous: it acquires
		// writeFrameMu, writes the entire frame, and Flushes the underlying
		// bufio writer before returning (write.go:42 → write.go:307-311).
		// The subsequent close handshake re-acquires the same writeFrameMu
		// to send the close frame, so the error event is guaranteed to
		// reach the kernel send buffer before any close frame is queued.
		// No explicit flush hop is required here.
		eventBytes := buildOpenAIFastPolicyBlockedWSEvent(blocked)
		if eventBytes != nil {
			writeCtx, cancelWrite := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
			_ = clientConn.Write(writeCtx, coderws.MessageText, eventBytes)
			cancelWrite()
		}
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, blocked.Message, blocked)
	}
	firstClientMessage = updatedFirst
	firstTransformEnvelope := firstEnvelope
	if firstPolicyDeferred {
		firstTransformEnvelope = firstPolicyDecision.applyToEnvelope(firstEnvelope)
	}
	if !sameOpenAIWSPayload(validatedFirstPayload, firstClientMessage) {
		firstTransformEnvelope, envelopeErr = ParseOpenAIWSClientEnvelope(firstClientMessage)
		if envelopeErr != nil {
			return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", envelopeErr)
		}
	}

	// 在 policy filter 之后再提取 service_tier / reasoning_effort 用于
	// usage 上报：filter
	// 命中时 service_tier 已经从 firstClientMessage 中删除，billing 应当
	// 反映上游实际处理的 tier（nil = default），而不是用户最初请求的
	// "priority"。HTTP 入口（line ~2728 extractOpenAIServiceTier(reqBody)）
	// 与 WS ingress（openai_ws_forwarder.go:2991 取自 payload）的语义一致。
	//
	// 多轮 passthrough：OpenAI Realtime / Responses WS 协议允许客户端在
	// 同一连接的不同 response.create 帧上发送不同 service_tier（参考
	// codex-rs/core/src/client.rs build_responses_request 每次重新填值）。
	// 因此使用 atomic.Pointer[string] 在 filter（runClientToUpstream
	// goroutine）和 OnTurnComplete / final result（runUpstreamToClient
	// goroutine）之间同步当前 turn 的 usage metadata。
	usageMeta.initFromEnvelope(firstTransformEnvelope)
	promptCacheKey := firstTransformEnvelope.PromptCacheKey

	wsURL, err := s.buildOpenAIResponsesWSURL(account, wsDecision)
	if err != nil {
		return fmt.Errorf("build ws url: %w", err)
	}
	wsHost := "-"
	wsPath := "-"
	if parsedURL, parseErr := url.Parse(wsURL); parseErr == nil && parsedURL != nil {
		wsHost = normalizeOpenAIWSLogValue(parsedURL.Host)
		wsPath = normalizeOpenAIWSLogValue(parsedURL.Path)
	}
	logOpenAIWSV2Passthrough(
		"relay_dial_start account_id=%d ws_host=%s ws_path=%s proxy_enabled=%v",
		account.ID,
		wsHost,
		wsPath,
		!wsDecision.AetherCapability.Effective && account.ProxyID != nil && account.Proxy != nil,
	)

	isCodexCLI := false
	if c != nil {
		isCodexCLI = openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator"))
	}
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		isCodexCLI = true
	}
	turnState := ""
	turnMetadata := ""
	if c != nil {
		turnState = strings.TrimSpace(c.GetHeader(openAIWSTurnStateHeader))
		turnMetadata = strings.TrimSpace(c.GetHeader(openAIWSTurnMetadataHeader))
	}
	headers, _ := s.buildOpenAIWSHeaders(c, account, token, wsDecision, isCodexCLI, turnState, turnMetadata, promptCacheKey)
	aetherCapability := wsDecision.AetherCapability
	if account.IsAetherWSManaged() && (!wsDecision.AetherManaged || !aetherCapability.Effective) {
		return fmt.Errorf("aether websocket account is not effective: %s", aetherCapability.Reason)
	}
	applyAetherWSHandshakeRequestHeaders(headers, aetherCapability)
	proxyURL := ""
	if !aetherCapability.Effective && account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	dialer := s.getOpenAIWSPassthroughDialer()
	if dialer == nil {
		return errors.New("openai ws passthrough dialer is nil")
	}

	dialCtx, cancelDial := context.WithTimeout(ctx, s.openAIWSDialTimeout())
	defer cancelDial()
	var upstreamConn openAIWSClientConn
	var statusCode int
	var handshakeHeaders http.Header
	if aetherCapability.Effective {
		// The administrator-managed Aether hop is local: avoid proxy lookup and
		// compression CPU. route-v1 is still negotiated in the same 101 RTT.
		optionsDialer, optionsErr := requireAetherLocalOpenAIWSDialer(dialer)
		if optionsErr != nil {
			return optionsErr
		}
		upstreamConn, statusCode, handshakeHeaders, err = optionsDialer.DialWithOptions(
			dialCtx,
			wsURL,
			headers,
			proxyURL,
			aetherLocalOpenAIWSDialOptions(),
		)
	} else {
		upstreamConn, statusCode, handshakeHeaders, err = dialer.Dial(dialCtx, wsURL, headers, proxyURL)
	}
	if err != nil {
		logOpenAIWSV2Passthrough(
			"relay_dial_failed account_id=%d status_code=%d err=%s",
			account.ID,
			statusCode,
			truncateOpenAIWSLogValue(err.Error(), openAIWSLogValueMaxLen),
		)
		if statusCode == http.StatusTooManyRequests {
			s.persistOpenAIWSRateLimitSignal(ctx, account, handshakeHeaders, nil, "rate_limit_exceeded", "rate_limit_error", strings.TrimSpace(err.Error()))
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
		return newOpenAIWSPreDispatchFailover(statusCode, handshakeHeaders)
	}
	negotiatedAether, negotiationErr := validateAetherWSHandshakeResponse(handshakeHeaders, aetherCapability)
	if negotiationErr != nil {
		_ = upstreamConn.Close()
		logOpenAIWSV2Passthrough(
			"aether_route_control_negotiation_failed account_id=%d err=%s",
			account.ID,
			truncateOpenAIWSLogValue(negotiationErr.Error(), openAIWSLogValueMaxLen),
		)
		return newOpenAIWSPreDispatchFailover(http.StatusBadGateway, handshakeHeaders)
	}
	var aetherRouteControl *aetherWSRouteControlConsumer
	if aetherCapability.Effective {
		reconnectEnabled := false
		reconnectSignalMode := ""
		if s.cfg != nil {
			reconnectEnabled = s.cfg.Gateway.OpenAIWS.ReconnectMigrationEnabled
			reconnectSignalMode = s.cfg.Gateway.OpenAIWS.ReconnectSignalMode
		}
		bindingGeneration := uint64(0)
		if hooks != nil {
			bindingGeneration = hooks.RouteControlBindingGeneration
			reconnectEnabled = reconnectEnabled && hooks.BeforeReconnectSignal != nil && bindingGeneration > 0
		} else {
			reconnectEnabled = false
		}
		aetherRouteControl, negotiationErr = newAetherWSRouteControlConsumer(aetherWSRouteControlConsumerConfig{
			Negotiated:          negotiatedAether,
			ReconnectEnabled:    reconnectEnabled,
			ReconnectSignalMode: reconnectSignalMode,
			BindingGeneration:   bindingGeneration,
		})
		if negotiationErr != nil {
			_ = upstreamConn.Close()
			logOpenAIWSV2Passthrough(
				"aether_route_control_setup_failed account_id=%d err=%s",
				account.ID,
				truncateOpenAIWSLogValue(negotiationErr.Error(), openAIWSLogValueMaxLen),
			)
			return newOpenAIWSPreDispatchFailover(http.StatusBadGateway, handshakeHeaders)
		}
	}
	defer func() {
		_ = upstreamConn.Close()
	}()
	logOpenAIWSV2Passthrough(
		"relay_dial_ok account_id=%d status_code=%d upstream_request_id=%s",
		account.ID,
		statusCode,
		openAIWSHeaderValueForLog(handshakeHeaders, "x-request-id"),
	)

	upstreamFrameConn, ok := upstreamConn.(openaiwsv2.FrameConn)
	if !ok {
		return errors.New("openai ws passthrough upstream connection does not support frame relay")
	}

	completedTurns := atomic.Int32{}
	onPolicyBlock := func(blocked *OpenAIFastBlockedError) {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
		// coder/websocket writes synchronously, so this error event is flushed
		// before the relay returns and the close frame is sent.
		eventBytes := buildOpenAIFastPolicyBlockedWSEvent(blocked)
		if eventBytes == nil {
			return
		}
		writeCtx, cancel := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
		_ = clientConn.Write(writeCtx, coderws.MessageText, eventBytes)
		cancel()
	}
	clientFrameConn := &openAIWSClientFrameConn{conn: clientConn}
	var readClientFrame func(context.Context, openaiwsv2.FrameConn) (coderws.MessageType, []byte, error)
	if hooks != nil && hooks.ReadClientFrame != nil {
		readClientFrame = func(readCtx context.Context, _ openaiwsv2.FrameConn) (coderws.MessageType, []byte, error) {
			return hooks.ReadClientFrame(readCtx)
		}
	}
	upstreamFirstMessageSent := false
	if hooks != nil && hooks.BeforeRequest != nil {
		if err := hooks.BeforeRequest(1, firstClientMessage, originalRequestModel); err != nil {
			return err
		}
	}
	if hooks != nil && hooks.BeforeTurn != nil {
		if err := hooks.BeforeTurn(1); err != nil {
			return err
		}
	}
	if aetherRouteControl != nil {
		routeEnvelope := firstTransformEnvelope
		tierMutation := (*aetherWSServiceTierMutation)(nil)
		if firstPolicyDeferred {
			routeEnvelope = firstEnvelope
			tierMutation = firstPolicyDecision.serviceTierMutation()
		}
		firstClientMessage, err = aetherRouteControl.prepareValidatedResponseCreateWithEnvelopeAndModelAndServiceTier(
			firstClientMessage,
			routeEnvelope,
			frozenRequestModel,
			tierMutation,
		)
		if err != nil {
			return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", err)
		}
	} else {
		firstClientMessage, err = transformOpenAIWSPassthroughResponseModel(firstClientMessage, firstTransformEnvelope.Model, frozenRequestModel)
		if err != nil {
			return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", err)
		}
	}
	if aetherCapability.Effective {
		if sizeErr := validateAetherWSRoutedPayload(firstClientMessage); sizeErr != nil {
			return NewOpenAIWSClientCloseError(coderws.StatusMessageTooBig, "websocket request payload is too large", sizeErr)
		}
	}
	if err := runOpenAIWSBeforeProviderWrite(hooks, 1, firstClientMessage, originalRequestModel); err != nil {
		return err
	}
	firstMessageStartedAt := time.Now()
	firstWriteCtx, cancelFirstWrite := context.WithTimeout(ctx, s.openAIWSWriteTimeout())
	firstWriteErr := upstreamFrameConn.WriteFrame(firstWriteCtx, coderws.MessageText, firstClientMessage)
	cancelFirstWrite()
	if firstWriteErr != nil {
		return wrapOpenAIWSIngressTurnError(
			"write_upstream",
			fmt.Errorf("write first upstream websocket request: %w", firstWriteErr),
			false,
		)
	}
	upstreamFirstMessageSent = true

	relayResult, relayExit := openaiwsv2.RunEntry(openaiwsv2.EntryInput{
		Ctx:                ctx,
		ClientConn:         clientFrameConn,
		UpstreamConn:       upstreamFrameConn,
		FirstClientMessage: firstClientMessage,
		Options: openaiwsv2.RelayOptions{
			WriteTimeout:            s.openAIWSWriteTimeout(),
			IdleTimeout:             s.openAIWSPassthroughIdleTimeout(),
			FirstMessageType:        coderws.MessageText,
			FirstMessageSent:        upstreamFirstMessageSent,
			FirstMessageStartedAt:   firstMessageStartedAt,
			InitialRequestModel:     initialRequestModel,
			ValidatedFirstEnvelope:  &firstEnvelope,
			ReadClientFrame:         readClientFrame,
			RequireClientTextFrames: true,
			BeforeInspectUpstreamFrame: func(msgType coderws.MessageType, payload []byte) error {
				if msgType != coderws.MessageText {
					return NewOpenAIWSClientCloseError(
						coderws.StatusUnsupportedData,
						"websocket binary request frames are unsupported",
						errors.New("client websocket request frame was binary"),
					)
				}
				if !aetherCapability.Effective {
					return nil
				}
				if sizeErr := validateAetherWSClientPayload(payload); sizeErr != nil {
					return NewOpenAIWSClientCloseError(coderws.StatusMessageTooBig, "websocket request payload is too large", sizeErr)
				}
				return nil
			},
			BeforeWriteUpstreamFrame: func(_ coderws.MessageType, _ []byte, envelope openaiwsv2.ClientEnvelope) error {
				if freezeErr := validateOpenAIWSPassthroughSessionModel(envelope, originalRequestModel); freezeErr != nil {
					return freezeErr
				}
				if envelope.Type != "session.update" || !envelope.HasSessionModel {
					return nil
				}
				usageMeta.sessionRequestModel = envelope.SessionModel
				return nil
			},
			TransformUpstreamFrame: func(_ coderws.MessageType, payload []byte, envelope openaiwsv2.ClientEnvelope) ([]byte, error) {
				updated, transformErr := transformOpenAIWSPassthroughSessionModel(payload, envelope, frozenRequestModel)
				if transformErr != nil {
					return nil, NewOpenAIWSClientCloseError(
						coderws.StatusPolicyViolation,
						"invalid websocket session update",
						transformErr,
					)
				}
				return updated, nil
			},
			BeforeWriteResponseCreate: func(_ coderws.MessageType, payload []byte, originalModel string) error {
				turnNo := int(completedTurns.Load()) + 1
				if turnNo < 2 {
					turnNo = 2
				}
				if originalModel != "" && originalModel != originalRequestModel {
					return NewOpenAIWSClientCloseError(
						coderws.StatusPolicyViolation,
						"changing model requires a new websocket connection",
						nil,
					)
				}
				if hooks != nil && hooks.BeforeRequest != nil {
					if err := hooks.BeforeRequest(turnNo, payload, originalModel); err != nil {
						return err
					}
				}
				if hooks != nil && hooks.BeforeTurn != nil {
					return hooks.BeforeTurn(turnNo)
				}
				return nil
			},
			TransformResponseCreate: func(payload []byte, envelope openaiwsv2.ClientEnvelope) ([]byte, error) {
				turnNo := int(completedTurns.Load()) + 1
				if turnNo < 2 {
					turnNo = 2
				}
				requestModelForThisFrame := usageMeta.requestModelForEnvelope(envelope)
				policyDecision, policyBlocked := s.evaluateOpenAIFastPolicyForWS(
					ctx,
					account,
					capturedSessionModel,
					envelope.ServiceTier,
					envelope.HasServiceTier,
				)
				if policyBlocked != nil {
					onPolicyBlock(policyBlocked)
					return nil, NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, policyBlocked.Message, policyBlocked)
				}
				// The normal Aether path has no transform hook after this point. Fold
				// policy tier mutation into the same route-fence/model edit pass so a
				// filtered or aliased tier does not create a second payload-sized copy.
				if aetherRouteControl != nil && (hooks == nil || hooks.TransformRequest == nil) {
					policyEnvelope := policyDecision.applyToEnvelope(envelope)
					usageMeta.updateFromEnvelope(policyEnvelope, requestModelForThisFrame)
					return aetherRouteControl.prepareValidatedResponseCreateWithEnvelopeAndModelAndServiceTier(
						payload,
						envelope,
						frozenRequestModel,
						policyDecision.serviceTierMutation(),
					)
				}
				validatedPayload := payload
				filtered, policyErr := applyOpenAIWSFastPolicyDecision(payload, policyDecision)
				if policyErr != nil {
					return nil, fmt.Errorf("apply openai fast policy to ws frame: %w", policyErr)
				}
				payload = filtered
				currentModel := requestModelForThisFrame
				mustReparseEnvelope := !sameOpenAIWSPayload(validatedPayload, payload)
				if hooks != nil && hooks.TransformRequest != nil {
					payload, err = hooks.TransformRequest(turnNo, payload)
					if err != nil {
						return nil, err
					}
					// A transform hook is not trusted to preserve the frozen physical
					// model. Force a final set instead of using a stale pre-hook hint.
					currentModel = ""
					mustReparseEnvelope = true
				}
				transformEnvelope := envelope
				if mustReparseEnvelope {
					transformEnvelope, err = ParseOpenAIWSClientEnvelope(payload)
					if err != nil {
						return nil, err
					}
				}
				usageMeta.updateFromEnvelope(transformEnvelope, requestModelForThisFrame)
				if aetherRouteControl == nil {
					return transformOpenAIWSPassthroughResponseModel(payload, currentModel, frozenRequestModel)
				}
				return aetherRouteControl.prepareValidatedResponseCreateWithEnvelopeAndModel(
					payload,
					transformEnvelope,
					frozenRequestModel,
				)
			},
			BeforeDispatchResponseCreate: func(_ coderws.MessageType, payload []byte, originalModel string) error {
				if aetherCapability.Effective {
					if sizeErr := validateAetherWSRoutedPayload(payload); sizeErr != nil {
						return NewOpenAIWSClientCloseError(coderws.StatusMessageTooBig, "websocket request payload is too large", sizeErr)
					}
				}
				turnNo := int(completedTurns.Load()) + 1
				if turnNo < 2 {
					turnNo = 2
				}
				return runOpenAIWSBeforeProviderWrite(hooks, turnNo, payload, originalModel)
			},
			InterceptUpstreamFrame: func(msgType coderws.MessageType, payload []byte) openaiwsv2.UpstreamFrameDirective {
				if aetherRouteControl == nil || msgType != coderws.MessageText {
					return openaiwsv2.UpstreamFrameDirective{}
				}
				consumed, decision, controlErr := aetherRouteControl.consumeUpstreamFrame(payload)
				if !consumed {
					return openaiwsv2.UpstreamFrameDirective{}
				}
				directive := openaiwsv2.UpstreamFrameDirective{Consume: true}
				if controlErr != nil {
					directive.Err = NewOpenAIWSClientCloseError(
						coderws.StatusInternalError,
						"aether route control protocol error",
						controlErr,
					)
					return directive
				}
				if decision.CloseAfterTerminal {
					directive.CloseAfterTerminal = true
					return directive
				}
				if !decision.SignalReconnect {
					directive.Err = NewOpenAIWSClientCloseError(
						coderws.StatusInternalError,
						"aether route control protocol error",
						errors.New("aether route control produced no supported action"),
					)
					return directive
				}
				return buildAetherWSReconnectDirective(hooks, decision, handshakeHeaders)
			},
			OnProviderFrameWritten: func(terminal bool) {
				if aetherRouteControl != nil {
					aetherRouteControl.markProviderFrameWritten(terminal)
				}
			},
			OnUsageParseFailure: func(eventType string, usageRaw string) {
				logOpenAIWSV2Passthrough(
					"usage_parse_failed event_type=%s usage_raw=%s",
					truncateOpenAIWSLogValue(eventType, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(usageRaw, openAIWSLogValueMaxLen),
				)
			},
			OnTurnComplete: func(turn openaiwsv2.RelayTurnResult) {
				turnNo := int(completedTurns.Add(1))
				turnResult := &OpenAIForwardResult{
					RequestID: turn.RequestID,
					Usage: OpenAIUsage{
						InputTokens:              turn.Usage.InputTokens,
						OutputTokens:             turn.Usage.OutputTokens,
						CacheCreationInputTokens: turn.Usage.CacheCreationInputTokens,
						CacheReadInputTokens:     turn.Usage.CacheReadInputTokens,
						ImageOutputTokens:        turn.Usage.ImageOutputTokens,
					},
					Model:             turn.RequestModel,
					UpstreamModel:     frozenRequestModel,
					ServiceTier:       usageMeta.serviceTier.Load(),
					ReasoningEffort:   usageMeta.reasoningEffort.Load(),
					Stream:            true,
					OpenAIWSMode:      true,
					TerminalEventType: turn.TerminalEventType,
					ResponseHeaders:   cloneHeader(handshakeHeaders),
					Duration:          turn.Duration,
					FirstTokenMs:      turn.FirstTokenMs,
				}
				logOpenAIWSV2Passthrough(
					"relay_turn_completed account_id=%d turn=%d request_id=%s terminal_event=%s duration_ms=%d first_token_ms=%d input_tokens=%d output_tokens=%d cache_read_tokens=%d",
					account.ID,
					turnNo,
					truncateOpenAIWSLogValue(turnResult.RequestID, openAIWSIDValueMaxLen),
					truncateOpenAIWSLogValue(turn.TerminalEventType, openAIWSLogValueMaxLen),
					turnResult.Duration.Milliseconds(),
					openAIWSFirstTokenMsForLog(turnResult.FirstTokenMs),
					turnResult.Usage.InputTokens,
					turnResult.Usage.OutputTokens,
					turnResult.Usage.CacheReadInputTokens,
				)
				if hooks != nil && hooks.AfterTurn != nil {
					hooks.AfterTurn(turnNo, turnResult, nil)
				}
			},
			BeforeWriteClient: func(msgType coderws.MessageType, payload []byte, wroteDownstream bool) error {
				if msgType != coderws.MessageText || wroteDownstream {
					return nil
				}
				if eventType, _, _ := parseOpenAIWSEventEnvelope(payload); eventType != "error" {
					return nil
				}
				errCodeRaw, errTypeRaw, errMsgRaw := parseOpenAIWSErrorEventFields(payload)
				if !isOpenAIWSRateLimitError(errCodeRaw, errTypeRaw, errMsgRaw) {
					return nil
				}
				if aetherRouteControl != nil {
					// The official Codex key/provider belongs to Aether. A valid
					// provider error proves the middle hop worked and must not rate
					// limit the local Aether account in sub2api.
					logOpenAIWSV2Passthrough(
						"relay_aether_provider_rate_limit account_id=%d err_code=%s err_type=%s",
						account.ID,
						truncateOpenAIWSLogValue(errCodeRaw, openAIWSLogValueMaxLen),
						truncateOpenAIWSLogValue(errTypeRaw, openAIWSLogValueMaxLen),
					)
					return nil
				}
				s.persistOpenAIWSRateLimitSignal(ctx, account, handshakeHeaders, payload, errCodeRaw, errTypeRaw, errMsgRaw)
				logOpenAIWSV2Passthrough(
					"relay_rate_limit_failover account_id=%d err_code=%s err_type=%s err_message=%s",
					account.ID,
					truncateOpenAIWSLogValue(errCodeRaw, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(errTypeRaw, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(errMsgRaw, openAIWSLogValueMaxLen),
				)
				failoverErr := &UpstreamFailoverError{
					StatusCode:      http.StatusTooManyRequests,
					ResponseBody:    append([]byte(nil), payload...),
					ResponseHeaders: cloneHeader(handshakeHeaders),
				}
				return openAIWSRateLimitFailoverError(
					int(completedTurns.Load())+1,
					aetherRouteControl != nil,
					failoverErr,
				)
			},
			OnTrace: func(event openaiwsv2.RelayTraceEvent) {
				logOpenAIWSV2Passthrough(
					"relay_trace account_id=%d stage=%s direction=%s msg_type=%s bytes=%d graceful=%v wrote_downstream=%v err=%s",
					account.ID,
					truncateOpenAIWSLogValue(event.Stage, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(event.Direction, openAIWSLogValueMaxLen),
					truncateOpenAIWSLogValue(event.MessageType, openAIWSLogValueMaxLen),
					event.PayloadBytes,
					event.Graceful,
					event.WroteDownstream,
					truncateOpenAIWSLogValue(event.Error, openAIWSLogValueMaxLen),
				)
			},
		},
	})

	result := &OpenAIForwardResult{
		RequestID: relayResult.RequestID,
		Usage: OpenAIUsage{
			InputTokens:              relayResult.Usage.InputTokens,
			OutputTokens:             relayResult.Usage.OutputTokens,
			CacheCreationInputTokens: relayResult.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     relayResult.Usage.CacheReadInputTokens,
			ImageOutputTokens:        relayResult.Usage.ImageOutputTokens,
		},
		Model:             relayResult.RequestModel,
		UpstreamModel:     frozenRequestModel,
		ServiceTier:       usageMeta.serviceTier.Load(),
		ReasoningEffort:   usageMeta.reasoningEffort.Load(),
		Stream:            true,
		OpenAIWSMode:      true,
		TerminalEventType: relayResult.TerminalEventType,
		ResponseHeaders:   cloneHeader(handshakeHeaders),
		Duration:          relayResult.Duration,
		FirstTokenMs:      relayResult.FirstTokenMs,
	}

	turnCount := int(completedTurns.Load())
	if relayExit == nil {
		logOpenAIWSV2Passthrough(
			"relay_completed account_id=%d request_id=%s terminal_event=%s duration_ms=%d c2u_frames=%d u2c_frames=%d dropped_frames=%d turns=%d",
			account.ID,
			truncateOpenAIWSLogValue(result.RequestID, openAIWSIDValueMaxLen),
			truncateOpenAIWSLogValue(relayResult.TerminalEventType, openAIWSLogValueMaxLen),
			result.Duration.Milliseconds(),
			relayResult.ClientToUpstreamFrames,
			relayResult.UpstreamToClientFrames,
			relayResult.DroppedDownstreamFrames,
			turnCount,
		)
		// 正常路径按 terminal 事件逐 turn 已回调；仅在零 turn 场景兜底回调一次。
		if turnCount == 0 && hooks != nil && hooks.AfterTurn != nil {
			hooks.AfterTurn(1, result, nil)
		}
		return nil
	}
	logOpenAIWSV2Passthrough(
		"relay_failed account_id=%d stage=%s wrote_downstream=%v err=%s duration_ms=%d c2u_frames=%d u2c_frames=%d dropped_frames=%d turns=%d",
		account.ID,
		truncateOpenAIWSLogValue(relayExit.Stage, openAIWSLogValueMaxLen),
		relayExit.WroteDownstream,
		truncateOpenAIWSLogValue(relayErrorText(relayExit.Err), openAIWSLogValueMaxLen),
		result.Duration.Milliseconds(),
		relayResult.ClientToUpstreamFrames,
		relayResult.UpstreamToClientFrames,
		relayResult.DroppedDownstreamFrames,
		turnCount,
	)

	relayErr := relayExit.Err
	if relayExit.Stage == "idle_timeout" {
		relayErr = NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"client websocket idle timeout",
			relayErr,
		)
	}
	turnErr := wrapOpenAIWSIngressTurnError(
		relayExit.Stage,
		relayErr,
		relayExit.WroteDownstream,
	)
	if hooks != nil && hooks.AfterTurn != nil {
		hooks.AfterTurn(turnCount+1, nil, turnErr)
	}
	return turnErr
}

func buildAetherWSReconnectDirective(
	hooks *OpenAIWSIngressHooks,
	decision aetherWSRouteControlDecision,
	handshakeHeaders http.Header,
) openaiwsv2.UpstreamFrameDirective {
	directive := openaiwsv2.UpstreamFrameDirective{Consume: true}
	if decision.InitialStepFailover {
		directive.Err = NewOpenAIWSInitialStepFailoverError(&UpstreamFailoverError{
			StatusCode:             http.StatusServiceUnavailable,
			ResponseHeaders:        cloneHeader(handshakeHeaders),
			DoNotPenalizeAccount:   decision.MiddleRouteDisposition == OpenAIWSMiddleRouteDispositionRetain,
			RetryAfterMS:           decision.RetryAfterMS,
			MiddleRouteDisposition: decision.MiddleRouteDisposition,
		})
		return directive
	}
	if hooks == nil || hooks.BeforeReconnectSignal == nil {
		directive.Err = NewOpenAIWSClientCloseError(
			coderws.StatusInternalError,
			"aether route migration admission unavailable",
			fmt.Errorf("%w: aether reconnect pre-signal hook is unavailable", ErrOpenAIWSLocalAdmission),
		)
		return directive
	}
	if admissionErr := hooks.BeforeReconnectSignal(OpenAIWSReconnectControl{
		ControlID:              decision.ControlID,
		BindingGeneration:      decision.BindingGeneration,
		MiddleRouteDisposition: decision.MiddleRouteDisposition,
	}); admissionErr != nil {
		directive.Err = NewOpenAIWSClientCloseError(
			coderws.StatusTryAgainLater,
			"aether route migration was not admitted",
			fmt.Errorf("%w: %w", ErrOpenAIWSLocalAdmission, admissionErr),
		)
		return directive
	}
	// The synthetic client payload is populated only after the shared-state
	// admission returns successfully. This ordering is the replay-safety fence.
	directive.ClientMessageType = coderws.MessageText
	directive.ClientPayload = aetherWSReconnectErrorPayload()
	directive.Exit = true
	directive.Err = NewOpenAIWSClientCloseError(
		coderws.StatusNormalClosure,
		"aether route migration requested",
		ErrOpenAIWSReconnectMigrationRequested,
	)
	return directive
}

func (s *OpenAIGatewayService) mapOpenAIWSPassthroughDialError(
	err error,
	statusCode int,
	handshakeHeaders http.Header,
) error {
	if err == nil {
		return nil
	}
	wrappedErr := err
	var dialErr *openAIWSDialError
	if !errors.As(err, &dialErr) {
		wrappedErr = &openAIWSDialError{
			StatusCode:      statusCode,
			ResponseHeaders: cloneHeader(handshakeHeaders),
			Err:             err,
		}
	}

	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewOpenAIWSClientCloseError(
			coderws.StatusTryAgainLater,
			"upstream websocket connect timeout",
			wrappedErr,
		)
	}
	if statusCode == http.StatusTooManyRequests {
		return NewOpenAIWSClientCloseError(
			coderws.StatusTryAgainLater,
			"upstream websocket is busy, please retry later",
			wrappedErr,
		)
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"upstream websocket authentication failed",
			wrappedErr,
		)
	}
	if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
		return NewOpenAIWSClientCloseError(
			coderws.StatusPolicyViolation,
			"upstream websocket handshake rejected",
			wrappedErr,
		)
	}
	return fmt.Errorf("openai ws passthrough dial: %w", wrappedErr)
}

func openaiwsv2RelayMessageTypeName(msgType coderws.MessageType) string {
	switch msgType {
	case coderws.MessageText:
		return "text"
	case coderws.MessageBinary:
		return "binary"
	default:
		return fmt.Sprintf("unknown(%d)", msgType)
	}
}

func relayErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func openAIWSFirstTokenMsForLog(firstTokenMs *int) int {
	if firstTokenMs == nil {
		return -1
	}
	return *firstTokenMs
}

func logOpenAIWSV2Passthrough(format string, args ...any) {
	logger.LegacyPrintf(
		"service.openai_ws_v2",
		"[OpenAI WS v2 passthrough] %s "+format,
		append([]any{openaiWSV2PassthroughModeFields}, args...)...,
	)
}
