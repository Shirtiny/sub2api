package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	aetherWSRouteControlEventType      = "aether.route_control"
	aetherWSRouteControlVersion        = 1
	aetherWSBindingGenerationInitial   = uint64(1)
	aetherWSRouteControlMaxFrameBytes  = 16 * 1024
	aetherWSRouteControlMaxIDBytes     = 160
	aetherWSRouteControlMaxReasonBytes = 512
	aetherWSRouteControlMaxRetryMS     = 60_000
	aetherWSRouteControlSeenLimit      = 64

	aetherWSRouteActionCloseAfterTerminal = "close_after_terminal"
	aetherWSRouteActionClientReconnect    = "client_reconnect"

	// These values identify the only adapter-reviewed proof accepted by this
	// route-v1 consumer. A non-empty arbitrary string is not execution proof.
	AetherWSAdapterProofClassCodexOfficialNotExecuted = "codex_official_ws.not_executed"
	AetherWSAdapterProofVersionV1                     = 1
)

var (
	aetherWSRouteControlMarker        = []byte(aetherWSRouteControlEventType)
	aetherWSRouteControlUnicodeEscape = []byte(`\u`)
)

const aetherWSReconnectErrorEvent = `{"type":"error","status":400,"error":{"type":"invalid_request_error","code":"websocket_connection_limit_reached","message":"Responses websocket connection limit reached (60 minutes). Create a new websocket connection to continue."}}`

type aetherWSRouteControlConsumerConfig struct {
	Negotiated          AetherWSNegotiatedCapabilities
	ReconnectEnabled    bool
	ReconnectSignalMode string
	BindingEpochID      string
	BindingGeneration   uint64
}

type aetherWSStepFence struct {
	Version                  int    `json:"version"`
	Sub2APIStepCorrelationID string `json:"sub2api_step_correlation_id"`
	Sub2APIBindingEpochID    string `json:"sub2api_binding_epoch_id"`
	Sub2APIBindingGeneration uint64 `json:"sub2api_binding_generation"`
}

type aetherWSRouteControlFrame struct {
	Type                         string                          `json:"type"`
	Version                      int                             `json:"version"`
	Action                       string                          `json:"action"`
	ControlID                    string                          `json:"control_id"`
	Scope                        string                          `json:"scope"`
	EffectiveAfter               string                          `json:"effective_after"`
	Reason                       string                          `json:"reason"`
	Sub2APIStepCorrelationID     string                          `json:"sub2api_step_correlation_id"`
	Sub2APIBindingEpochID        string                          `json:"sub2api_binding_epoch_id"`
	Sub2APIBindingGeneration     uint64                          `json:"sub2api_binding_generation"`
	AetherStepID                 string                          `json:"aether_step_id"`
	AetherAttemptID              string                          `json:"aether_attempt_id"`
	CurrentAttemptState          string                          `json:"current_attempt_state"`
	ProviderWriteState           string                          `json:"provider_write_state"`
	ProviderExecutionDisposition string                          `json:"provider_execution_disposition"`
	AdapterProofClass            *string                         `json:"adapter_proof_class"`
	AdapterProofVersion          *int                            `json:"adapter_proof_version"`
	RetryAfterMS                 int                             `json:"retry_after_ms"`
	RecommendedAction            string                          `json:"recommended_action"`
	MiddleRouteDisposition       *OpenAIWSMiddleRouteDisposition `json:"middle_route_disposition,omitempty"`
	ProviderFallbackUsed         bool                            `json:"provider_fallback_used,omitempty"`
}

type aetherWSRouteControlDecision struct {
	Action                 string
	ControlID              string
	BindingGeneration      uint64
	RetryAfterMS           int
	MiddleRouteDisposition OpenAIWSMiddleRouteDisposition
	CloseAfterTerminal     bool
	SignalReconnect        bool
	InitialStepFailover    bool
}

type aetherWSRouteControlIdentity struct {
	Action                       string
	Scope                        string
	EffectiveAfter               string
	Reason                       string
	CorrelationID                string
	BindingEpochID               string
	BindingGeneration            uint64
	AetherStepID                 string
	AetherAttemptID              string
	CurrentAttemptState          string
	ProviderWriteState           string
	ProviderExecutionDisposition string
	AdapterProofClass            string
	AdapterProofVersion          int
	RetryAfterMS                 int
	RecommendedAction            string
	MiddleRouteDisposition       OpenAIWSMiddleRouteDisposition
	ProviderFallbackUsed         bool
}

type aetherWSSeenRouteControl struct {
	identity aetherWSRouteControlIdentity
	decision aetherWSRouteControlDecision
}

// aetherWSRouteControlConsumer owns one physical sub2api -> Aether binding.
// The mutex is used only on response.create/control events. Provider delta
// frames touch two atomics and never allocate, parse JSON, or take this lock.
type aetherWSRouteControlConsumer struct {
	enabled           bool
	reconnectEnabled  bool
	bindingEpochID    string
	bindingGeneration uint64

	mu         sync.Mutex
	stepNumber int
	active     aetherWSStepFence
	seen       map[string]aetherWSSeenRouteControl
	seenOrder  []string

	providerOutputStarted atomic.Bool
	terminalWritten       atomic.Bool
}

func newAetherWSRouteControlConsumer(cfg aetherWSRouteControlConsumerConfig) (*aetherWSRouteControlConsumer, error) {
	enabled := cfg.Negotiated.ControlProtocol == AetherWSControlProtocolRouteV1 &&
		cfg.Negotiated.CloseAfterTerminal && cfg.Negotiated.ClientReconnect
	if !enabled {
		return nil, errors.New("aether route-v1 capabilities were not negotiated")
	}
	bindingEpochID := strings.TrimSpace(cfg.BindingEpochID)
	if bindingEpochID == "" {
		bindingEpochID = uuid.NewString()
	}
	if !validAetherWSOpaqueID(bindingEpochID) {
		return nil, errors.New("invalid aether binding epoch id")
	}
	bindingGeneration := cfg.BindingGeneration
	if bindingGeneration == 0 {
		bindingGeneration = aetherWSBindingGenerationInitial
	}
	reconnectEnabled := cfg.ReconnectEnabled &&
		strings.EqualFold(strings.TrimSpace(cfg.ReconnectSignalMode), "websocket_connection_limit_reached")
	return &aetherWSRouteControlConsumer{
		enabled:           true,
		reconnectEnabled:  reconnectEnabled,
		bindingEpochID:    bindingEpochID,
		bindingGeneration: bindingGeneration,
		seen:              make(map[string]aetherWSSeenRouteControl, 4),
		seenOrder:         make([]string, 0, 4),
	}, nil
}

func (c *aetherWSRouteControlConsumer) prepareResponseCreate(payload []byte) ([]byte, error) {
	if c == nil || !c.enabled {
		return nil, errors.New("aether route control consumer is disabled")
	}
	if !gjson.ValidBytes(payload) || strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "response.create" {
		return nil, errors.New("aether step fence requires a response.create object")
	}
	return c.prepareValidatedResponseCreate(payload)
}

func (c *aetherWSRouteControlConsumer) prepareValidatedResponseCreate(payload []byte) ([]byte, error) {
	metadata := gjson.GetBytes(payload, "client_metadata")
	return c.prepareValidatedResponseCreateWithMetadata(payload, metadata.Exists(), metadata.IsObject())
}

func (c *aetherWSRouteControlConsumer) prepareValidatedResponseCreateWithEnvelope(
	payload []byte,
	envelope openaiwsv2.ClientEnvelope,
) ([]byte, error) {
	return c.prepareValidatedResponseCreateWithMetadataAndModel(
		payload,
		envelope.HasClientMetadata,
		envelope.ClientMetadataIsObject,
		envelope,
		"",
		nil,
	)
}

func (c *aetherWSRouteControlConsumer) prepareValidatedResponseCreateWithEnvelopeAndModel(
	payload []byte,
	envelope openaiwsv2.ClientEnvelope,
	upstreamModel string,
) ([]byte, error) {
	return c.prepareValidatedResponseCreateWithEnvelopeAndModelAndServiceTier(payload, envelope, upstreamModel, nil)
}

func (c *aetherWSRouteControlConsumer) prepareValidatedResponseCreateWithEnvelopeAndModelAndServiceTier(
	payload []byte,
	envelope openaiwsv2.ClientEnvelope,
	upstreamModel string,
	tierMutation *aetherWSServiceTierMutation,
) ([]byte, error) {
	return c.prepareValidatedResponseCreateWithMetadataAndModel(
		payload,
		envelope.HasClientMetadata,
		envelope.ClientMetadataIsObject,
		envelope,
		upstreamModel,
		tierMutation,
	)
}

func (c *aetherWSRouteControlConsumer) prepareValidatedResponseCreateWithMetadata(
	payload []byte,
	hasClientMetadata bool,
	clientMetadataIsObject bool,
) ([]byte, error) {
	return c.prepareValidatedResponseCreateWithMetadataAndModel(
		payload,
		hasClientMetadata,
		clientMetadataIsObject,
		openaiwsv2.ClientEnvelope{},
		"",
		nil,
	)
}

func (c *aetherWSRouteControlConsumer) prepareValidatedResponseCreateWithMetadataAndModel(
	payload []byte,
	hasClientMetadata bool,
	clientMetadataIsObject bool,
	envelope openaiwsv2.ClientEnvelope,
	upstreamModel string,
	tierMutation *aetherWSServiceTierMutation,
) ([]byte, error) {
	if c == nil || !c.enabled {
		return nil, errors.New("aether route control consumer is disabled")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.stepNumber++
	c.active = aetherWSStepFence{
		Version:                  aetherWSRouteControlVersion,
		Sub2APIStepCorrelationID: uuid.NewString(),
		Sub2APIBindingEpochID:    c.bindingEpochID,
		Sub2APIBindingGeneration: c.bindingGeneration,
	}
	c.providerOutputStarted.Store(false)
	c.terminalWritten.Store(false)

	fenceJSON, err := json.Marshal(c.active)
	if err != nil {
		return nil, fmt.Errorf("marshal aether step fence: %w", err)
	}
	if !hasClientMetadata {
		return appendAetherWSClientMetadataFenceAndModelAndTier(payload, fenceJSON, envelope, upstreamModel, tierMutation)
	}
	if clientMetadataIsObject && !envelope.HasAetherStepControl {
		if _, _, ok := envelope.ClientMetadataValueRange(); ok {
			return appendAetherWSMetadataObjectFenceAndModelAndTier(payload, fenceJSON, envelope, upstreamModel, tierMutation)
		}
	}
	updated := payload
	if strings.TrimSpace(upstreamModel) != "" {
		updated, err = transformOpenAIWSPassthroughResponseModel(payload, envelope.Model, upstreamModel)
		if err != nil {
			return nil, err
		}
	}
	if tierMutation != nil && (tierMutation.remove || tierMutation.replacement != "") {
		var tierErr error
		if tierMutation.remove {
			updated, tierErr = sjson.DeleteBytes(updated, "service_tier")
		} else {
			updated, tierErr = sjson.SetBytes(updated, "service_tier", tierMutation.replacement)
		}
		if tierErr != nil {
			return nil, fmt.Errorf("apply service_tier mutation: %w", tierErr)
		}
	}
	if !clientMetadataIsObject {
		replacement := make([]byte, 0, len(fenceJSON)+48)
		replacement = append(replacement, `{"aether.sub2api_step_control":`...)
		replacement = append(replacement, fenceJSON...)
		replacement = append(replacement, '}')
		updated, setErr := sjson.SetRawBytes(updated, "client_metadata", replacement)
		if setErr != nil {
			return nil, fmt.Errorf("normalize aether step metadata: %w", setErr)
		}
		return updated, nil
	}
	updated, err = sjson.SetRawBytes(updated, "client_metadata.aether\\.sub2api_step_control", fenceJSON)
	if err != nil {
		return nil, fmt.Errorf("inject aether step fence: %w", err)
	}
	return updated, nil
}

type aetherWSJSONEdit struct {
	start       int
	end         int
	replacement []byte
}

// aetherWSServiceTierMutation is folded into the route-fence/model edit pass
// so the common Aether turn needs only one payload-sized allocation.
type aetherWSServiceTierMutation struct {
	remove      bool
	replacement string
}

func appendAetherWSMetadataObjectFenceAndModelAndTier(
	payload []byte,
	fenceJSON []byte,
	envelope openaiwsv2.ClientEnvelope,
	upstreamModel string,
	tierMutation *aetherWSServiceTierMutation,
) ([]byte, error) {
	metadataStart, metadataEnd, ok := envelope.ClientMetadataValueRange()
	if !ok || metadataStart < 0 || metadataEnd > len(payload) || metadataEnd-metadataStart < 2 ||
		payload[metadataStart] != '{' || payload[metadataEnd-1] != '}' {
		return nil, errors.New("aether client_metadata range is invalid")
	}

	metadataEmpty := true
	for _, value := range payload[metadataStart+1 : metadataEnd-1] {
		switch value {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			metadataEmpty = false
		}
		break
	}
	fencePrefix := []byte(`,"aether.sub2api_step_control":`)
	if metadataEmpty {
		fencePrefix = fencePrefix[1:]
	}
	fenceInsertion := make([]byte, 0, len(fencePrefix)+len(fenceJSON))
	fenceInsertion = append(fenceInsertion, fencePrefix...)
	fenceInsertion = append(fenceInsertion, fenceJSON...)
	edits := [3]aetherWSJSONEdit{{
		start:       metadataEnd - 1,
		end:         metadataEnd - 1,
		replacement: fenceInsertion,
	}}
	editCount := 1

	if tierMutation != nil && (tierMutation.remove || tierMutation.replacement != "") {
		if tierMutation.remove {
			start, end, ok := envelope.ServiceTierFieldRange()
			if !ok {
				return nil, errors.New("aether service_tier range is invalid")
			}
			edits[editCount] = aetherWSJSONEdit{start: start, end: end}
		} else {
			start, end, ok := envelope.ServiceTierValueRange()
			if !ok {
				return nil, errors.New("aether service_tier value range is invalid")
			}
			edits[editCount] = aetherWSJSONEdit{start: start, end: end, replacement: strconv.AppendQuote(nil, tierMutation.replacement)}
		}
		editCount++
	}
	if strings.TrimSpace(upstreamModel) != "" && strings.TrimSpace(envelope.Model) != strings.TrimSpace(upstreamModel) {
		quotedModel := strconv.AppendQuote(nil, upstreamModel)
		if modelStart, modelEnd, hasModelRange := envelope.ModelValueRange(); hasModelRange {
			edits[editCount] = aetherWSJSONEdit{start: modelStart, end: modelEnd, replacement: quotedModel}
		} else {
			topEnd := len(payload) - 1
			for topEnd >= 0 {
				switch payload[topEnd] {
				case ' ', '\t', '\r', '\n':
					topEnd--
					continue
				}
				break
			}
			if topEnd < 0 || payload[topEnd] != '}' {
				return nil, errors.New("aether response.create range is invalid")
			}
			modelInsertion := append([]byte(`,"model":`), quotedModel...)
			edits[editCount] = aetherWSJSONEdit{start: topEnd, end: topEnd, replacement: modelInsertion}
		}
		editCount++
	}
	return applyAetherWSJSONEdits(payload, edits[:editCount])
}

func appendAetherWSClientMetadataFenceAndModelAndTier(
	payload []byte,
	fenceJSON []byte,
	envelope openaiwsv2.ClientEnvelope,
	upstreamModel string,
	tierMutation *aetherWSServiceTierMutation,
) ([]byte, error) {
	end := len(payload) - 1
	for end >= 0 {
		switch payload[end] {
		case ' ', '\t', '\r', '\n':
			end--
			continue
		}
		break
	}
	if end < 0 || payload[end] != '}' {
		return nil, errors.New("aether step fence requires a top-level JSON object")
	}
	const metadataPrefix = `,"client_metadata":{"aether.sub2api_step_control":`
	quotedModel := []byte(nil)
	modelStart, modelEnd, hasModelRange := envelope.ModelValueRange()
	replaceModel := strings.TrimSpace(upstreamModel) != "" && strings.TrimSpace(envelope.Model) != strings.TrimSpace(upstreamModel)
	if replaceModel {
		quotedModel = strconv.AppendQuote(nil, upstreamModel)
	}
	if hasModelRange && (modelStart < 0 || modelEnd > end || modelEnd <= modelStart) {
		return nil, errors.New("aether step fence model range is invalid")
	}
	edits := [3]aetherWSJSONEdit{}
	editCount := 0
	if replaceModel && hasModelRange {
		edits[editCount] = aetherWSJSONEdit{start: modelStart, end: modelEnd, replacement: quotedModel}
		editCount++
	}
	metadataInsertion := make([]byte, 0, len(metadataPrefix)+len(fenceJSON)+1+len(`,"model":`)+len(quotedModel))
	if replaceModel && !hasModelRange {
		metadataInsertion = append(metadataInsertion, `,"model":`...)
		metadataInsertion = append(metadataInsertion, quotedModel...)
	}
	metadataInsertion = append(metadataInsertion, metadataPrefix...)
	metadataInsertion = append(metadataInsertion, fenceJSON...)
	metadataInsertion = append(metadataInsertion, '}')
	edits[editCount] = aetherWSJSONEdit{start: end, end: end, replacement: metadataInsertion}
	editCount++
	if tierMutation != nil && (tierMutation.remove || tierMutation.replacement != "") {
		if tierMutation.remove {
			start, tierEnd, ok := envelope.ServiceTierFieldRange()
			if !ok {
				return nil, errors.New("aether service_tier range is invalid")
			}
			edits[editCount] = aetherWSJSONEdit{start: start, end: tierEnd}
		} else {
			start, tierEnd, ok := envelope.ServiceTierValueRange()
			if !ok {
				return nil, errors.New("aether service_tier value range is invalid")
			}
			edits[editCount] = aetherWSJSONEdit{start: start, end: tierEnd, replacement: strconv.AppendQuote(nil, tierMutation.replacement)}
		}
		editCount++
	}
	return applyAetherWSJSONEdits(payload, edits[:editCount])
}

func applyAetherWSJSONEdits(payload []byte, edits []aetherWSJSONEdit) ([]byte, error) {
	for index := 1; index < len(edits); index++ {
		current := edits[index]
		position := index
		for position > 0 && current.start < edits[position-1].start {
			edits[position] = edits[position-1]
			position--
		}
		edits[position] = current
	}
	extra := 0
	for index, edit := range edits {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(payload) ||
			(index > 0 && edit.start < edits[index-1].end) {
			return nil, errors.New("aether response.create edits overlap")
		}
		extra += len(edit.replacement) - (edit.end - edit.start)
	}
	updated := make([]byte, 0, len(payload)+extra)
	cursor := 0
	for _, edit := range edits {
		updated = append(updated, payload[cursor:edit.start]...)
		updated = append(updated, edit.replacement...)
		cursor = edit.end
	}
	updated = append(updated, payload[cursor:]...)
	return updated, nil
}

// consumeUpstreamFrame performs a cheap marker/type scan for ordinary frames.
// Only an exact reserved control type enters the bounded strict JSON decoder.
func (c *aetherWSRouteControlConsumer) consumeUpstreamFrame(payload []byte) (bool, aetherWSRouteControlDecision, error) {
	if c == nil || !c.enabled || !aetherWSMayContainReservedType(payload) {
		return false, aetherWSRouteControlDecision{}, nil
	}
	fastInspection, _ := openaiwsv2.InspectFirstTopLevelStringField(payload, "type", aetherWSRouteControlEventType)
	if fastInspection.Matched && (len(payload) == 0 || len(payload) > aetherWSRouteControlMaxFrameBytes) {
		return true, aetherWSRouteControlDecision{}, errors.New("aether route control frame size is invalid")
	}
	inspection, inspectErr := openaiwsv2.InspectTopLevelStringField(payload, "type", aetherWSRouteControlEventType)
	if !inspection.Matched {
		return false, aetherWSRouteControlDecision{}, nil
	}
	if len(payload) == 0 || len(payload) > aetherWSRouteControlMaxFrameBytes {
		return true, aetherWSRouteControlDecision{}, errors.New("aether route control frame size is invalid")
	}
	if inspectErr != nil {
		return true, aetherWSRouteControlDecision{}, fmt.Errorf("inspect aether route control candidate: %w", inspectErr)
	}
	if inspection.Count != 1 {
		return true, aetherWSRouteControlDecision{}, errors.New("aether route control type field is duplicated")
	}
	if err := rejectDuplicateAetherWSControlFields(payload); err != nil {
		return true, aetherWSRouteControlDecision{}, err
	}

	var frame aetherWSRouteControlFrame
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&frame); err != nil {
		return true, aetherWSRouteControlDecision{}, fmt.Errorf("decode aether route control: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return true, aetherWSRouteControlDecision{}, errors.New("aether route control has trailing JSON")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	decision, identity, err := c.validateControlLocked(frame)
	if err != nil {
		return true, aetherWSRouteControlDecision{}, err
	}
	if previous, exists := c.seen[frame.ControlID]; exists {
		if previous.identity != identity {
			return true, aetherWSRouteControlDecision{}, errors.New("aether route control id was reused")
		}
		return true, previous.decision, nil
	}
	c.rememberControlLocked(frame.ControlID, identity, decision)
	return true, decision, nil
}

// aetherWSMayContainReservedType is the zero-allocation negative filter. The
// standard-library byte search uses its optimized substring implementation;
// avoid walking every ordinary 'a' or backslash in a potentially large delta.
// A JSON string that decodes to the reserved ASCII type contains either the
// complete literal marker or at least one Unicode escape.
func aetherWSMayContainReservedType(payload []byte) bool {
	return bytes.Contains(payload, aetherWSRouteControlMarker) ||
		bytes.Contains(payload, aetherWSRouteControlUnicodeEscape)
}

func rejectDuplicateAetherWSControlFields(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	opening, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode aether route control object: %w", err)
	}
	delim, ok := opening.(json.Delim)
	if !ok || delim != '{' {
		return errors.New("aether route control must be a JSON object")
	}
	seen := make(map[string]struct{}, 20)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode aether route control field: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("aether route control field name is invalid")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("aether route control field %q is duplicated", key)
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode aether route control field %q: %w", key, err)
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode aether route control object end: %w", err)
	}
	if delim, ok := closing.(json.Delim); !ok || delim != '}' {
		return errors.New("aether route control object is not closed")
	}
	return nil
}

func (c *aetherWSRouteControlConsumer) markProviderFrameWritten(terminal bool) {
	if c == nil || !c.enabled {
		return
	}
	c.providerOutputStarted.Store(true)
	if terminal {
		c.terminalWritten.Store(true)
	}
}

func (c *aetherWSRouteControlConsumer) validateControlLocked(frame aetherWSRouteControlFrame) (aetherWSRouteControlDecision, aetherWSRouteControlIdentity, error) {
	identity := identityForAetherWSRouteControl(frame)
	if frame.Type != aetherWSRouteControlEventType || frame.Version != aetherWSRouteControlVersion {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether route control type or version is invalid")
	}
	if frame.Action != aetherWSRouteActionCloseAfterTerminal && frame.Action != aetherWSRouteActionClientReconnect {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether route control action is unsupported")
	}
	for name, value := range map[string]string{
		"control_id":                  frame.ControlID,
		"sub2api_step_correlation_id": frame.Sub2APIStepCorrelationID,
		"sub2api_binding_epoch_id":    frame.Sub2APIBindingEpochID,
		"aether_step_id":              frame.AetherStepID,
		"aether_attempt_id":           frame.AetherAttemptID,
	} {
		if !validAetherWSOpaqueID(value) {
			return aetherWSRouteControlDecision{}, identity, fmt.Errorf("aether route control %s is invalid", name)
		}
	}
	if frame.Sub2APIStepCorrelationID != c.active.Sub2APIStepCorrelationID ||
		frame.Sub2APIBindingEpochID != c.active.Sub2APIBindingEpochID ||
		frame.Sub2APIBindingGeneration == 0 ||
		frame.Sub2APIBindingGeneration != c.active.Sub2APIBindingGeneration {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether route control step fence mismatch")
	}
	if !validAetherWSReason(frame.Reason) || frame.RetryAfterMS < 0 || frame.RetryAfterMS > aetherWSRouteControlMaxRetryMS {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether route control reason or retry delay is invalid")
	}
	if frame.RecommendedAction != frame.Action {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether route control recommendation mismatch")
	}
	if frame.ProviderFallbackUsed {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether provider fallback is unsupported on the Codex-only websocket route")
	}
	if frame.Action == aetherWSRouteActionCloseAfterTerminal {
		return c.validateCloseAfterTerminalLocked(frame, identity)
	}
	return c.validateClientReconnectLocked(frame, identity)
}

func (c *aetherWSRouteControlConsumer) validateCloseAfterTerminalLocked(frame aetherWSRouteControlFrame, identity aetherWSRouteControlIdentity) (aetherWSRouteControlDecision, aetherWSRouteControlIdentity, error) {
	if frame.Scope != "next_binding" || frame.EffectiveAfter != "current_terminal" ||
		frame.CurrentAttemptState != "terminal" || frame.ProviderWriteState != "confirmed" ||
		frame.ProviderExecutionDisposition != "terminal" || frame.AdapterProofClass != nil ||
		frame.AdapterProofVersion != nil || frame.MiddleRouteDisposition != nil {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether close-after-terminal proof is invalid")
	}
	return aetherWSRouteControlDecision{
		Action:             frame.Action,
		CloseAfterTerminal: true,
	}, identity, nil
}

func (c *aetherWSRouteControlConsumer) validateClientReconnectLocked(frame aetherWSRouteControlFrame, identity aetherWSRouteControlIdentity) (aetherWSRouteControlDecision, aetherWSRouteControlIdentity, error) {
	if frame.MiddleRouteDisposition == nil || !frame.MiddleRouteDisposition.Valid() {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether client reconnect middle route disposition is invalid")
	}
	if frame.Scope != "current_step" || frame.EffectiveAfter != "immediate" {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether client-reconnect scope is invalid")
	}
	if c.providerOutputStarted.Load() || c.terminalWritten.Load() {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether client reconnect arrived after provider output commit")
	}

	casePrepared := frame.CurrentAttemptState == "prepared" &&
		frame.ProviderWriteState == "not_started" &&
		frame.ProviderExecutionDisposition == "not_dispatched" &&
		frame.AdapterProofClass == nil && frame.AdapterProofVersion == nil
	caseProvenNotExecuted := frame.CurrentAttemptState == "rejected_before_execution" &&
		(frame.ProviderWriteState == "not_started" || frame.ProviderWriteState == "confirmed") &&
		frame.ProviderExecutionDisposition == "proven_not_executed" &&
		frame.AdapterProofClass != nil &&
		*frame.AdapterProofClass == AetherWSAdapterProofClassCodexOfficialNotExecuted &&
		frame.AdapterProofVersion != nil && *frame.AdapterProofVersion == AetherWSAdapterProofVersionV1
	if !casePrepared && !caseProvenNotExecuted {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether client-reconnect execution proof is invalid")
	}
	initialStepFailover := c.stepNumber == 1
	if !initialStepFailover && !c.reconnectEnabled {
		return aetherWSRouteControlDecision{}, identity, errors.New("aether reconnect migration is disabled")
	}
	return aetherWSRouteControlDecision{
		Action:                 frame.Action,
		ControlID:              frame.ControlID,
		BindingGeneration:      frame.Sub2APIBindingGeneration,
		RetryAfterMS:           frame.RetryAfterMS,
		MiddleRouteDisposition: *frame.MiddleRouteDisposition,
		SignalReconnect:        true,
		InitialStepFailover:    initialStepFailover,
	}, identity, nil
}

func (c *aetherWSRouteControlConsumer) rememberControlLocked(controlID string, identity aetherWSRouteControlIdentity, decision aetherWSRouteControlDecision) {
	if len(c.seenOrder) >= aetherWSRouteControlSeenLimit {
		oldest := c.seenOrder[0]
		delete(c.seen, oldest)
		copy(c.seenOrder, c.seenOrder[1:])
		c.seenOrder = c.seenOrder[:len(c.seenOrder)-1]
	}
	c.seen[controlID] = aetherWSSeenRouteControl{identity: identity, decision: decision}
	c.seenOrder = append(c.seenOrder, controlID)
}

func identityForAetherWSRouteControl(frame aetherWSRouteControlFrame) aetherWSRouteControlIdentity {
	identity := aetherWSRouteControlIdentity{
		Action:                       frame.Action,
		Scope:                        frame.Scope,
		EffectiveAfter:               frame.EffectiveAfter,
		Reason:                       frame.Reason,
		CorrelationID:                frame.Sub2APIStepCorrelationID,
		BindingEpochID:               frame.Sub2APIBindingEpochID,
		BindingGeneration:            frame.Sub2APIBindingGeneration,
		AetherStepID:                 frame.AetherStepID,
		AetherAttemptID:              frame.AetherAttemptID,
		CurrentAttemptState:          frame.CurrentAttemptState,
		ProviderWriteState:           frame.ProviderWriteState,
		ProviderExecutionDisposition: frame.ProviderExecutionDisposition,
		RetryAfterMS:                 frame.RetryAfterMS,
		RecommendedAction:            frame.RecommendedAction,
		ProviderFallbackUsed:         frame.ProviderFallbackUsed,
	}
	if frame.MiddleRouteDisposition != nil {
		identity.MiddleRouteDisposition = *frame.MiddleRouteDisposition
	}
	if frame.AdapterProofClass != nil {
		identity.AdapterProofClass = *frame.AdapterProofClass
	}
	if frame.AdapterProofVersion != nil {
		identity.AdapterProofVersion = *frame.AdapterProofVersion
	}
	return identity
}

func validAetherWSOpaqueID(value string) bool {
	if value == "" || len(value) > aetherWSRouteControlMaxIDBytes {
		return false
	}
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' || ch == ':' {
			continue
		}
		return false
	}
	return true
}

func validAetherWSReason(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > aetherWSRouteControlMaxReasonBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func aetherWSReconnectErrorPayload() []byte {
	return []byte(aetherWSReconnectErrorEvent)
}
