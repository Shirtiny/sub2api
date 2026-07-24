package openai_ws_v2

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/tidwall/gjson"
)

type FrameConn interface {
	ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error)
	WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error
	Close() error
}

type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	ImageOutputTokens        int
}

type RelayResult struct {
	RequestModel            string
	Usage                   Usage
	RequestID               string
	TerminalEventType       string
	FirstTokenMs            *int
	FirstByteMs             *int
	Duration                time.Duration
	ClientToUpstreamFrames  int64
	UpstreamToClientFrames  int64
	DroppedDownstreamFrames int64
}

type RelayTurnResult struct {
	RequestModel      string
	Usage             Usage
	RequestID         string
	TerminalEventType string
	Duration          time.Duration
	FirstTokenMs      *int
	FirstByteMs       *int
}

type RelayExit struct {
	Stage           string
	Err             error
	WroteDownstream bool
}

type RelayOptions struct {
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	UpstreamDrainTimeout  time.Duration
	FirstMessageType      coderws.MessageType
	FirstMessageSent      bool
	FirstMessageStartedAt time.Time
	InitialRequestModel   string
	// ValidatedFirstEnvelope avoids rescanning a manually dispatched first
	// frame. The caller must obtain it from ParseClientEnvelope before write.
	ValidatedFirstEnvelope          *ClientEnvelope
	StartClientAfterFirstDownstream bool
	RequireClientTextFrames         bool
	OnUsageParseFailure             func(eventType string, usageRaw string)
	OnTurnComplete                  func(turn RelayTurnResult)
	BeforeWriteClient               func(msgType coderws.MessageType, payload []byte, wroteDownstream bool) error
	BeforeInspectUpstreamFrame      func(msgType coderws.MessageType, payload []byte) error
	BeforeWriteUpstreamFrame        func(msgType coderws.MessageType, payload []byte, envelope ClientEnvelope) error
	TransformUpstreamFrame          func(msgType coderws.MessageType, payload []byte, envelope ClientEnvelope) ([]byte, error)
	BeforeWriteUpstream             func(msgType coderws.MessageType, payload []byte) error
	BeforeWriteResponseCreate       func(msgType coderws.MessageType, payload []byte, originalModel string) error
	TransformResponseCreate         func(payload []byte, envelope ClientEnvelope) ([]byte, error)
	BeforeDispatchResponseCreate    func(msgType coderws.MessageType, payload []byte, originalModel string) error
	InterceptUpstreamFrame          func(msgType coderws.MessageType, payload []byte) UpstreamFrameDirective
	OnProviderFrameWritten          func(terminal bool)
	ReadClientFrame                 func(ctx context.Context, clientConn FrameConn) (coderws.MessageType, []byte, error)
	OnTrace                         func(event RelayTraceEvent)
	Now                             func() time.Time
}

// UpstreamFrameDirective is an optional, connection-scoped interception result.
// It is used by trusted middle-hop protocols; ordinary provider frames leave
// Consume false and continue through the existing relay path unchanged.
type UpstreamFrameDirective struct {
	Consume                          bool
	CloseAfterTerminal               bool
	CloseAfterTerminalAlreadyWritten bool
	ClientMessageType                coderws.MessageType
	ClientPayload                    []byte
	Exit                             bool
	Graceful                         bool
	Err                              error
}

var errResponseCreateInFlight = errors.New("response.create received before previous response terminal")
var errResponseStepClosed = errors.New("response.create received after response step gate closed")
var errResponseCreatedMissingID = errors.New("response.created missing response id")
var errResponseProtocolIdentifierInvalid = errors.New("response event contains an invalid protocol identifier")
var errResponseCompletedWithoutCreated = errors.New("response.completed received before matching response.created")
var errResponseEventWithoutCreated = errors.New("response event received before matching response.created")
var errResponseStepIDMismatch = errors.New("response event id does not match active response")

type responseStepPhase uint8

const (
	responseStepIdle responseStepPhase = iota
	responseStepPreparing
	responseStepInFlight
	responseStepCommitting
	responseStepClosed
	responseStepSettledIDLimit = 16
	responseStepIDMaxBytes     = 256
	responseEventTypeMaxBytes  = 128
)

type responseStepGate struct {
	mu                 sync.Mutex
	phase              responseStepPhase
	generation         uint64
	sessionModel       string
	activeRequestModel string
	activeResponseID   string
	activeStartedAt    time.Time
	commitDone         chan struct{}
	settledIDs         map[string]struct{}
	settledOrder       []string
	phaseSnapshot      atomic.Uint32
	responseIDSnapshot atomic.Pointer[string]
}

type responseStepEventDecision struct {
	consume          bool
	terminalClaimed  bool
	closeGate        bool
	closeConnection  bool
	generation       uint64
	requestModel     string
	timingResponseID string
	turnStartedAt    time.Time
	err              error
}

type responseStepBegin struct {
	started       bool
	envelope      ClientEnvelope
	originalModel string
}

func newResponseStepGate(firstClientMessage ...[]byte) *responseStepGate {
	model := ""
	if len(firstClientMessage) > 0 {
		if envelope, err := ParseClientEnvelope(firstClientMessage[0]); err == nil && envelope.Type == "response.create" {
			model = envelope.Model
		}
	}
	return newResponseStepGateWithModel(model)
}

func newResponseStepGateWithModel(model string) *responseStepGate {
	gate := &responseStepGate{
		phase:              responseStepInFlight,
		generation:         1,
		sessionModel:       model,
		activeRequestModel: model,
		settledIDs:         make(map[string]struct{}, responseStepSettledIDLimit),
		settledOrder:       make([]string, 0, responseStepSettledIDLimit),
	}
	gate.phaseSnapshot.Store(uint32(responseStepInFlight))
	return gate
}

func (g *responseStepGate) begin(msgType coderws.MessageType, payload []byte) (bool, error) {
	begin, err := g.inspectAndBegin(msgType, payload)
	return begin.started, err
}

func (g *responseStepGate) inspectAndBegin(msgType coderws.MessageType, payload []byte) (responseStepBegin, error) {
	if g == nil || msgType != coderws.MessageText {
		return responseStepBegin{}, nil
	}
	envelope, err := ParseClientEnvelope(payload)
	if err != nil {
		return responseStepBegin{}, err
	}
	begin := responseStepBegin{envelope: envelope, originalModel: envelope.Model}
	if envelope.Type != "response.create" {
		return begin, nil
	}
	for {
		g.mu.Lock()
		switch g.phase {
		case responseStepCommitting:
			commitDone := g.commitDone
			g.mu.Unlock()
			if commitDone != nil {
				<-commitDone
			}
			continue
		case responseStepClosed:
			g.mu.Unlock()
			return responseStepBegin{}, errResponseStepClosed
		case responseStepPreparing, responseStepInFlight:
			g.mu.Unlock()
			return responseStepBegin{}, errResponseCreateInFlight
		case responseStepIdle:
			model := envelope.Model
			if model == "" {
				model = g.sessionModel
			} else {
				g.sessionModel = model
			}
			g.generation++
			g.phase = responseStepPreparing
			g.activeRequestModel = model
			g.activeResponseID = ""
			g.activeStartedAt = time.Time{}
			g.responseIDSnapshot.Store(nil)
			g.phaseSnapshot.Store(uint32(responseStepPreparing))
			g.mu.Unlock()
			begin.started = true
			begin.originalModel = model
			return begin, nil
		default:
			g.mu.Unlock()
			return responseStepBegin{}, errResponseStepClosed
		}
	}
}

func (g *responseStepGate) abortBegin() {
	if g == nil {
		return
	}
	g.mu.Lock()
	if g.phase == responseStepPreparing {
		g.phase = responseStepIdle
		g.phaseSnapshot.Store(uint32(responseStepIdle))
		g.activeRequestModel = ""
		g.activeResponseID = ""
		g.responseIDSnapshot.Store(nil)
	}
	g.mu.Unlock()
}

// dispatchPrepared publishes a later response.create as active only after all
// admission hooks and payload transforms have completed. Upstream frames that
// arrive while those hooks are running therefore cannot settle the new turn.
func (g *responseStepGate) dispatchPrepared(startedAt time.Time) error {
	if g == nil {
		return errResponseStepClosed
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != responseStepPreparing {
		return errResponseStepClosed
	}
	g.phase = responseStepInFlight
	g.activeStartedAt = startedAt
	g.phaseSnapshot.Store(uint32(responseStepInFlight))
	return nil
}

func (g *responseStepGate) observe(observed observedUpstreamEvent) responseStepEventDecision {
	if observed.protocolErr != nil {
		return responseStepEventDecision{err: observed.protocolErr}
	}
	if g == nil {
		return responseStepEventDecision{}
	}
	if observed.eventType == "" {
		return g.currentInFlightDecision()
	}
	responseID := strings.TrimSpace(observed.responseID)
	responseIDRaw := observed.responseIDRaw
	if responseID == "" && len(responseIDRaw) > 0 && !validResponseStepIdentifierBytes(responseIDRaw, responseStepIDMaxBytes) {
		return responseStepEventDecision{err: errResponseProtocolIdentifierInvalid}
	}
	if responseID != "" && !validResponseStepIdentifier(responseID, responseStepIDMaxBytes) {
		return responseStepEventDecision{err: errResponseProtocolIdentifierInvalid}
	}
	boundaryEvent := isResponseStepBoundaryEvent(observed.eventType)
	// A later turn is not attributable until its admission and transform hooks
	// finish. Delayed frames from the previous turn are consumed in this phase.
	if responseStepPhase(g.phaseSnapshot.Load()) == responseStepPreparing {
		return responseStepEventDecision{consume: true}
	}
	// Deltas stay entirely off the gate lock and use immutable atomic provenance.
	if observed.eventType != "response.created" && !observed.terminal && !boundaryEvent {
		if responseID == "" && len(responseIDRaw) > 0 {
			return g.observeIncrementalResponseIDBytes(responseIDRaw)
		}
		return g.observeIncrementalResponseID(responseID)
	}
	if responseID == "" && len(responseIDRaw) > 0 {
		responseID = string(responseIDRaw)
	}
	if boundaryEvent && responseID == "" {
		return responseStepEventDecision{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if observed.eventType == "response.created" {
		if responseID == "" {
			return responseStepEventDecision{err: errResponseCreatedMissingID}
		}
		if g.isSettledLocked(responseID) || g.phase == responseStepIdle || g.phase == responseStepClosed {
			return responseStepEventDecision{consume: true}
		}
		if g.phase != responseStepInFlight {
			return responseStepEventDecision{consume: true}
		}
		if g.activeResponseID == "" {
			g.activeResponseID = responseID
			responseIDCopy := responseID
			g.responseIDSnapshot.Store(&responseIDCopy)
			return responseStepEventDecision{generation: g.generation, requestModel: g.activeRequestModel, turnStartedAt: g.activeStartedAt}
		}
		if g.activeResponseID == responseID {
			return responseStepEventDecision{consume: true}
		}
		return responseStepEventDecision{err: errResponseStepIDMismatch}
	}

	if boundaryEvent {
		if g.isSettledLocked(responseID) || g.phase == responseStepIdle || g.phase == responseStepClosed {
			return responseStepEventDecision{consume: true}
		}
		if g.phase != responseStepInFlight {
			return responseStepEventDecision{consume: true}
		}
		if g.activeResponseID == "" {
			return responseStepEventDecision{err: errResponseEventWithoutCreated}
		}
		if responseID != g.activeResponseID {
			return responseStepEventDecision{err: errResponseStepIDMismatch}
		}
		return responseStepEventDecision{generation: g.generation, requestModel: g.activeRequestModel, turnStartedAt: g.activeStartedAt}
	}
	if responseID != "" && g.isSettledLocked(responseID) {
		return responseStepEventDecision{consume: true}
	}
	if observed.eventType == "error" && g.phase == responseStepIdle {
		g.phase = responseStepClosed
		g.phaseSnapshot.Store(uint32(responseStepClosed))
		g.responseIDSnapshot.Store(nil)
		return responseStepEventDecision{closeConnection: true}
	}
	if g.phase != responseStepInFlight {
		return responseStepEventDecision{consume: true}
	}

	closeGate := isFailureTerminalEvent(observed.eventType)
	if observed.eventType == "response.completed" {
		if responseID == "" || g.activeResponseID == "" {
			return responseStepEventDecision{err: errResponseCompletedWithoutCreated}
		}
		if responseID != g.activeResponseID {
			return responseStepEventDecision{err: errResponseStepIDMismatch}
		}
	} else if g.activeResponseID != "" && responseID != "" && responseID != g.activeResponseID {
		return responseStepEventDecision{err: errResponseStepIDMismatch}
	}

	g.phase = responseStepCommitting
	g.phaseSnapshot.Store(uint32(responseStepCommitting))
	g.commitDone = make(chan struct{})
	g.rememberSettledLocked(responseID)
	return responseStepEventDecision{
		terminalClaimed:  true,
		closeGate:        closeGate,
		closeConnection:  closeGate,
		generation:       g.generation,
		requestModel:     g.activeRequestModel,
		timingResponseID: g.activeResponseID,
		turnStartedAt:    g.activeStartedAt,
	}
}

func (g *responseStepGate) observeIncrementalResponseIDBytes(responseID []byte) responseStepEventDecision {
	if len(responseID) == 0 {
		return responseStepEventDecision{}
	}
	phase := responseStepPhase(g.phaseSnapshot.Load())
	if phase != responseStepInFlight {
		return responseStepEventDecision{consume: true}
	}
	activeResponseID := g.responseIDSnapshot.Load()
	if activeResponseID == nil {
		return responseStepEventDecision{err: errResponseEventWithoutCreated}
	}
	if len(responseID) != len(*activeResponseID) {
		return responseStepEventDecision{err: errResponseStepIDMismatch}
	}
	for index := range responseID {
		if responseID[index] != (*activeResponseID)[index] {
			return responseStepEventDecision{err: errResponseStepIDMismatch}
		}
	}
	return responseStepEventDecision{}
}

func (g *responseStepGate) observeIncrementalResponseID(responseID string) responseStepEventDecision {
	if responseID == "" {
		return g.currentInFlightDecision()
	}
	phase := responseStepPhase(g.phaseSnapshot.Load())
	if phase != responseStepInFlight {
		return responseStepEventDecision{consume: true}
	}
	activeResponseID := g.responseIDSnapshot.Load()
	if activeResponseID == nil {
		return responseStepEventDecision{err: errResponseEventWithoutCreated}
	}
	if responseID != *activeResponseID {
		return responseStepEventDecision{err: errResponseStepIDMismatch}
	}
	return responseStepEventDecision{}
}

func (g *responseStepGate) currentInFlightDecision() responseStepEventDecision {
	if g == nil {
		return responseStepEventDecision{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != responseStepInFlight {
		return responseStepEventDecision{}
	}
	return responseStepEventDecision{
		generation:    g.generation,
		requestModel:  g.activeRequestModel,
		turnStartedAt: g.activeStartedAt,
	}
}

func (g *responseStepGate) finishTerminal(closeGate bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	if g.phase == responseStepCommitting {
		if closeGate {
			g.phase = responseStepClosed
		} else {
			g.phase = responseStepIdle
		}
		g.phaseSnapshot.Store(uint32(g.phase))
		g.activeRequestModel = ""
		g.activeResponseID = ""
		g.activeStartedAt = time.Time{}
		g.responseIDSnapshot.Store(nil)
		if g.commitDone != nil {
			close(g.commitDone)
			g.commitDone = nil
		}
	}
	g.mu.Unlock()
}

func (g *responseStepGate) isSettledLocked(responseID string) bool {
	if responseID == "" || g.settledIDs == nil {
		return false
	}
	_, ok := g.settledIDs[responseID]
	return ok
}

func (g *responseStepGate) rememberSettledLocked(responseID string) {
	if responseID == "" || g.isSettledLocked(responseID) {
		return
	}
	if len(g.settledOrder) == responseStepSettledIDLimit {
		delete(g.settledIDs, g.settledOrder[0])
		copy(g.settledOrder, g.settledOrder[1:])
		g.settledOrder = g.settledOrder[:len(g.settledOrder)-1]
	}
	g.settledIDs[responseID] = struct{}{}
	g.settledOrder = append(g.settledOrder, responseID)
}

type RelayTraceEvent struct {
	Stage           string
	Direction       string
	MessageType     string
	PayloadBytes    int
	Graceful        bool
	WroteDownstream bool
	Error           string
}

type relayState struct {
	usage                   Usage
	requestModel            string
	lastResponseID          string
	terminalEventType       string
	firstTokenMs            *int
	firstByteMs             *int
	turnTimingByID          map[string]*relayTurnTiming
	activeTurn              *relayTurnTiming
	terminalWrittenToClient atomic.Bool
	interceptUpstream       func(msgType coderws.MessageType, payload []byte) UpstreamFrameDirective
	onProviderWritten       func(terminal bool)
}

type relayExitSignal struct {
	stage           string
	err             error
	graceful        bool
	wroteDownstream bool
}

type observedUpstreamEvent struct {
	terminal         bool
	eventType        string
	responseID       string
	responseIDRaw    []byte
	requestModel     string
	timingResponseID string
	turnStartedAt    time.Time
	generation       uint64
	message          []byte
	observedAt       time.Time
	tokenEvent       bool
	usage            Usage
	duration         time.Duration
	firstToken       *int
	firstByte        *int
	turnTiming       *relayTurnTiming
	protocolErr      error
}

type relayTurnTiming struct {
	startAt      time.Time
	firstTokenMs *int
	firstByteMs  *int
}

func Relay(
	ctx context.Context,
	clientConn FrameConn,
	upstreamConn FrameConn,
	firstClientMessage []byte,
	options RelayOptions,
) (RelayResult, *RelayExit) {
	result := RelayResult{}
	if clientConn == nil || upstreamConn == nil {
		return result, &RelayExit{Stage: "relay_init", Err: errors.New("relay connection is nil")}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	nowFn := options.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	writeTimeout := options.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = 2 * time.Minute
	}
	drainTimeout := options.UpstreamDrainTimeout
	if drainTimeout <= 0 {
		drainTimeout = 1200 * time.Millisecond
	}
	firstMessageType := options.FirstMessageType
	if firstMessageType != coderws.MessageBinary {
		firstMessageType = coderws.MessageText
	}
	firstEnvelope := ClientEnvelope{}
	var firstEnvelopeErr error
	if options.ValidatedFirstEnvelope != nil {
		firstEnvelope = *options.ValidatedFirstEnvelope
	} else {
		firstEnvelope, firstEnvelopeErr = ParseClientEnvelope(firstClientMessage)
	}
	if firstEnvelopeErr != nil && firstMessageType == coderws.MessageText {
		return result, &RelayExit{Stage: "relay_init", Err: firstEnvelopeErr}
	}
	if firstEnvelopeErr == nil {
		result.RequestModel = firstEnvelope.Model
	}
	if initialRequestModel := strings.TrimSpace(options.InitialRequestModel); initialRequestModel != "" {
		result.RequestModel = initialRequestModel
	}
	startAt := options.FirstMessageStartedAt
	if startAt.IsZero() {
		startAt = nowFn()
	}
	state := &relayState{
		requestModel:      result.RequestModel,
		interceptUpstream: options.InterceptUpstreamFrame,
		onProviderWritten: options.OnProviderFrameWritten,
	}
	onTrace := options.OnTrace

	relayCtx, relayCancel := context.WithCancel(ctx)
	defer relayCancel()

	lastActivity := atomic.Int64{}
	lastActivity.Store(nowFn().UnixNano())
	markActivity := func() {
		lastActivity.Store(nowFn().UnixNano())
	}

	upstreamWriteCtx := newWriteDeadlineContext(relayCtx)
	clientWriteCtx := newWriteDeadlineContext(relayCtx)
	defer upstreamWriteCtx.Stop()
	defer clientWriteCtx.Stop()
	writeUpstream := func(msgType coderws.MessageType, payload []byte) error {
		if err := upstreamWriteCtx.Reset(writeTimeout); err != nil {
			return err
		}
		err := upstreamConn.WriteFrame(upstreamWriteCtx, msgType, payload)
		upstreamWriteCtx.Disarm()
		return err
	}
	writeClient := func(msgType coderws.MessageType, payload []byte) error {
		if err := clientWriteCtx.Reset(writeTimeout); err != nil {
			return err
		}
		err := clientConn.WriteFrame(clientWriteCtx, msgType, payload)
		clientWriteCtx.Disarm()
		return err
	}
	stepGate := newResponseStepGateWithModel(result.RequestModel)
	stepGate.activeStartedAt = startAt
	writeNextUpstream := func(msgType coderws.MessageType, payload []byte) error {
		if options.BeforeInspectUpstreamFrame != nil {
			if err := options.BeforeInspectUpstreamFrame(msgType, payload); err != nil {
				return err
			}
		}
		if options.RequireClientTextFrames && msgType != coderws.MessageText {
			return errors.New("client websocket binary frames are unsupported")
		}
		begin, err := stepGate.inspectAndBegin(msgType, payload)
		if err != nil {
			return err
		}
		if options.BeforeWriteUpstreamFrame != nil {
			if err := options.BeforeWriteUpstreamFrame(msgType, payload, begin.envelope); err != nil {
				if begin.started {
					stepGate.abortBegin()
				}
				return err
			}
		}
		if options.TransformUpstreamFrame != nil {
			payload, err = options.TransformUpstreamFrame(msgType, payload, begin.envelope)
			if err != nil {
				if begin.started {
					stepGate.abortBegin()
				}
				return err
			}
		}
		if begin.started && options.BeforeWriteUpstream != nil {
			if err := options.BeforeWriteUpstream(msgType, payload); err != nil {
				stepGate.abortBegin()
				return err
			}
		}
		if begin.started && options.BeforeWriteResponseCreate != nil {
			if err := options.BeforeWriteResponseCreate(msgType, payload, begin.originalModel); err != nil {
				stepGate.abortBegin()
				return err
			}
		}
		if begin.started && options.TransformResponseCreate != nil {
			payload, err = options.TransformResponseCreate(payload, begin.envelope)
			if err != nil {
				stepGate.abortBegin()
				return err
			}
		}
		dispatchStartedAt := time.Time{}
		if begin.started {
			dispatchStartedAt = nowFn()
		}
		if begin.started && options.BeforeDispatchResponseCreate != nil {
			if err := options.BeforeDispatchResponseCreate(msgType, payload, begin.originalModel); err != nil {
				stepGate.abortBegin()
				return err
			}
		}
		if begin.started {
			if err := stepGate.dispatchPrepared(dispatchStartedAt); err != nil {
				stepGate.abortBegin()
				return err
			}
		}
		return writeUpstream(msgType, payload)
	}

	clientToUpstreamFrames := &atomic.Int64{}
	upstreamToClientFrames := &atomic.Int64{}
	droppedDownstreamFrames := &atomic.Int64{}
	emitRelayTrace(onTrace, RelayTraceEvent{
		Stage:        "relay_start",
		PayloadBytes: len(firstClientMessage),
		MessageType:  relayMessageTypeString(firstMessageType),
	})

	if options.FirstMessageSent {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:        "write_first_message_skipped",
			Direction:    "client_to_upstream",
			MessageType:  relayMessageTypeString(firstMessageType),
			PayloadBytes: len(firstClientMessage),
		})
	} else {
		if err := writeUpstream(firstMessageType, firstClientMessage); err != nil {
			result.Duration = nowFn().Sub(startAt)
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:        "write_first_message_failed",
				Direction:    "client_to_upstream",
				MessageType:  relayMessageTypeString(firstMessageType),
				PayloadBytes: len(firstClientMessage),
				Error:        err.Error(),
			})
			return result, &RelayExit{Stage: "write_upstream", Err: err}
		}
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:        "write_first_message_ok",
			Direction:    "client_to_upstream",
			MessageType:  relayMessageTypeString(firstMessageType),
			PayloadBytes: len(firstClientMessage),
		})
	}
	clientToUpstreamFrames.Add(1)
	markActivity()

	exitCh := make(chan relayExitSignal, 3)
	dropDownstreamWrites := atomic.Bool{}
	clientReaderStarted := atomic.Bool{}
	startClientReader := func() {
		if !clientReaderStarted.CompareAndSwap(false, true) {
			return
		}
		go runClientToUpstream(relayCtx, clientConn, options.ReadClientFrame, writeNextUpstream, markActivity, clientToUpstreamFrames, onTrace, exitCh)
	}
	if !options.StartClientAfterFirstDownstream {
		startClientReader()
	}
	upstreamReaderDone := make(chan struct{})
	go func() {
		defer close(upstreamReaderDone)
		runUpstreamToClient(
			relayCtx,
			upstreamConn,
			writeClient,
			startAt,
			nowFn,
			state,
			options.OnUsageParseFailure,
			options.OnTurnComplete,
			options.BeforeWriteClient,
			func() {
				if options.StartClientAfterFirstDownstream {
					startClientReader()
				}
			},
			&dropDownstreamWrites,
			upstreamToClientFrames,
			droppedDownstreamFrames,
			markActivity,
			onTrace,
			stepGate,
			exitCh,
		)
	}()
	go runIdleWatchdog(relayCtx, nowFn, options.IdleTimeout, &lastActivity, onTrace, exitCh)

	firstExit := <-exitCh
	emitRelayTrace(onTrace, RelayTraceEvent{
		Stage:           "first_exit",
		Direction:       relayDirectionFromStage(firstExit.stage),
		Graceful:        firstExit.graceful,
		WroteDownstream: firstExit.wroteDownstream,
		Error:           relayErrorString(firstExit.err),
	})
	combinedWroteDownstream := firstExit.wroteDownstream
	secondExit := relayExitSignal{graceful: true}
	hasSecondExit := false

	// 客户端断开后尽力继续读取上游短窗口，捕获延迟 usage/terminal 事件用于计费。
	if firstExit.stage == "read_client" && firstExit.graceful {
		dropDownstreamWrites.Store(true)
		secondExit, hasSecondExit = waitRelayExit(exitCh, drainTimeout)
	} else {
		relayCancel()
		_ = upstreamConn.Close()
		if clientReaderStarted.Load() {
			secondExit, hasSecondExit = waitRelayExit(exitCh, 200*time.Millisecond)
		}
	}
	if hasSecondExit {
		combinedWroteDownstream = combinedWroteDownstream || secondExit.wroteDownstream
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "second_exit",
			Direction:       relayDirectionFromStage(secondExit.stage),
			Graceful:        secondExit.graceful,
			WroteDownstream: secondExit.wroteDownstream,
			Error:           relayErrorString(secondExit.err),
		})
	}

	relayCancel()
	_ = upstreamConn.Close()
	<-upstreamReaderDone

	enrichResult(&result, state, nowFn().Sub(startAt))
	result.ClientToUpstreamFrames = clientToUpstreamFrames.Load()
	result.UpstreamToClientFrames = upstreamToClientFrames.Load()
	result.DroppedDownstreamFrames = droppedDownstreamFrames.Load()
	if firstExit.stage == "read_client" && firstExit.graceful {
		if state.terminalWrittenToClient.Load() && (!hasSecondExit || secondExit.graceful) {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "relay_complete",
				Graceful:        true,
				WroteDownstream: combinedWroteDownstream,
			})
			return result, nil
		}
		stage := "client_disconnected"
		exitErr := firstExit.err
		if hasSecondExit && !secondExit.graceful {
			stage = secondExit.stage
			exitErr = secondExit.err
		}
		if exitErr == nil {
			exitErr = io.EOF
		}
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_exit",
			Direction:       relayDirectionFromStage(stage),
			Graceful:        false,
			WroteDownstream: combinedWroteDownstream,
			Error:           relayErrorString(exitErr),
		})
		return result, &RelayExit{
			Stage:           stage,
			Err:             exitErr,
			WroteDownstream: combinedWroteDownstream,
		}
	}
	if firstExit.graceful && (!hasSecondExit || secondExit.graceful) {
		if result.TerminalEventType == "" {
			exitErr := firstExit.err
			if exitErr == nil {
				exitErr = io.EOF
			}
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "relay_exit",
				Direction:       relayDirectionFromStage(firstExit.stage),
				Graceful:        false,
				WroteDownstream: combinedWroteDownstream,
				Error:           relayErrorString(exitErr),
			})
			return result, &RelayExit{
				Stage:           firstExit.stage,
				Err:             exitErr,
				WroteDownstream: combinedWroteDownstream,
			}
		}
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_complete",
			Graceful:        true,
			WroteDownstream: combinedWroteDownstream,
		})
		_ = clientConn.Close()
		return result, nil
	}
	if !firstExit.graceful {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_exit",
			Direction:       relayDirectionFromStage(firstExit.stage),
			Graceful:        false,
			WroteDownstream: combinedWroteDownstream,
			Error:           relayErrorString(firstExit.err),
		})
		return result, &RelayExit{
			Stage:           firstExit.stage,
			Err:             firstExit.err,
			WroteDownstream: combinedWroteDownstream,
		}
	}
	if hasSecondExit && !secondExit.graceful {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_exit",
			Direction:       relayDirectionFromStage(secondExit.stage),
			Graceful:        false,
			WroteDownstream: combinedWroteDownstream,
			Error:           relayErrorString(secondExit.err),
		})
		return result, &RelayExit{
			Stage:           secondExit.stage,
			Err:             secondExit.err,
			WroteDownstream: combinedWroteDownstream,
		}
	}
	if options.FirstMessageSent {
		emitRelayTrace(onTrace, RelayTraceEvent{
			Stage:           "relay_client_closed",
			Graceful:        true,
			WroteDownstream: combinedWroteDownstream,
		})
		return result, nil
	}
	emitRelayTrace(onTrace, RelayTraceEvent{
		Stage:           "relay_complete",
		Graceful:        true,
		WroteDownstream: combinedWroteDownstream,
	})
	_ = clientConn.Close()
	return result, nil
}

func runClientToUpstream(
	ctx context.Context,
	clientConn FrameConn,
	readClientFrame func(context.Context, FrameConn) (coderws.MessageType, []byte, error),
	writeUpstream func(msgType coderws.MessageType, payload []byte) error,
	markActivity func(),
	forwardedFrames *atomic.Int64,
	onTrace func(event RelayTraceEvent),
	exitCh chan<- relayExitSignal,
) {
	if readClientFrame == nil {
		readClientFrame = func(ctx context.Context, conn FrameConn) (coderws.MessageType, []byte, error) {
			return conn.ReadFrame(ctx)
		}
	}
	for {
		msgType, payload, err := readClientFrame(ctx, clientConn)
		if err != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:     "read_client_failed",
				Direction: "client_to_upstream",
				Error:     err.Error(),
				Graceful:  isDisconnectError(err),
			})
			exitCh <- relayExitSignal{stage: "read_client", err: err, graceful: isDisconnectError(err)}
			return
		}
		markActivity()
		if err := writeUpstream(msgType, payload); err != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:        "write_upstream_failed",
				Direction:    "client_to_upstream",
				MessageType:  relayMessageTypeString(msgType),
				PayloadBytes: len(payload),
				Error:        err.Error(),
			})
			exitCh <- relayExitSignal{stage: "write_upstream", err: err}
			return
		}
		if forwardedFrames != nil {
			forwardedFrames.Add(1)
		}
		markActivity()
	}
}

func runUpstreamToClient(
	ctx context.Context,
	upstreamConn FrameConn,
	writeClient func(msgType coderws.MessageType, payload []byte) error,
	startAt time.Time,
	nowFn func() time.Time,
	state *relayState,
	onUsageParseFailure func(eventType string, usageRaw string),
	onTurnComplete func(turn RelayTurnResult),
	beforeWriteClient func(msgType coderws.MessageType, payload []byte, wroteDownstream bool) error,
	afterWriteClient func(),
	dropDownstreamWrites *atomic.Bool,
	forwardedFrames *atomic.Int64,
	droppedFrames *atomic.Int64,
	markActivity func(),
	onTrace func(event RelayTraceEvent),
	stepGate *responseStepGate,
	exitCh chan<- relayExitSignal,
) {
	wroteDownstream := false
	stepWroteDownstream := false
	closeAfterTerminal := false
	var interceptUpstreamFrame func(msgType coderws.MessageType, payload []byte) UpstreamFrameDirective
	var onProviderFrameWritten func(terminal bool)
	if state != nil {
		interceptUpstreamFrame = state.interceptUpstream
		onProviderFrameWritten = state.onProviderWritten
	}
	for {
		msgType, payload, err := upstreamConn.ReadFrame(ctx)
		if err != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "read_upstream_failed",
				Direction:       "upstream_to_client",
				Error:           err.Error(),
				Graceful:        isDisconnectError(err),
				WroteDownstream: wroteDownstream,
			})
			exitCh <- relayExitSignal{
				stage:           "read_upstream",
				err:             err,
				graceful:        isDisconnectError(err),
				wroteDownstream: wroteDownstream,
			}
			return
		}
		markActivity()
		if interceptUpstreamFrame != nil {
			directive := interceptUpstreamFrame(msgType, payload)
			if directive.Consume {
				closeAfterTerminal = closeAfterTerminal || directive.CloseAfterTerminal
				if len(directive.ClientPayload) > 0 {
					clientMessageType := directive.ClientMessageType
					if clientMessageType != coderws.MessageBinary {
						clientMessageType = coderws.MessageText
					}
					if err := writeClient(clientMessageType, directive.ClientPayload); err != nil {
						exitCh <- relayExitSignal{stage: "write_client", err: err, wroteDownstream: wroteDownstream}
						return
					}
					wroteDownstream = true
					if afterWriteClient != nil {
						afterWriteClient()
					}
					if forwardedFrames != nil {
						forwardedFrames.Add(1)
					}
				}
				markActivity()
				if directive.Exit || directive.Err != nil {
					exitCh <- relayExitSignal{
						stage:           "upstream_control",
						err:             directive.Err,
						graceful:        directive.Graceful && directive.Err == nil,
						wroteDownstream: wroteDownstream,
					}
					return
				}
				if directive.CloseAfterTerminal && directive.CloseAfterTerminalAlreadyWritten {
					exitCh <- relayExitSignal{
						stage:           "route_control_close_after_terminal",
						graceful:        true,
						wroteDownstream: wroteDownstream,
					}
					return
				}
				continue
			}
		}
		observedAt := nowFn()
		observedEvent := observedUpstreamEvent{observedAt: observedAt}
		switch msgType {
		case coderws.MessageText:
			observedEvent = observeUpstreamMessage(
				state,
				payload,
				startAt,
				func() time.Time { return observedAt },
				onUsageParseFailure,
			)
		case coderws.MessageBinary:
			// binary frame 直接透传，不进入 JSON 观测路径（避免无效解析开销）。
		}
		observedEvent.observedAt = observedAt
		decision := stepGate.observe(observedEvent)
		if decision.err != nil {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "upstream_step_protocol_rejected",
				Direction:       "upstream_to_client",
				MessageType:     relayMessageTypeString(msgType),
				PayloadBytes:    len(payload),
				WroteDownstream: wroteDownstream,
				Error:           decision.err.Error(),
			})
			exitCh <- relayExitSignal{
				stage:           "upstream_step_protocol",
				err:             decision.err,
				wroteDownstream: wroteDownstream,
			}
			return
		}
		if decision.consume {
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "stale_upstream_event_consumed",
				Direction:       "upstream_to_client",
				MessageType:     relayMessageTypeString(msgType),
				PayloadBytes:    len(payload),
				WroteDownstream: wroteDownstream,
			})
			markActivity()
			continue
		}
		observedEvent.generation = decision.generation
		observedEvent.requestModel = decision.requestModel
		observedEvent.timingResponseID = decision.timingResponseID
		observedEvent.turnStartedAt = decision.turnStartedAt
		markFirstUpstreamEvent(state, &observedEvent, startAt, observedAt)
		if beforeWriteClient != nil && (!decision.closeConnection || decision.terminalClaimed) {
			if err := beforeWriteClient(msgType, payload, stepWroteDownstream); err != nil {
				if decision.terminalClaimed {
					stepGate.finishTerminal(true)
				}
				emitRelayTrace(onTrace, RelayTraceEvent{
					Stage:           "upstream_message_rejected",
					Direction:       "upstream_to_client",
					MessageType:     relayMessageTypeString(msgType),
					PayloadBytes:    len(payload),
					WroteDownstream: wroteDownstream,
					Error:           err.Error(),
				})
				exitCh <- relayExitSignal{
					stage:           "upstream_message",
					err:             err,
					wroteDownstream: wroteDownstream,
				}
				return
			}
		}
		if !observedEvent.terminal || decision.terminalClaimed {
			applyObservedUpstreamEvent(state, &observedEvent, startAt, onUsageParseFailure)
		}
		if dropDownstreamWrites != nil && dropDownstreamWrites.Load() {
			if droppedFrames != nil {
				droppedFrames.Add(1)
			}
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "drop_downstream_frame",
				Direction:       "upstream_to_client",
				MessageType:     relayMessageTypeString(msgType),
				PayloadBytes:    len(payload),
				WroteDownstream: wroteDownstream,
			})
			if decision.terminalClaimed || decision.closeConnection {
				if decision.terminalClaimed {
					emitTurnComplete(onTurnComplete, state, observedEvent)
					stepGate.finishTerminal(true)
				}
				exitCh <- relayExitSignal{
					stage:           "drain_terminal",
					graceful:        true,
					wroteDownstream: wroteDownstream,
				}
				return
			}
			markActivity()
			continue
		}
		if err := writeClient(msgType, payload); err != nil {
			if decision.terminalClaimed {
				emitTurnComplete(onTurnComplete, state, observedEvent)
				stepGate.finishTerminal(true)
			}
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:           "write_client_failed",
				Direction:       "upstream_to_client",
				MessageType:     relayMessageTypeString(msgType),
				PayloadBytes:    len(payload),
				WroteDownstream: wroteDownstream,
				Error:           err.Error(),
			})
			exitCh <- relayExitSignal{stage: "write_client", err: err, wroteDownstream: wroteDownstream}
			return
		}
		wroteDownstream = true
		stepWroteDownstream = true
		if onProviderFrameWritten != nil {
			onProviderFrameWritten(observedEvent.terminal)
		}
		if afterWriteClient != nil {
			afterWriteClient()
		}
		if decision.terminalClaimed {
			emitTurnComplete(onTurnComplete, state, observedEvent)
			stepGate.finishTerminal(decision.closeGate || closeAfterTerminal)
			if state != nil {
				state.terminalWrittenToClient.Store(true)
			}
		}
		if observedEvent.terminal {
			stepWroteDownstream = false
		}
		if forwardedFrames != nil {
			forwardedFrames.Add(1)
		}
		markActivity()
		if decision.closeConnection || (decision.terminalClaimed && closeAfterTerminal) {
			stage := "terminal_failure"
			if closeAfterTerminal {
				stage = "route_control_close_after_terminal"
			}
			exitCh <- relayExitSignal{
				stage:           stage,
				graceful:        true,
				wroteDownstream: wroteDownstream,
			}
			return
		}
	}
}

func runIdleWatchdog(
	ctx context.Context,
	nowFn func() time.Time,
	idleTimeout time.Duration,
	lastActivity *atomic.Int64,
	onTrace func(event RelayTraceEvent),
	exitCh chan<- relayExitSignal,
) {
	if idleTimeout <= 0 {
		return
	}
	checkInterval := minDuration(idleTimeout/4, 5*time.Second)
	if checkInterval < time.Second {
		checkInterval = time.Second
	}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			last := time.Unix(0, lastActivity.Load())
			if nowFn().Sub(last) < idleTimeout {
				continue
			}
			emitRelayTrace(onTrace, RelayTraceEvent{
				Stage:     "idle_timeout_triggered",
				Direction: "watchdog",
				Error:     context.DeadlineExceeded.Error(),
			})
			exitCh <- relayExitSignal{stage: "idle_timeout", err: context.DeadlineExceeded}
			return
		}
	}
}

func emitRelayTrace(onTrace func(event RelayTraceEvent), event RelayTraceEvent) {
	if onTrace == nil {
		return
	}
	onTrace(event)
}

func relayMessageTypeString(msgType coderws.MessageType) string {
	switch msgType {
	case coderws.MessageText:
		return "text"
	case coderws.MessageBinary:
		return "binary"
	default:
		return "unknown(" + strconv.Itoa(int(msgType)) + ")"
	}
}

func relayDirectionFromStage(stage string) string {
	switch stage {
	case "read_client", "write_upstream":
		return "client_to_upstream"
	case "read_upstream", "write_client", "drain_terminal", "upstream_control", "route_control_close_after_terminal", "terminal_failure", "upstream_step_protocol":
		return "upstream_to_client"
	case "idle_timeout":
		return "watchdog"
	default:
		return ""
	}
}

func relayErrorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func observeUpstreamMessage(
	_ *relayState,
	message []byte,
	_ time.Time,
	nowFn func() time.Time,
	_ func(eventType string, usageRaw string),
) observedUpstreamEvent {
	if len(message) == 0 {
		return observedUpstreamEvent{}
	}
	if envelope, ok := inspectFastUpstreamEventEnvelope(message); ok {
		now := time.Now()
		if nowFn != nil {
			now = nowFn()
		}
		responseID := ""
		responseIDRaw := envelope.responseIDRaw
		if envelope.eventType == "response.created" || isTerminalEvent(envelope.eventType) || isResponseStepBoundaryEvent(envelope.eventType) {
			responseID = string(responseIDRaw)
			responseIDRaw = nil
		}
		return observedUpstreamEvent{
			terminal:      isTerminalEvent(envelope.eventType),
			eventType:     envelope.eventType,
			responseID:    responseID,
			responseIDRaw: responseIDRaw,
			message:       message,
			observedAt:    now,
			tokenEvent:    isTokenEvent(envelope.eventType),
		}
	}
	values := gjson.GetManyBytes(message, "type", "response.id", "response_id", "id")
	eventType, valid := boundedProtocolJSONString(values[0], responseEventTypeMaxBytes)
	if !valid {
		return observedUpstreamEvent{protocolErr: errResponseProtocolIdentifierInvalid}
	}
	if eventType == "" {
		return observedUpstreamEvent{}
	}
	responseID, valid := boundedProtocolJSONString(values[1], responseStepIDMaxBytes)
	if !valid {
		return observedUpstreamEvent{protocolErr: errResponseProtocolIdentifierInvalid}
	}
	if responseID == "" {
		responseID, valid = boundedProtocolJSONString(values[2], responseStepIDMaxBytes)
		if !valid {
			return observedUpstreamEvent{protocolErr: errResponseProtocolIdentifierInvalid}
		}
	}
	// 仅 terminal 事件兜底读取顶层 id，避免把 event_id 当成 response_id 关联到 turn。
	if responseID == "" && eventType != "error" && isTerminalEvent(eventType) {
		responseID, valid = boundedProtocolJSONString(values[3], responseStepIDMaxBytes)
		if !valid {
			return observedUpstreamEvent{protocolErr: errResponseProtocolIdentifierInvalid}
		}
	}
	now := time.Now()
	if nowFn != nil {
		now = nowFn()
	}
	return observedUpstreamEvent{
		terminal:   isTerminalEvent(eventType),
		eventType:  eventType,
		responseID: responseID,
		message:    message,
		observedAt: now,
		tokenEvent: isTokenEvent(eventType),
	}
}

// Check the raw token before String() so an oversized JSON string cannot force
// a large allocation on the relay task or become retained connection state.
func boundedProtocolJSONString(result gjson.Result, maxBytes int) (string, bool) {
	if result.Raw == "" || result.Raw == "null" {
		return "", true
	}
	if result.Type != gjson.String || maxBytes <= 0 || len(result.Raw) > maxBytes+2 {
		return "", false
	}
	value := strings.TrimSpace(result.String())
	if value == "" {
		return "", true
	}
	return value, validResponseStepIdentifier(value, maxBytes)
}

func validResponseStepIdentifier(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && isASCIIString(value)
}

func validResponseStepIdentifierBytes(value []byte, maxBytes int) bool {
	if len(value) == 0 || len(value) > maxBytes {
		return false
	}
	for _, current := range value {
		if current > 0x7f {
			return false
		}
	}
	return true
}

func isASCIIString(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] > 0x7f {
			return false
		}
	}
	return true
}

func applyObservedUpstreamEvent(
	state *relayState,
	observed *observedUpstreamEvent,
	startAt time.Time,
	onUsageParseFailure func(eventType string, usageRaw string),
) {
	if state == nil || observed == nil || observed.eventType == "" {
		return
	}
	now := observed.observedAt
	if now.IsZero() {
		now = time.Now()
	}

	var turnTiming *relayTurnTiming
	timingResponseID := strings.TrimSpace(observed.timingResponseID)
	if timingResponseID == "" {
		timingResponseID = strings.TrimSpace(observed.responseID)
	}
	if timingResponseID != "" {
		turnStartedAt := observed.turnStartedAt
		if turnStartedAt.IsZero() {
			turnStartedAt = now
		}
		turnTiming = openAIWSRelayGetOrInitTurnTiming(state, timingResponseID, turnStartedAt)
	} else if state.activeTurn != nil {
		turnTiming = state.activeTurn
	}
	observed.turnTiming = turnTiming
	if observed.tokenEvent {
		if state.firstTokenMs == nil {
			ms := int(now.Sub(startAt).Milliseconds())
			if ms >= 0 {
				state.firstTokenMs = &ms
			}
		}
		if turnTiming == nil {
			turnTiming = state.activeTurn
			observed.turnTiming = turnTiming
		}
		if turnTiming != nil && turnTiming.firstTokenMs == nil {
			ms := int(now.Sub(turnTiming.startAt).Milliseconds())
			if ms >= 0 {
				turnTiming.firstTokenMs = &ms
			}
		}
	}
	if !observed.terminal {
		return
	}

	observed.usage = parseUsage(observed.message, observed.eventType, onUsageParseFailure)
	accumulateUsage(state, observed.usage)
	state.requestModel = observed.requestModel
	state.terminalEventType = observed.eventType
	state.lastResponseID = strings.TrimSpace(observed.responseID)
	if timingResponseID == "" {
		timingResponseID = state.lastResponseID
	}
	if timingResponseID != "" {
		if completedTiming, ok := openAIWSRelayDeleteTurnTiming(state, timingResponseID); ok {
			duration := now.Sub(completedTiming.startAt)
			if duration < 0 {
				duration = 0
			}
			observed.duration = duration
			observed.firstToken = openAIWSRelayCloneIntPtr(completedTiming.firstTokenMs)
			observed.firstByte = openAIWSRelayCloneIntPtr(completedTiming.firstByteMs)
		}
	} else if !observed.turnStartedAt.IsZero() {
		duration := now.Sub(observed.turnStartedAt)
		if duration < 0 {
			duration = 0
		}
		observed.duration = duration
	}
}

// markFirstUpstreamEvent records the first attributable provider frame when it
// is observed. Client delivery time is intentionally excluded from upstream
// TTFB, matching Aether's stream telemetry boundary.
func markFirstUpstreamEvent(state *relayState, observed *observedUpstreamEvent, relayStartedAt, observedAt time.Time) {
	if state == nil || observed == nil || observedAt.IsZero() {
		return
	}
	turnStartedAt := observed.turnStartedAt
	if observed.turnTiming != nil && !observed.turnTiming.startAt.IsZero() {
		turnStartedAt = observed.turnTiming.startAt
	}
	if turnStartedAt.IsZero() {
		return
	}
	if observed.turnTiming == nil {
		timing := state.activeTurn
		if timing == nil || timing.startAt.IsZero() || !timing.startAt.Equal(turnStartedAt) {
			timing = &relayTurnTiming{startAt: turnStartedAt}
			state.activeTurn = timing
		}
		observed.turnTiming = timing
	}
	turnElapsed := observedAt.Sub(turnStartedAt)
	if turnElapsed < 0 {
		turnElapsed = 0
	}
	if observed.turnTiming != nil && observed.turnTiming.firstByteMs == nil {
		ms := int(turnElapsed.Milliseconds())
		observed.turnTiming.firstByteMs = &ms
	}
	if observed.firstByte == nil {
		if observed.turnTiming != nil {
			observed.firstByte = openAIWSRelayCloneIntPtr(observed.turnTiming.firstByteMs)
		}
		if observed.firstByte == nil {
			ms := int(turnElapsed.Milliseconds())
			observed.firstByte = &ms
		}
	}
	if state.firstByteMs == nil {
		relayElapsed := observedAt.Sub(relayStartedAt)
		if relayElapsed < 0 {
			relayElapsed = 0
		}
		ms := int(relayElapsed.Milliseconds())
		state.firstByteMs = &ms
	}
}

func emitTurnComplete(
	onTurnComplete func(turn RelayTurnResult),
	state *relayState,
	observed observedUpstreamEvent,
) {
	if onTurnComplete == nil || !observed.terminal {
		return
	}
	responseID := strings.TrimSpace(observed.responseID)
	if responseID == "" && !isFailureTerminalEvent(observed.eventType) {
		return
	}
	requestModel := strings.TrimSpace(observed.requestModel)
	if requestModel == "" && state != nil {
		requestModel = state.requestModel
	}
	onTurnComplete(RelayTurnResult{
		RequestModel:      requestModel,
		Usage:             observed.usage,
		RequestID:         responseID,
		TerminalEventType: observed.eventType,
		Duration:          observed.duration,
		FirstTokenMs:      openAIWSRelayCloneIntPtr(observed.firstToken),
		FirstByteMs:       openAIWSRelayCloneIntPtr(observed.firstByte),
	})
}

func openAIWSRelayGetOrInitTurnTiming(state *relayState, responseID string, now time.Time) *relayTurnTiming {
	if state == nil {
		return nil
	}
	if state.turnTimingByID == nil {
		state.turnTimingByID = make(map[string]*relayTurnTiming, 8)
	}
	timing, ok := state.turnTimingByID[responseID]
	if !ok || timing == nil || timing.startAt.IsZero() {
		timing = state.activeTurn
		if timing == nil || timing.startAt.IsZero() || !timing.startAt.Equal(now) {
			timing = &relayTurnTiming{startAt: now}
		}
		state.turnTimingByID[responseID] = timing
		state.activeTurn = timing
		return timing
	}
	return timing
}

func openAIWSRelayDeleteTurnTiming(state *relayState, responseID string) (relayTurnTiming, bool) {
	if state == nil || state.turnTimingByID == nil {
		return relayTurnTiming{}, false
	}
	timing, ok := state.turnTimingByID[responseID]
	if !ok || timing == nil {
		return relayTurnTiming{}, false
	}
	delete(state.turnTimingByID, responseID)
	if state.activeTurn == timing {
		state.activeTurn = nil
	}
	return *timing, true
}

func openAIWSRelayCloneIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func parseUsageAndAccumulate(
	state *relayState,
	message []byte,
	eventType string,
	onParseFailure func(eventType string, usageRaw string),
) Usage {
	if state == nil {
		return Usage{}
	}
	parsedUsage := parseUsage(message, eventType, onParseFailure)
	accumulateUsage(state, parsedUsage)
	return parsedUsage
}

func parseUsage(
	message []byte,
	eventType string,
	onParseFailure func(eventType string, usageRaw string),
) Usage {
	if len(message) == 0 || !shouldParseUsage(eventType) {
		return Usage{}
	}
	usageResult := gjson.GetBytes(message, "response.usage")
	if !usageResult.Exists() {
		return Usage{}
	}
	usageRaw := strings.TrimSpace(usageResult.Raw)
	if usageRaw == "" || !strings.HasPrefix(usageRaw, "{") {
		recordUsageParseFailure()
		if onParseFailure != nil {
			onParseFailure(eventType, usageRaw)
		}
		return Usage{}
	}

	inputResult := gjson.GetBytes(message, "response.usage.input_tokens")
	if !inputResult.Exists() {
		inputResult = gjson.GetBytes(message, "response.usage.prompt_tokens")
	}
	outputResult := gjson.GetBytes(message, "response.usage.output_tokens")
	if !outputResult.Exists() {
		outputResult = gjson.GetBytes(message, "response.usage.completion_tokens")
	}
	cachedResult := gjson.GetBytes(message, "response.usage.input_tokens_details.cached_tokens")
	if !cachedResult.Exists() {
		cachedResult = gjson.GetBytes(message, "response.usage.prompt_tokens_details.cached_tokens")
	}
	imageTokens := usageResult.Get("output_tokens_details.image_tokens").Int()
	if imageTokens == 0 {
		imageTokens = usageResult.Get("completion_tokens_details.image_tokens").Int()
	}

	inputTokens, inputOK := parseUsageIntField(inputResult, true)
	outputTokens, outputOK := parseUsageIntField(outputResult, true)
	cachedTokens, cachedOK := parseUsageIntField(cachedResult, false)
	if !inputOK || !outputOK || !cachedOK {
		recordUsageParseFailure()
		if onParseFailure != nil {
			onParseFailure(eventType, usageRaw)
		}
		// 解析失败时不做部分字段累加，避免计费 usage 出现“半有效”状态。
		return Usage{}
	}
	parsedUsage := Usage{
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheCreationInputTokens: openAICacheCreationTokensFromUsage(usageResult),
		CacheReadInputTokens:     cachedTokens,
		ImageOutputTokens:        int(imageTokens),
	}
	return parsedUsage
}

func accumulateUsage(state *relayState, parsedUsage Usage) {
	if state == nil {
		return
	}
	state.usage.InputTokens += parsedUsage.InputTokens
	state.usage.OutputTokens += parsedUsage.OutputTokens
	state.usage.CacheCreationInputTokens += parsedUsage.CacheCreationInputTokens
	state.usage.CacheReadInputTokens += parsedUsage.CacheReadInputTokens
	state.usage.ImageOutputTokens += parsedUsage.ImageOutputTokens
}

func parseUsageIntField(value gjson.Result, required bool) (int, bool) {
	if !value.Exists() {
		return 0, !required
	}
	if value.Type != gjson.Number {
		return 0, false
	}
	return int(value.Int()), true
}

func openAICacheCreationTokensFromUsage(value gjson.Result) int {
	for _, field := range []string{
		"input_tokens_details.cache_write_tokens",
		"prompt_tokens_details.cache_write_tokens",
		"input_tokens_details.cache_creation_tokens",
		"prompt_tokens_details.cache_creation_tokens",
	} {
		result := value.Get(field)
		if result.Exists() {
			return max(int(result.Int()), 0)
		}
	}
	for _, field := range []string{
		"cache_write_tokens",
		"cache_creation_input_tokens",
		"cache_write_input_tokens",
		"cache_creation_tokens",
	} {
		if tokens := int(value.Get(field).Int()); tokens > 0 {
			return tokens
		}
	}
	return 0
}

func enrichResult(result *RelayResult, state *relayState, duration time.Duration) {
	if result == nil {
		return
	}
	result.Duration = duration
	if state == nil {
		return
	}
	result.RequestModel = state.requestModel
	result.Usage = state.usage
	result.RequestID = state.lastResponseID
	result.TerminalEventType = state.terminalEventType
	result.FirstTokenMs = state.firstTokenMs
	result.FirstByteMs = state.firstByteMs
}

func isDisconnectError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
		return true
	}
	switch coderws.CloseStatus(err) {
	case coderws.StatusNormalClosure, coderws.StatusGoingAway, coderws.StatusNoStatusRcvd, coderws.StatusAbnormalClosure:
		return true
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}
	return strings.Contains(message, "failed to read frame header: eof") ||
		strings.Contains(message, "unexpected eof") ||
		strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "broken pipe")
}

func isTerminalEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
		return true
	default:
		return false
	}
}

func isFailureTerminalEvent(eventType string) bool {
	return eventType == "response.failed" || eventType == "response.incomplete" || eventType == "error"
}

func isResponseStepBoundaryEvent(eventType string) bool {
	switch eventType {
	case "response.in_progress", "response.output_item.added", "response.output_item.done":
		return true
	default:
		return false
	}
}

func shouldParseUsage(eventType string) bool {
	switch eventType {
	case "response.completed", "response.failed", "response.incomplete":
		return true
	default:
		return false
	}
}

func isTokenEvent(eventType string) bool {
	if eventType == "" {
		return false
	}
	switch eventType {
	case "response.created", "response.in_progress", "response.output_item.added", "response.output_item.done",
		"response.done", "response.cancelled", "response.canceled":
		return false
	}
	if strings.Contains(eventType, ".delta") {
		return true
	}
	if strings.HasPrefix(eventType, "response.output_text") {
		return true
	}
	if strings.HasPrefix(eventType, "response.output") {
		return true
	}
	return eventType == "response.completed"
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func waitRelayExit(exitCh <-chan relayExitSignal, timeout time.Duration) (relayExitSignal, bool) {
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
	}
	select {
	case sig := <-exitCh:
		return sig, true
	case <-time.After(timeout):
		return relayExitSignal{}, false
	}
}
