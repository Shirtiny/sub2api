package openai_ws_v2

import (
	"bytes"
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	ClientEnvelopeMaxTopLevelFields  = 128
	ClientEnvelopeMaxKeyBytes        = 256
	ClientEnvelopeMaxDepth           = 64
	ClientEnvelopeMaxEventTypeBytes  = 128
	ClientEnvelopeMaxIdentifierBytes = 256
	ClientEnvelopeMaxRouteIDBytes    = 512
	ClientEnvelopeMaxOptionBytes     = 64
	ClientEnvelopeMaxCacheKeyBytes   = 512

	clientEnvelopeMaxNestedFields    = 64
	clientEnvelopeMaxEncodedKeyBytes = ClientEnvelopeMaxKeyBytes * 6
)

var (
	errClientEnvelopeInvalidJSON       = errors.New("client websocket envelope is invalid JSON")
	errClientEnvelopeDuplicateField    = errors.New("client websocket envelope contains a duplicate field")
	errClientEnvelopeTooManyFields     = errors.New("client websocket envelope contains too many fields")
	errClientEnvelopeKeyTooLong        = errors.New("client websocket envelope field name is too long")
	errClientEnvelopeTooDeep           = errors.New("client websocket envelope exceeds the nesting limit")
	errClientEnvelopeTypeRequired      = errors.New("client websocket envelope type is required")
	errClientEnvelopeFieldTypeInvalid  = errors.New("client websocket envelope field type is invalid")
	errClientEnvelopeIdentifierInvalid = errors.New("client websocket envelope identifier is invalid")
)

// ClientEnvelope contains only bounded routing metadata. Request content stays
// in the caller-owned payload and is never copied by ParseClientEnvelope.
type ClientEnvelope struct {
	Type                           string
	Model                          string
	PreviousResponseID             string
	SessionModel                   string
	ServiceTier                    string
	ReasoningEffort                string
	PromptCacheKey                 string
	ClientMetadataSessionID        string
	ClientMetadataThreadID         string
	HasModel                       bool
	HasPreviousResponseID          bool
	HasSessionModel                bool
	HasServiceTier                 bool
	HasReasoningEffort             bool
	HasPromptCacheKey              bool
	HasClientMetadata              bool
	ClientMetadataIsObject         bool
	HasAetherStepControl           bool
	HasClientMetadataRouteIdentity bool
	modelValueStart                int
	modelValueEnd                  int
	serviceTierValueStart          int
	serviceTierValueEnd            int
	serviceTierFieldStart          int
	serviceTierFieldEnd            int
	clientMetadataValueStart       int
	clientMetadataValueEnd         int
}

// ModelValueRange returns the byte range of the JSON string value, including
// quotes, inside the validated caller-owned payload.
func (e ClientEnvelope) ModelValueRange() (start int, end int, ok bool) {
	if !e.HasModel || e.modelValueStart < 0 || e.modelValueEnd <= e.modelValueStart {
		return 0, 0, false
	}
	return e.modelValueStart, e.modelValueEnd, true
}

// ServiceTierValueRange returns the quoted service_tier value range.
func (e ClientEnvelope) ServiceTierValueRange() (start int, end int, ok bool) {
	if !e.HasServiceTier || e.serviceTierValueStart < 0 || e.serviceTierValueEnd <= e.serviceTierValueStart {
		return 0, 0, false
	}
	return e.serviceTierValueStart, e.serviceTierValueEnd, true
}

// ServiceTierFieldRange returns a deletion-safe range for the complete
// top-level service_tier member, including one adjacent comma.
func (e ClientEnvelope) ServiceTierFieldRange() (start int, end int, ok bool) {
	if !e.HasServiceTier || e.serviceTierFieldStart < 0 || e.serviceTierFieldEnd <= e.serviceTierFieldStart {
		return 0, 0, false
	}
	return e.serviceTierFieldStart, e.serviceTierFieldEnd, true
}

// ClientMetadataValueRange returns the validated top-level client_metadata
// JSON value range. Check ClientMetadataIsObject before treating the final
// byte as an object closing brace.
func (e ClientEnvelope) ClientMetadataValueRange() (start int, end int, ok bool) {
	if !e.HasClientMetadata || e.clientMetadataValueStart < 0 || e.clientMetadataValueEnd <= e.clientMetadataValueStart {
		return 0, 0, false
	}
	return e.clientMetadataValueStart, e.clientMetadataValueEnd, true
}

// TopLevelStringFieldInspection reports every occurrence of one decoded
// top-level key and whether any of its string values equals the decoded target.
// Partial results remain valid when inspection later returns a JSON error.
type TopLevelStringFieldInspection struct {
	Count   int
	Matched bool
}

// InspectTopLevelStringField scans one JSON object without materializing
// unrelated values. Field names and the expected value are bounded so matching
// uses fixed-size stack buffers even when unknown string values are very large.
func InspectTopLevelStringField(payload []byte, fieldName, expectedValue string) (TopLevelStringFieldInspection, error) {
	return inspectTopLevelStringField(payload, fieldName, expectedValue, false)
}

// InspectFirstTopLevelStringField stops after the first matching top-level key.
// It is a zero-allocation classification hint and intentionally does not
// validate bytes after that value; use InspectTopLevelStringField for the full
// count and strict parse result.
func InspectFirstTopLevelStringField(payload []byte, fieldName, expectedValue string) (TopLevelStringFieldInspection, error) {
	return inspectTopLevelStringField(payload, fieldName, expectedValue, true)
}

func inspectTopLevelStringField(payload []byte, fieldName, expectedValue string, stopAfterFirst bool) (TopLevelStringFieldInspection, error) {
	inspection := TopLevelStringFieldInspection{}
	if fieldName == "" || len(fieldName) > ClientEnvelopeMaxKeyBytes || len(expectedValue) > ClientEnvelopeMaxIdentifierBytes {
		return inspection, errClientEnvelopeIdentifierInvalid
	}

	parser := clientJSONParser{data: payload}
	parser.skipSpace()
	if !parser.consumeByte('{') {
		return inspection, errClientEnvelopeInvalidJSON
	}
	parser.skipSpace()
	if parser.consumeByte('}') {
		parser.skipSpace()
		if parser.pos != len(parser.data) {
			return inspection, errClientEnvelopeInvalidJSON
		}
		return inspection, nil
	}

	fieldCount := 0
	for {
		if fieldCount >= ClientEnvelopeMaxTopLevelFields {
			return inspection, errClientEnvelopeTooManyFields
		}
		fieldCount++
		parser.skipSpace()
		key, err := parser.scanString(ClientEnvelopeMaxKeyBytes, clientEnvelopeMaxEncodedKeyBytes)
		if err != nil {
			return inspection, normalizeClientKeyError(err)
		}
		var keyBytes [ClientEnvelopeMaxKeyBytes]byte
		keyLength := decodeClientJSONStringInto(parser.data, key, keyBytes[:])
		matchedKey := clientKeyEqual(keyBytes[:keyLength], fieldName)

		parser.skipSpace()
		if !parser.consumeByte(':') {
			return inspection, errClientEnvelopeInvalidJSON
		}
		parser.skipSpace()
		if matchedKey {
			inspection.Count++
			if parser.pos < len(parser.data) && parser.data[parser.pos] == '"' {
				value, valueErr := parser.scanString(0, 0)
				if valueErr != nil {
					return inspection, valueErr
				}
				if value.decodedLen == len(expectedValue) {
					var valueBytes [ClientEnvelopeMaxIdentifierBytes]byte
					valueLength := decodeClientJSONStringInto(parser.data, value, valueBytes[:])
					if clientKeyEqual(valueBytes[:valueLength], expectedValue) {
						inspection.Matched = true
					}
				}
			} else if err := parser.scanValue(1); err != nil {
				return inspection, err
			}
		} else if err := parser.scanValue(1); err != nil {
			return inspection, err
		}
		if matchedKey && stopAfterFirst {
			return inspection, nil
		}

		parser.skipSpace()
		if parser.consumeByte('}') {
			break
		}
		if !parser.consumeByte(',') {
			return inspection, errClientEnvelopeInvalidJSON
		}
	}
	parser.skipSpace()
	if parser.pos != len(parser.data) {
		return inspection, errClientEnvelopeInvalidJSON
	}
	return inspection, nil
}

// ParseClientEnvelope strictly validates one client text frame. It rejects
// duplicate top-level keys (including escaped-key aliases), bounds structural
// work, and extracts only the small fields needed by the relay.
func ParseClientEnvelope(payload []byte) (ClientEnvelope, error) {
	parser := clientJSONParser{data: payload}
	return parser.parseTopLevelEnvelope()
}

type clientJSONStringMeta struct {
	contentStart int
	contentEnd   int
	decodedLen   int
	hash         uint64
}

type clientJSONParser struct {
	data []byte
	pos  int
}

func (p *clientJSONParser) parseTopLevelEnvelope() (ClientEnvelope, error) {
	envelope := ClientEnvelope{}
	p.skipSpace()
	if !p.consumeByte('{') {
		return envelope, errClientEnvelopeInvalidJSON
	}
	p.skipSpace()
	if p.consumeByte('}') {
		return envelope, errClientEnvelopeTypeRequired
	}

	var seen [ClientEnvelopeMaxTopLevelFields]clientJSONStringMeta
	seenCount := 0
	fieldPrefixStart := -1
	for {
		if seenCount >= ClientEnvelopeMaxTopLevelFields {
			return envelope, errClientEnvelopeTooManyFields
		}
		p.skipSpace()
		fieldIndex := seenCount
		fieldStart := p.pos
		if fieldPrefixStart >= 0 {
			fieldStart = fieldPrefixStart
		}
		key, err := p.scanString(ClientEnvelopeMaxKeyBytes, clientEnvelopeMaxEncodedKeyBytes)
		if err != nil {
			return envelope, normalizeClientKeyError(err)
		}
		var keyBytes [ClientEnvelopeMaxKeyBytes]byte
		keyLen := decodeClientJSONStringInto(p.data, key, keyBytes[:])
		for i := 0; i < seenCount; i++ {
			if seen[i].hash != key.hash || seen[i].decodedLen != key.decodedLen {
				continue
			}
			var previous [ClientEnvelopeMaxKeyBytes]byte
			previousLen := decodeClientJSONStringInto(p.data, seen[i], previous[:])
			if previousLen == keyLen && bytes.Equal(previous[:previousLen], keyBytes[:keyLen]) {
				return envelope, errClientEnvelopeDuplicateField
			}
		}
		seen[seenCount] = key
		seenCount++

		p.skipSpace()
		if !p.consumeByte(':') {
			return envelope, errClientEnvelopeInvalidJSON
		}
		p.skipSpace()
		switch {
		case clientKeyEqual(keyBytes[:keyLen], "type"):
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxEventTypeBytes, false)
			if valueErr != nil {
				return envelope, valueErr
			}
			envelope.Type = strings.TrimSpace(value)
		case clientKeyEqual(keyBytes[:keyLen], "model"):
			valueStart := p.pos
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxIdentifierBytes, true)
			if valueErr != nil {
				return envelope, valueErr
			}
			envelope.Model = strings.TrimSpace(value)
			envelope.HasModel = true
			envelope.modelValueStart = valueStart
			envelope.modelValueEnd = p.pos
		case clientKeyEqual(keyBytes[:keyLen], "previous_response_id"):
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxIdentifierBytes, true)
			if valueErr != nil {
				return envelope, valueErr
			}
			envelope.PreviousResponseID = strings.TrimSpace(value)
			envelope.HasPreviousResponseID = true
		case clientKeyEqual(keyBytes[:keyLen], "session"):
			value, present, valueErr := p.scanSessionModel(1)
			if valueErr != nil {
				return envelope, valueErr
			}
			envelope.SessionModel = value
			envelope.HasSessionModel = present
		case clientKeyEqual(keyBytes[:keyLen], "service_tier"):
			valueStart := p.pos
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxOptionBytes, true)
			if valueErr != nil {
				return envelope, valueErr
			}
			envelope.ServiceTier = strings.TrimSpace(value)
			envelope.HasServiceTier = true
			envelope.serviceTierValueStart = valueStart
			envelope.serviceTierValueEnd = p.pos
			envelope.serviceTierFieldStart = fieldStart
			envelope.serviceTierFieldEnd = p.pos
		case clientKeyEqual(keyBytes[:keyLen], "reasoning_effort"):
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxOptionBytes, true)
			if valueErr != nil {
				return envelope, valueErr
			}
			envelope.ReasoningEffort = strings.TrimSpace(value)
			envelope.HasReasoningEffort = true
		case clientKeyEqual(keyBytes[:keyLen], "reasoning"):
			value, present, valueErr := p.scanReasoningEffort(1)
			if valueErr != nil {
				return envelope, valueErr
			}
			if present {
				envelope.ReasoningEffort = value
				envelope.HasReasoningEffort = true
			}
		case clientKeyEqual(keyBytes[:keyLen], "prompt_cache_key"):
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxCacheKeyBytes, true)
			if valueErr != nil {
				return envelope, valueErr
			}
			envelope.PromptCacheKey = strings.TrimSpace(value)
			envelope.HasPromptCacheKey = true
		case clientKeyEqual(keyBytes[:keyLen], "client_metadata"):
			envelope.HasClientMetadata = true
			envelope.ClientMetadataIsObject = p.pos < len(p.data) && p.data[p.pos] == '{'
			envelope.clientMetadataValueStart = p.pos
			sessionID, threadID, present, hasStepControl, valueErr := p.scanClientMetadataRouteIdentity(1)
			if valueErr != nil {
				return envelope, valueErr
			}
			envelope.clientMetadataValueEnd = p.pos
			envelope.ClientMetadataSessionID = sessionID
			envelope.ClientMetadataThreadID = threadID
			envelope.HasAetherStepControl = hasStepControl
			envelope.HasClientMetadataRouteIdentity = present
		default:
			if err := p.scanValue(1); err != nil {
				return envelope, err
			}
		}

		p.skipSpace()
		if p.consumeByte('}') {
			break
		}
		commaStart := p.pos
		if !p.consumeByte(',') {
			return envelope, errClientEnvelopeInvalidJSON
		}
		if envelope.HasServiceTier && fieldIndex == 0 && envelope.serviceTierFieldStart == fieldStart {
			envelope.serviceTierFieldEnd = p.pos
		}
		fieldPrefixStart = commaStart
	}
	p.skipSpace()
	if p.pos != len(p.data) {
		return envelope, errClientEnvelopeInvalidJSON
	}
	if envelope.Type == "" {
		return envelope, errClientEnvelopeTypeRequired
	}
	return envelope, nil
}

func (p *clientJSONParser) scanSessionModel(depth int) (string, bool, error) {
	if depth > ClientEnvelopeMaxDepth {
		return "", false, errClientEnvelopeTooDeep
	}
	p.skipSpace()
	if p.pos >= len(p.data) || p.data[p.pos] != '{' {
		if err := p.scanValue(depth); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	p.pos++
	p.skipSpace()
	if p.consumeByte('}') {
		return "", false, nil
	}

	var seen [clientEnvelopeMaxNestedFields]clientJSONStringMeta
	seenCount := 0
	model := ""
	hasModel := false
	for {
		if seenCount >= clientEnvelopeMaxNestedFields {
			return "", false, errClientEnvelopeTooManyFields
		}
		p.skipSpace()
		key, err := p.scanString(ClientEnvelopeMaxKeyBytes, clientEnvelopeMaxEncodedKeyBytes)
		if err != nil {
			return "", false, normalizeClientKeyError(err)
		}
		var keyBytes [ClientEnvelopeMaxKeyBytes]byte
		keyLen := decodeClientJSONStringInto(p.data, key, keyBytes[:])
		for i := 0; i < seenCount; i++ {
			if seen[i].hash != key.hash || seen[i].decodedLen != key.decodedLen {
				continue
			}
			var previous [ClientEnvelopeMaxKeyBytes]byte
			previousLen := decodeClientJSONStringInto(p.data, seen[i], previous[:])
			if previousLen == keyLen && bytes.Equal(previous[:previousLen], keyBytes[:keyLen]) {
				return "", false, errClientEnvelopeDuplicateField
			}
		}
		seen[seenCount] = key
		seenCount++
		p.skipSpace()
		if !p.consumeByte(':') {
			return "", false, errClientEnvelopeInvalidJSON
		}
		p.skipSpace()
		if clientKeyEqual(keyBytes[:keyLen], "model") {
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxIdentifierBytes, true)
			if valueErr != nil {
				return "", false, valueErr
			}
			model = strings.TrimSpace(value)
			hasModel = true
		} else if err := p.scanValue(depth + 1); err != nil {
			return "", false, err
		}
		p.skipSpace()
		if p.consumeByte('}') {
			break
		}
		if !p.consumeByte(',') {
			return "", false, errClientEnvelopeInvalidJSON
		}
	}
	return model, hasModel, nil
}

func (p *clientJSONParser) scanReasoningEffort(depth int) (string, bool, error) {
	if depth > ClientEnvelopeMaxDepth {
		return "", false, errClientEnvelopeTooDeep
	}
	p.skipSpace()
	if p.pos >= len(p.data) || p.data[p.pos] != '{' {
		if err := p.scanValue(depth); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	p.pos++
	p.skipSpace()
	if p.consumeByte('}') {
		return "", false, nil
	}

	var seen [clientEnvelopeMaxNestedFields]clientJSONStringMeta
	seenCount := 0
	effort := ""
	hasEffort := false
	for {
		if seenCount >= clientEnvelopeMaxNestedFields {
			return "", false, errClientEnvelopeTooManyFields
		}
		p.skipSpace()
		key, err := p.scanString(ClientEnvelopeMaxKeyBytes, clientEnvelopeMaxEncodedKeyBytes)
		if err != nil {
			return "", false, normalizeClientKeyError(err)
		}
		var keyBytes [ClientEnvelopeMaxKeyBytes]byte
		keyLen := decodeClientJSONStringInto(p.data, key, keyBytes[:])
		for i := 0; i < seenCount; i++ {
			if seen[i].hash != key.hash || seen[i].decodedLen != key.decodedLen {
				continue
			}
			var previous [ClientEnvelopeMaxKeyBytes]byte
			previousLen := decodeClientJSONStringInto(p.data, seen[i], previous[:])
			if previousLen == keyLen && bytes.Equal(previous[:previousLen], keyBytes[:keyLen]) {
				return "", false, errClientEnvelopeDuplicateField
			}
		}
		seen[seenCount] = key
		seenCount++
		p.skipSpace()
		if !p.consumeByte(':') {
			return "", false, errClientEnvelopeInvalidJSON
		}
		p.skipSpace()
		if clientKeyEqual(keyBytes[:keyLen], "effort") {
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxOptionBytes, true)
			if valueErr != nil {
				return "", false, valueErr
			}
			effort = strings.TrimSpace(value)
			hasEffort = true
		} else if err := p.scanValue(depth + 1); err != nil {
			return "", false, err
		}
		p.skipSpace()
		if p.consumeByte('}') {
			break
		}
		if !p.consumeByte(',') {
			return "", false, errClientEnvelopeInvalidJSON
		}
	}
	return effort, hasEffort, nil
}

func (p *clientJSONParser) scanClientMetadataRouteIdentity(depth int) (string, string, bool, bool, error) {
	if depth > ClientEnvelopeMaxDepth {
		return "", "", false, false, errClientEnvelopeTooDeep
	}
	p.skipSpace()
	if p.pos >= len(p.data) {
		return "", "", false, false, errClientEnvelopeInvalidJSON
	}
	if p.data[p.pos] != '{' {
		valueStart := p.pos
		if err := p.scanValue(depth); err != nil {
			return "", "", false, false, err
		}
		if bytes.Equal(p.data[valueStart:p.pos], []byte("null")) {
			return "", "", false, false, nil
		}
		return "", "", false, false, errClientEnvelopeFieldTypeInvalid
	}
	p.pos++
	p.skipSpace()
	if p.consumeByte('}') {
		return "", "", false, false, nil
	}

	var seen [clientEnvelopeMaxNestedFields]clientJSONStringMeta
	seenCount := 0
	sessionID := ""
	threadID := ""
	hasSessionID := false
	hasThreadID := false
	hasStepControl := false
	for {
		if seenCount >= clientEnvelopeMaxNestedFields {
			return "", "", false, false, errClientEnvelopeTooManyFields
		}
		p.skipSpace()
		key, err := p.scanString(ClientEnvelopeMaxKeyBytes, clientEnvelopeMaxEncodedKeyBytes)
		if err != nil {
			return "", "", false, false, normalizeClientKeyError(err)
		}
		var keyBytes [ClientEnvelopeMaxKeyBytes]byte
		keyLen := decodeClientJSONStringInto(p.data, key, keyBytes[:])
		for index := 0; index < seenCount; index++ {
			if seen[index].hash != key.hash || seen[index].decodedLen != key.decodedLen {
				continue
			}
			var previous [ClientEnvelopeMaxKeyBytes]byte
			previousLen := decodeClientJSONStringInto(p.data, seen[index], previous[:])
			if previousLen == keyLen && bytes.Equal(previous[:previousLen], keyBytes[:keyLen]) {
				return "", "", false, false, errClientEnvelopeDuplicateField
			}
		}
		seen[seenCount] = key
		seenCount++

		p.skipSpace()
		if !p.consumeByte(':') {
			return "", "", false, false, errClientEnvelopeInvalidJSON
		}
		p.skipSpace()
		switch {
		case clientKeyEqual(keyBytes[:keyLen], "session_id"):
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxRouteIDBytes, false)
			if valueErr != nil {
				return "", "", false, false, valueErr
			}
			sessionID = strings.TrimSpace(value)
			hasSessionID = true
		case clientKeyEqual(keyBytes[:keyLen], "thread_id"):
			value, valueErr := p.scanBoundedStringValue(ClientEnvelopeMaxRouteIDBytes, false)
			if valueErr != nil {
				return "", "", false, false, valueErr
			}
			threadID = strings.TrimSpace(value)
			hasThreadID = true
		case clientKeyEqual(keyBytes[:keyLen], "aether.sub2api_step_control"):
			hasStepControl = true
			if err := p.scanValue(depth + 1); err != nil {
				return "", "", false, false, err
			}
		default:
			if err := p.scanValue(depth + 1); err != nil {
				return "", "", false, false, err
			}
		}

		p.skipSpace()
		if p.consumeByte('}') {
			break
		}
		if !p.consumeByte(',') {
			return "", "", false, false, errClientEnvelopeInvalidJSON
		}
	}
	if !hasSessionID && !hasThreadID {
		return "", "", false, hasStepControl, nil
	}
	if !hasSessionID || !hasThreadID || sessionID == "" || threadID == "" {
		return "", "", false, false, errClientEnvelopeIdentifierInvalid
	}
	return sessionID, threadID, true, hasStepControl, nil
}

func (p *clientJSONParser) scanBoundedStringValue(maxBytes int, asciiOnly bool) (string, error) {
	if maxBytes <= 0 || maxBytes > ClientEnvelopeMaxRouteIDBytes || p.pos >= len(p.data) || p.data[p.pos] != '"' {
		return "", errClientEnvelopeFieldTypeInvalid
	}
	meta, err := p.scanString(maxBytes, maxBytes*6)
	if err != nil {
		return "", errClientEnvelopeIdentifierInvalid
	}
	var decoded [ClientEnvelopeMaxRouteIDBytes]byte
	length := decodeClientJSONStringInto(p.data, meta, decoded[:])
	if asciiOnly {
		for _, value := range decoded[:length] {
			if value > 0x7f {
				return "", errClientEnvelopeIdentifierInvalid
			}
		}
	}
	return string(decoded[:length]), nil
}

func (p *clientJSONParser) scanValue(depth int) error {
	if depth > ClientEnvelopeMaxDepth {
		return errClientEnvelopeTooDeep
	}
	p.skipSpace()
	if p.pos >= len(p.data) {
		return errClientEnvelopeInvalidJSON
	}
	switch p.data[p.pos] {
	case '"':
		_, err := p.scanString(0, 0)
		return err
	case '{':
		p.pos++
		p.skipSpace()
		if p.consumeByte('}') {
			return nil
		}
		for {
			p.skipSpace()
			if _, err := p.scanString(0, 0); err != nil {
				return err
			}
			p.skipSpace()
			if !p.consumeByte(':') {
				return errClientEnvelopeInvalidJSON
			}
			if err := p.scanValue(depth + 1); err != nil {
				return err
			}
			p.skipSpace()
			if p.consumeByte('}') {
				return nil
			}
			if !p.consumeByte(',') {
				return errClientEnvelopeInvalidJSON
			}
		}
	case '[':
		p.pos++
		p.skipSpace()
		if p.consumeByte(']') {
			return nil
		}
		for {
			if err := p.scanValue(depth + 1); err != nil {
				return err
			}
			p.skipSpace()
			if p.consumeByte(']') {
				return nil
			}
			if !p.consumeByte(',') {
				return errClientEnvelopeInvalidJSON
			}
		}
	case 't':
		return p.scanLiteral("true")
	case 'f':
		return p.scanLiteral("false")
	case 'n':
		return p.scanLiteral("null")
	default:
		return p.scanNumber()
	}
}

func (p *clientJSONParser) scanLiteral(literal string) error {
	if len(p.data)-p.pos < len(literal) || string(p.data[p.pos:p.pos+len(literal)]) != literal {
		return errClientEnvelopeInvalidJSON
	}
	p.pos += len(literal)
	return nil
}

func (p *clientJSONParser) scanNumber() error {
	start := p.pos
	if p.consumeByte('-') && p.pos >= len(p.data) {
		return errClientEnvelopeInvalidJSON
	}
	if p.consumeByte('0') {
		if p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			return errClientEnvelopeInvalidJSON
		}
	} else {
		if p.pos >= len(p.data) || p.data[p.pos] < '1' || p.data[p.pos] > '9' {
			return errClientEnvelopeInvalidJSON
		}
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
	}
	if p.consumeByte('.') {
		fractionStart := p.pos
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
		if p.pos == fractionStart {
			return errClientEnvelopeInvalidJSON
		}
	}
	if p.pos < len(p.data) && (p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.data) && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
			p.pos++
		}
		exponentStart := p.pos
		for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
			p.pos++
		}
		if p.pos == exponentStart {
			return errClientEnvelopeInvalidJSON
		}
	}
	if p.pos == start {
		return errClientEnvelopeInvalidJSON
	}
	return nil
}

func (p *clientJSONParser) scanString(maxDecodedBytes, maxEncodedBytes int) (clientJSONStringMeta, error) {
	meta := clientJSONStringMeta{hash: 1469598103934665603}
	if !p.consumeByte('"') {
		return meta, errClientEnvelopeInvalidJSON
	}
	meta.contentStart = p.pos
	emit := func(value byte) error {
		meta.decodedLen++
		if maxDecodedBytes > 0 && meta.decodedLen > maxDecodedBytes {
			return errClientEnvelopeKeyTooLong
		}
		meta.hash ^= uint64(value)
		meta.hash *= 1099511628211
		return nil
	}
	for p.pos < len(p.data) {
		value := p.data[p.pos]
		if value == '"' {
			meta.contentEnd = p.pos
			if maxEncodedBytes > 0 && meta.contentEnd-meta.contentStart > maxEncodedBytes {
				return meta, errClientEnvelopeKeyTooLong
			}
			p.pos++
			return meta, nil
		}
		if value < 0x20 {
			return meta, errClientEnvelopeInvalidJSON
		}
		if value == '\\' {
			p.pos++
			if p.pos >= len(p.data) {
				return meta, errClientEnvelopeInvalidJSON
			}
			escaped := p.data[p.pos]
			p.pos++
			switch escaped {
			case '"', '\\', '/':
				if err := emit(escaped); err != nil {
					return meta, err
				}
			case 'b':
				if err := emit('\b'); err != nil {
					return meta, err
				}
			case 'f':
				if err := emit('\f'); err != nil {
					return meta, err
				}
			case 'n':
				if err := emit('\n'); err != nil {
					return meta, err
				}
			case 'r':
				if err := emit('\r'); err != nil {
					return meta, err
				}
			case 't':
				if err := emit('\t'); err != nil {
					return meta, err
				}
			case 'u':
				r, runeErr := p.scanEscapedRune()
				if runeErr != nil {
					return meta, runeErr
				}
				var encoded [utf8.UTFMax]byte
				length := utf8.EncodeRune(encoded[:], r)
				for _, encodedByte := range encoded[:length] {
					if err := emit(encodedByte); err != nil {
						return meta, err
					}
				}
			default:
				return meta, errClientEnvelopeInvalidJSON
			}
			continue
		}
		if value < utf8.RuneSelf {
			p.pos++
			if err := emit(value); err != nil {
				return meta, err
			}
			continue
		}
		r, length := utf8.DecodeRune(p.data[p.pos:])
		if r == utf8.RuneError && length == 1 {
			return meta, errClientEnvelopeInvalidJSON
		}
		for _, rawByte := range p.data[p.pos : p.pos+length] {
			if err := emit(rawByte); err != nil {
				return meta, err
			}
		}
		p.pos += length
	}
	return meta, errClientEnvelopeInvalidJSON
}

func (p *clientJSONParser) scanEscapedRune() (rune, error) {
	first, ok := parseClientHex4(p.data, p.pos)
	if !ok {
		return 0, errClientEnvelopeInvalidJSON
	}
	p.pos += 4
	if first >= 0xd800 && first <= 0xdbff {
		if len(p.data)-p.pos < 6 || p.data[p.pos] != '\\' || p.data[p.pos+1] != 'u' {
			return 0, errClientEnvelopeInvalidJSON
		}
		second, secondOK := parseClientHex4(p.data, p.pos+2)
		if !secondOK || second < 0xdc00 || second > 0xdfff {
			return 0, errClientEnvelopeInvalidJSON
		}
		p.pos += 6
		return rune(0x10000 + (first-0xd800)*0x400 + (second - 0xdc00)), nil
	}
	if first >= 0xdc00 && first <= 0xdfff {
		return 0, errClientEnvelopeInvalidJSON
	}
	return rune(first), nil
}

func decodeClientJSONStringInto(data []byte, meta clientJSONStringMeta, output []byte) int {
	position := meta.contentStart
	written := 0
	for position < meta.contentEnd {
		value := data[position]
		if value != '\\' {
			if value < utf8.RuneSelf {
				output[written] = value
				written++
				position++
				continue
			}
			_, length := utf8.DecodeRune(data[position:meta.contentEnd])
			copy(output[written:], data[position:position+length])
			written += length
			position += length
			continue
		}
		position++
		escaped := data[position]
		position++
		switch escaped {
		case '"', '\\', '/':
			output[written] = escaped
			written++
		case 'b':
			output[written] = '\b'
			written++
		case 'f':
			output[written] = '\f'
			written++
		case 'n':
			output[written] = '\n'
			written++
		case 'r':
			output[written] = '\r'
			written++
		case 't':
			output[written] = '\t'
			written++
		case 'u':
			first, _ := parseClientHex4(data, position)
			position += 4
			r := rune(first)
			if first >= 0xd800 && first <= 0xdbff {
				second, _ := parseClientHex4(data, position+2)
				position += 6
				r = rune(0x10000 + (first-0xd800)*0x400 + (second - 0xdc00))
			}
			written += utf8.EncodeRune(output[written:], r)
		}
	}
	return written
}

func parseClientHex4(data []byte, start int) (int, bool) {
	if start < 0 || len(data)-start < 4 {
		return 0, false
	}
	value := 0
	for _, digit := range data[start : start+4] {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value += int(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value += int(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value += int(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func normalizeClientKeyError(err error) error {
	if errors.Is(err, errClientEnvelopeKeyTooLong) {
		return errClientEnvelopeKeyTooLong
	}
	return err
}

func clientKeyEqual(value []byte, expected string) bool {
	if len(value) != len(expected) {
		return false
	}
	for index := range value {
		if value[index] != expected[index] {
			return false
		}
	}
	return true
}

func (p *clientJSONParser) skipSpace() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return
		}
	}
}

func (p *clientJSONParser) consumeByte(value byte) bool {
	if p.pos >= len(p.data) || p.data[p.pos] != value {
		return false
	}
	p.pos++
	return true
}
