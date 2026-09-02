package openai_ws_v2

import (
	"context"
	"errors"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type passthroughTestFrame struct {
	msgType coderws.MessageType
	payload []byte
}

type passthroughTestFrameConn struct {
	mu     sync.Mutex
	writes []passthroughTestFrame
	readCh chan passthroughTestFrame
	once   sync.Once
}

type delayedReadFrameConn struct {
	base       FrameConn
	firstDelay time.Duration
	once       sync.Once
}

type closeSpyFrameConn struct {
	closeCalls atomic.Int32
}

type closeTrackingFrameConn struct {
	FrameConn
	closeCalls atomic.Int32
}

type closeAfterWriteFrameConn struct {
	*passthroughTestFrameConn
	closeAfter []byte
}

func newPassthroughTestFrameConn(frames []passthroughTestFrame, autoClose bool) *passthroughTestFrameConn {
	c := &passthroughTestFrameConn{
		readCh: make(chan passthroughTestFrame, len(frames)+1),
	}
	for _, frame := range frames {
		copied := passthroughTestFrame{msgType: frame.msgType, payload: append([]byte(nil), frame.payload...)}
		c.readCh <- copied
	}
	if autoClose {
		close(c.readCh)
	}
	return c
}

func (c *passthroughTestFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return coderws.MessageText, nil, ctx.Err()
	case frame, ok := <-c.readCh:
		if !ok {
			return coderws.MessageText, nil, io.EOF
		}
		return frame.msgType, append([]byte(nil), frame.payload...), nil
	}
}

func (c *passthroughTestFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, passthroughTestFrame{msgType: msgType, payload: append([]byte(nil), payload...)})
	return nil
}

func (c *passthroughTestFrameConn) Close() error {
	c.once.Do(func() {
		defer func() { _ = recover() }()
		close(c.readCh)
	})
	return nil
}

func (c *passthroughTestFrameConn) Writes() []passthroughTestFrame {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]passthroughTestFrame, len(c.writes))
	copy(out, c.writes)
	return out
}

func (c *delayedReadFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if c == nil || c.base == nil {
		return coderws.MessageText, nil, io.EOF
	}
	c.once.Do(func() {
		if c.firstDelay > 0 {
			timer := time.NewTimer(c.firstDelay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
			case <-timer.C:
			}
		}
	})
	return c.base.ReadFrame(ctx)
}

func (c *delayedReadFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if c == nil || c.base == nil {
		return io.EOF
	}
	return c.base.WriteFrame(ctx, msgType, payload)
}

func (c *delayedReadFrameConn) Close() error {
	if c == nil || c.base == nil {
		return nil
	}
	return c.base.Close()
}

func (c *closeSpyFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	<-ctx.Done()
	return coderws.MessageText, nil, ctx.Err()
}

func (c *closeSpyFrameConn) WriteFrame(ctx context.Context, _ coderws.MessageType, _ []byte) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (c *closeSpyFrameConn) Close() error {
	if c != nil {
		c.closeCalls.Add(1)
	}
	return nil
}

func (c *closeSpyFrameConn) CloseCalls() int32 {
	if c == nil {
		return 0
	}
	return c.closeCalls.Load()
}

func (c *closeTrackingFrameConn) Close() error {
	if c == nil {
		return nil
	}
	c.closeCalls.Add(1)
	if c.FrameConn == nil {
		return nil
	}
	return c.FrameConn.Close()
}

func (c *closeAfterWriteFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if err := c.passthroughTestFrameConn.WriteFrame(ctx, msgType, payload); err != nil {
		return err
	}
	if string(payload) == string(c.closeAfter) {
		return c.Close()
	}
	return nil
}

func TestRelay_BasicRelayAndUsage(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_123"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_123","usage":{"input_tokens":7,"output_tokens":3,"input_tokens_details":{"cached_tokens":2}}}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[{"type":"input_text","text":"hello"}]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	require.Nil(t, relayExit)
	require.Equal(t, "gpt-5.3-codex", result.RequestModel)
	require.Equal(t, "resp_123", result.RequestID)
	require.Equal(t, "response.completed", result.TerminalEventType)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.Equal(t, 2, result.Usage.CacheReadInputTokens)
	require.NotNil(t, result.FirstTokenMs)
	require.Equal(t, int64(1), result.ClientToUpstreamFrames)
	require.Equal(t, int64(2), result.UpstreamToClientFrames)
	require.Equal(t, int64(0), result.DroppedDownstreamFrames)

	upstreamWrites := upstreamConn.Writes()
	require.Len(t, upstreamWrites, 1)
	require.Equal(t, coderws.MessageText, upstreamWrites[0].msgType)
	require.JSONEq(t, string(firstPayload), string(upstreamWrites[0].payload))

	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 2)
	require.Equal(t, coderws.MessageText, clientWrites[0].msgType)
	require.JSONEq(t, `{"type":"response.created","response":{"id":"resp_123"}}`, string(clientWrites[0].payload))
	require.JSONEq(t, `{"type":"response.completed","response":{"id":"resp_123","usage":{"input_tokens":7,"output_tokens":3,"input_tokens_details":{"cached_tokens":2}}}}`, string(clientWrites[1].payload))
}

func TestRelay_RejectsSecondCreateBeforeTerminalWithoutUpstreamWrite(t *testing.T) {
	t.Parallel()
	var transformCalls atomic.Int32

	clientConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.create","model":"gpt-5","input":"second"}`),
		},
	}, true)
	upstreamBase := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_first","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}, true)
	upstreamConn := &delayedReadFrameConn{base: upstreamBase, firstDelay: 100 * time.Millisecond}

	_, relayExit := Relay(
		context.Background(),
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-5","input":"first"}`),
		RelayOptions{
			TransformResponseCreate: func(payload []byte, _ ClientEnvelope) ([]byte, error) {
				transformCalls.Add(1)
				return payload, nil
			},
		},
	)

	require.NotNil(t, relayExit)
	require.Equal(t, "write_upstream", relayExit.Stage)
	require.ErrorIs(t, relayExit.Err, errResponseCreateInFlight)
	require.Len(t, upstreamBase.Writes(), 1, "only the initial response.create may reach upstream")
	require.Zero(t, transformCalls.Load(), "a rejected concurrent step must not mutate the active step fence")
}

func TestRelay_BeforeWriteClientUsesStepLocalDownstreamState(t *testing.T) {
	t.Parallel()

	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_first"}}`)},
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.output_text.delta","delta":"first"}`)},
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.completed","response":{"id":"resp_first","usage":{"input_tokens":1,"output_tokens":1}}}`)},
		{msgType: coderws.MessageText, payload: []byte(`{"type":"codex.rate_limits","remaining":10}`)},
	}, true)

	var stepStates []bool
	dropDownstreamWrites := &atomic.Bool{}
	exitCh := make(chan relayExitSignal, 1)
	runUpstreamToClient(
		context.Background(),
		upstreamConn,
		func(_ coderws.MessageType, _ []byte) error { return nil },
		time.Now(),
		time.Now,
		&relayState{},
		nil,
		nil,
		func(_ coderws.MessageType, _ []byte, stepWroteDownstream bool) error {
			stepStates = append(stepStates, stepWroteDownstream)
			return nil
		},
		nil,
		dropDownstreamWrites,
		nil,
		nil,
		func() {},
		nil,
		newResponseStepGate(),
		exitCh,
	)

	exit := <-exitCh
	require.Equal(t, "read_upstream", exit.stage)
	require.ErrorIs(t, exit.err, io.EOF)
	require.Equal(t, []bool{false, true, true, false}, stepStates)
}

func TestRelay_ConsumesControlAndClosesOnlyAfterTerminalWrite(t *testing.T) {
	t.Parallel()

	controlPayload := []byte(`{"type":"aether.route_control","action":"close_after_terminal"}`)
	deltaPayload := []byte(`{"type":"response.output_text.delta","response_id":"resp_control","delta":"hello"}`)
	terminalPayload := []byte(`{"type":"response.completed","response":{"id":"resp_control","usage":{"input_tokens":1,"output_tokens":1}}}`)
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: controlPayload},
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_control"}}`)},
		{msgType: coderws.MessageText, payload: deltaPayload},
		{msgType: coderws.MessageText, payload: terminalPayload},
	}, true)

	var providerWritesMu sync.Mutex
	providerWrites := make([]bool, 0, 2)
	turnCompleted := atomic.Bool{}
	result, relayExit := Relay(
		context.Background(),
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-5"}`),
		RelayOptions{
			InterceptUpstreamFrame: func(_ coderws.MessageType, payload []byte) UpstreamFrameDirective {
				if string(payload) != string(controlPayload) {
					return UpstreamFrameDirective{}
				}
				return UpstreamFrameDirective{Consume: true, CloseAfterTerminal: true}
			},
			OnProviderFrameWritten: func(terminal bool) {
				providerWritesMu.Lock()
				providerWrites = append(providerWrites, terminal)
				providerWritesMu.Unlock()
			},
			OnTurnComplete: func(RelayTurnResult) {
				turnCompleted.Store(true)
			},
		},
	)

	require.Nil(t, relayExit)
	require.True(t, turnCompleted.Load(), "terminal callback must finish before controlled close")
	require.Equal(t, int64(3), result.UpstreamToClientFrames)
	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 3)
	require.JSONEq(t, `{"type":"response.created","response":{"id":"resp_control"}}`, string(clientWrites[0].payload))
	require.Equal(t, deltaPayload, clientWrites[1].payload)
	require.Equal(t, terminalPayload, clientWrites[2].payload)
	for _, write := range clientWrites {
		require.NotContains(t, string(write.payload), "aether.route_control")
	}
	providerWritesMu.Lock()
	require.Equal(t, []bool{false, false, true}, providerWrites)
	providerWritesMu.Unlock()
}

func TestRelay_ConsumesCloseControlAfterTerminalWrite(t *testing.T) {
	t.Parallel()

	controlPayload := []byte(`{"type":"aether.route_control","action":"close_after_terminal"}`)
	terminalPayload := []byte(`{"type":"response.completed","response":{"id":"resp_late_control","usage":{"input_tokens":1,"output_tokens":1}}}`)
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_late_control"}}`)},
		{msgType: coderws.MessageText, payload: terminalPayload},
		{msgType: coderws.MessageText, payload: controlPayload},
	}, true)

	turnCompleted := atomic.Bool{}
	result, relayExit := Relay(
		context.Background(),
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-5"}`),
		RelayOptions{
			InterceptUpstreamFrame: func(_ coderws.MessageType, payload []byte) UpstreamFrameDirective {
				if string(payload) != string(controlPayload) {
					return UpstreamFrameDirective{}
				}
				return UpstreamFrameDirective{
					Consume:                          true,
					CloseAfterTerminal:               true,
					CloseAfterTerminalAlreadyWritten: true,
				}
			},
			OnTurnComplete: func(RelayTurnResult) {
				turnCompleted.Store(true)
			},
		},
	)

	require.Nil(t, relayExit)
	require.True(t, turnCompleted.Load())
	require.Equal(t, int64(2), result.UpstreamToClientFrames)
	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 2)
	require.Equal(t, terminalPayload, clientWrites[1].payload)
	for _, write := range clientWrites {
		require.NotContains(t, string(write.payload), "aether.route_control")
	}
}

func TestRelay_CloseControlForSecondTurnWaitsForSecondTerminal(t *testing.T) {
	t.Parallel()

	controlPayload := []byte(`{"type":"aether.route_control","action":"close_after_terminal"}`)
	firstTerminal := []byte(`{"type":"response.completed","response":{"id":"resp_first","usage":{"input_tokens":1,"output_tokens":1}}}`)
	secondTerminal := []byte(`{"type":"response.completed","response":{"id":"resp_second","usage":{"input_tokens":2,"output_tokens":1}}}`)
	const providerFenceDelay = 30 * time.Millisecond
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn(nil, false)
	secondDispatched := make(chan struct{})
	feederDone := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		defer close(feederDone)
		defer func() { _ = upstreamConn.Close() }()
		send := func(frame passthroughTestFrame) bool {
			select {
			case upstreamConn.readCh <- frame:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if !send(passthroughTestFrame{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_first"}}`)}) ||
			!send(passthroughTestFrame{msgType: coderws.MessageText, payload: firstTerminal}) {
			return
		}
		select {
		case <-secondDispatched:
		case <-ctx.Done():
			return
		}
		for _, frame := range []passthroughTestFrame{
			{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_second"}}`)},
			{msgType: coderws.MessageText, payload: controlPayload},
			{msgType: coderws.MessageText, payload: secondTerminal},
		} {
			if !send(frame) {
				return
			}
		}
	}()

	var secondTurn RelayTurnResult
	result, relayExit := Relay(
		ctx,
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-5","input":"first"}`),
		RelayOptions{
			BeforeDispatchResponseCreate: func(_ coderws.MessageType, _ []byte, _ string) error {
				time.Sleep(providerFenceDelay)
				close(secondDispatched)
				return nil
			},
			InterceptUpstreamFrame: func(_ coderws.MessageType, payload []byte) UpstreamFrameDirective {
				if string(payload) != string(controlPayload) {
					return UpstreamFrameDirective{}
				}
				return UpstreamFrameDirective{Consume: true, CloseAfterTerminal: true}
			},
			OnTurnComplete: func(turn RelayTurnResult) {
				if turn.RequestID == "resp_second" {
					secondTurn = turn
					return
				}
				if turn.RequestID != "resp_first" {
					return
				}
				select {
				case clientConn.readCh <- passthroughTestFrame{msgType: coderws.MessageText, payload: []byte(`{"type":"response.create","model":"gpt-5","input":"second"}`)}:
				case <-ctx.Done():
				}
			},
		},
	)
	<-feederDone

	require.Nil(t, relayExit)
	require.Equal(t, "response.completed", result.TerminalEventType)
	require.Equal(t, "resp_second", result.RequestID)
	require.NotNil(t, secondTurn.FirstByteMs)
	require.GreaterOrEqual(
		t,
		*secondTurn.FirstByteMs,
		int((providerFenceDelay - 5*time.Millisecond).Milliseconds()),
	)
	require.Equal(t, int64(4), result.UpstreamToClientFrames)
	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 4)
	require.Equal(t, firstTerminal, clientWrites[1].payload)
	require.Equal(t, secondTerminal, clientWrites[3].payload)
	for _, write := range clientWrites {
		require.NotContains(t, string(write.payload), "aether.route_control")
	}
}

func TestRelay_ControlReconnectWritesReplacementAndNeverForwardsControl(t *testing.T) {
	t.Parallel()

	controlPayload := []byte(`{"type":"aether.route_control","action":"client_reconnect"}`)
	pinnedPayload := []byte(`{"type":"error","error":{"code":"websocket_connection_limit_reached"}}`)
	controlErr := errors.New("controlled reconnect")
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: controlPayload},
	}, true)

	_, relayExit := Relay(
		context.Background(),
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-5"}`),
		RelayOptions{
			InterceptUpstreamFrame: func(_ coderws.MessageType, _ []byte) UpstreamFrameDirective {
				return UpstreamFrameDirective{
					Consume:           true,
					ClientMessageType: coderws.MessageText,
					ClientPayload:     pinnedPayload,
					Exit:              true,
					Err:               controlErr,
				}
			},
		},
	)

	require.NotNil(t, relayExit)
	require.Equal(t, "upstream_control", relayExit.Stage)
	require.ErrorIs(t, relayExit.Err, controlErr)
	require.True(t, relayExit.WroteDownstream)
	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 1)
	require.Equal(t, pinnedPayload, clientWrites[0].payload)
	require.NotContains(t, string(clientWrites[0].payload), "aether.route_control")
}

func TestRelay_InitialControlFailoverDoesNotConsumeNextClientFrame(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	persistentFrames := make(chan passthroughTestFrame)
	readClientFrame := func(ctx context.Context, _ FrameConn) (coderws.MessageType, []byte, error) {
		select {
		case frame := <-persistentFrames:
			return frame.msgType, frame.payload, nil
		case <-ctx.Done():
			return coderws.MessageText, nil, ctx.Err()
		}
	}
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: []byte(`{"type":"aether.route_control","action":"client_reconnect"}`)},
	}, true)
	controlErr := errors.New("initial account failover")

	_, relayExit := Relay(
		context.Background(),
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-5","input":"first"}`),
		RelayOptions{
			ReadClientFrame: readClientFrame,
			InterceptUpstreamFrame: func(_ coderws.MessageType, _ []byte) UpstreamFrameDirective {
				return UpstreamFrameDirective{Consume: true, Err: controlErr}
			},
		},
	)

	require.NotNil(t, relayExit)
	require.ErrorIs(t, relayExit.Err, controlErr)

	nextClientPayload := []byte(`{"type":"response.create","model":"gpt-5","input":"next"}`)
	go func() {
		persistentFrames <- passthroughTestFrame{msgType: coderws.MessageText, payload: nextClientPayload}
	}()
	readCtx, cancelRead := context.WithTimeout(context.Background(), time.Second)
	defer cancelRead()
	_, preserved, err := readClientFrame(readCtx, clientConn)
	require.NoError(t, err)
	require.Equal(t, nextClientPayload, preserved)
}

func TestRelay_RequireClientTextFramesRejectsBinaryBeforeHooksAndUpstreamWrite(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn([]passthroughTestFrame{{
		msgType: coderws.MessageBinary,
		payload: []byte(`{"type":"response.create","model":"gpt-5","input":"bypass"}`),
	}}, false)
	upstreamConn := newPassthroughTestFrameConn(nil, false)
	var hookCalls atomic.Int32

	_, relayExit := Relay(
		context.Background(),
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-5","input":"first"}`),
		RelayOptions{
			RequireClientTextFrames: true,
			BeforeWriteUpstreamFrame: func(coderws.MessageType, []byte, ClientEnvelope) error {
				hookCalls.Add(1)
				return nil
			},
			BeforeWriteResponseCreate: func(coderws.MessageType, []byte, string) error {
				hookCalls.Add(1)
				return nil
			},
		},
	)

	require.NotNil(t, relayExit)
	require.Equal(t, "write_upstream", relayExit.Stage)
	require.Contains(t, relayExit.Err.Error(), "binary frames")
	require.Zero(t, hookCalls.Load())
	require.Len(t, upstreamConn.Writes(), 1, "only the already-admitted first text frame may reach upstream")
}

func TestRelay_ResponseCreateTransformWaitsForTerminalCallback(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_first"}}`)},
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.completed","response":{"id":"resp_first","usage":{"input_tokens":1,"output_tokens":1}}}`)},
	}, false)
	terminalCallbackEntered := make(chan struct{})
	releaseTerminalCallback := make(chan struct{})
	transformed := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = Relay(
			ctx,
			clientConn,
			upstreamConn,
			[]byte(`{"type":"response.create","model":"gpt-5","input":"first"}`),
			RelayOptions{
				StartClientAfterFirstDownstream: true,
				OnTurnComplete: func(RelayTurnResult) {
					close(terminalCallbackEntered)
					<-releaseTerminalCallback
				},
				TransformResponseCreate: func(payload []byte, _ ClientEnvelope) ([]byte, error) {
					transformed <- struct{}{}
					return payload, nil
				},
			},
		)
	}()

	select {
	case <-terminalCallbackEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("terminal callback did not start")
	}
	clientConn.readCh <- passthroughTestFrame{
		msgType: coderws.MessageText,
		payload: []byte(`{"type":"response.create","model":"gpt-5","input":"second"}`),
	}
	select {
	case <-transformed:
		t.Fatal("next step transformed before terminal callback released the step gate")
	case <-time.After(75 * time.Millisecond):
	}
	close(releaseTerminalCallback)
	select {
	case <-transformed:
	case <-time.After(2 * time.Second):
		t.Fatal("next step was not transformed after terminal callback completed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not stop after cancellation")
	}
}

func TestRelay_FunctionCallOutputBytesPreserved(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_func"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_func","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[{"type":"function_call_output","call_id":"call_abc123","output":"{\"ok\":true}"}]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	require.Nil(t, relayExit)

	upstreamWrites := upstreamConn.Writes()
	require.Len(t, upstreamWrites, 1)
	require.Equal(t, coderws.MessageText, upstreamWrites[0].msgType)
	require.Equal(t, firstPayload, upstreamWrites[0].payload)
}

func TestRelay_UpstreamDisconnect(t *testing.T) {
	t.Parallel()

	// 上游立即关闭（EOF），客户端不发送额外帧
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn(nil, true) // 立即 close -> EOF

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	require.NotNil(t, relayExit, "上游未发送终态事件前断开必须返回错误")
	require.Equal(t, "read_upstream", relayExit.Stage)
	require.ErrorIs(t, relayExit.Err, io.EOF)
	require.Equal(t, "gpt-4o", result.RequestModel)
}

func TestRelay_ClientDisconnect(t *testing.T) {
	t.Parallel()

	// 客户端立即关闭（EOF），上游阻塞读取直到 context 取消
	clientConn := newPassthroughTestFrameConn(nil, true) // 立即 close -> EOF
	upstreamConn := newPassthroughTestFrameConn(nil, false)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	require.NotNil(t, relayExit, "客户端 EOF 应返回可观测的中断状态")
	require.Equal(t, "client_disconnected", relayExit.Stage)
	require.Equal(t, "gpt-4o", result.RequestModel)
}

func TestRelay_FirstMessageAlreadySentStillObservesClientDisconnectBeforeTTFT(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, true)
	upstreamConn := newPassthroughTestFrameConn(nil, false)
	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		FirstMessageSent: true,
	})
	require.NotNil(t, relayExit)
	require.Equal(t, "client_disconnected", relayExit.Stage)
}

func TestRelay_ClientDisconnectAfterTerminalWriteCompletes(t *testing.T) {
	t.Parallel()

	cancelPayload := []byte(`{"type":"aether.turn.cancel"}`)
	terminalPayload := []byte(`{"type":"response.completed","response":{"id":"resp_done","usage":{"input_tokens":2,"output_tokens":1}}}`)
	clientConn := &closeAfterWriteFrameConn{
		passthroughTestFrameConn: newPassthroughTestFrameConn(nil, false),
		closeAfter:               terminalPayload,
	}
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_done"}}`)},
		{msgType: coderws.MessageText, payload: terminalPayload},
	}, false)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, relayExit := Relay(
		ctx,
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-4o","input":[]}`),
		RelayOptions{
			UpstreamDrainTimeout:            100 * time.Millisecond,
			ClientDisconnectUpstreamPayload: cancelPayload,
		},
	)

	require.Nil(t, relayExit)
	require.Equal(t, "response.completed", result.TerminalEventType)
	require.Equal(t, int64(2), result.UpstreamToClientFrames)
	require.Zero(t, result.DroppedDownstreamFrames)
	require.Len(t, upstreamConn.Writes(), 1, "a settled turn must not receive a cancel signal")
}

func TestRelay_ClientDisconnect_DrainCapturesLateUsage(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, true)
	upstreamBase := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_drain"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_drain","usage":{"input_tokens":6,"output_tokens":4,"input_tokens_details":{"cached_tokens":1}}}}`),
		},
	}, true)
	upstreamConn := &delayedReadFrameConn{
		base:       upstreamBase,
		firstDelay: 80 * time.Millisecond,
	}

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		UpstreamDrainTimeout: 400 * time.Millisecond,
	})
	require.NotNil(t, relayExit)
	require.Equal(t, "client_disconnected", relayExit.Stage)
	require.Equal(t, "resp_drain", result.RequestID)
	require.Equal(t, "response.completed", result.TerminalEventType)
	require.Equal(t, 6, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 1, result.Usage.CacheReadInputTokens)
	require.Equal(t, int64(1), result.ClientToUpstreamFrames)
	require.Equal(t, int64(0), result.UpstreamToClientFrames)
	require.Equal(t, int64(2), result.DroppedDownstreamFrames)
}

func TestRelay_ClientDisconnectSignalsInFlightUpstreamAndCapturesCancelledUsage(t *testing.T) {
	t.Parallel()

	cancelPayload := []byte(`{"type":"aether.turn.cancel"}`)
	terminalPayload := []byte(`{"type":"response.cancelled","response":{"usage":{"input_tokens":8,"output_tokens":1,"input_tokens_details":{"cached_tokens":7}}}}`)
	clientConn := newPassthroughTestFrameConn(nil, true)
	upstreamBase := newPassthroughTestFrameConn(nil, false)
	var terminalOnce sync.Once
	upstreamConn := &scriptedUpstreamFrameConn{
		base: upstreamBase,
		onWrite: func(payload []byte) {
			if string(payload) != string(cancelPayload) {
				return
			}
			terminalOnce.Do(func() {
				upstreamBase.readCh <- passthroughTestFrame{
					msgType: coderws.MessageText,
					payload: terminalPayload,
				}
			})
		},
	}
	turns := make([]RelayTurnResult, 0, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, relayExit := Relay(
		ctx,
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-5.6-sol"}`),
		RelayOptions{
			ClientDisconnectUpstreamPayload: cancelPayload,
			UpstreamDrainTimeout:            400 * time.Millisecond,
			OnTurnComplete: func(turn RelayTurnResult) {
				turns = append(turns, turn)
			},
		},
	)

	require.NotNil(t, relayExit)
	require.Equal(t, "client_disconnected", relayExit.Stage)
	require.Equal(t, "response.cancelled", result.TerminalEventType)
	require.Equal(t, 8, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.CacheReadInputTokens)
	require.Equal(t, 1, result.Usage.OutputTokens)
	require.Len(t, turns, 1)
	require.Equal(t, result.Usage, turns[0].Usage)
	require.Equal(t, int64(1), result.ClientToUpstreamFrames)
	require.Equal(t, int64(1), result.DroppedDownstreamFrames)

	upstreamWrites := upstreamBase.Writes()
	require.Len(t, upstreamWrites, 2)
	require.JSONEq(t, string(cancelPayload), string(upstreamWrites[1].payload))
}

func TestRelay_IdleTimeout(t *testing.T) {
	t.Parallel()

	// 客户端和上游都不发送帧，idle timeout 应触发
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn(nil, false)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 使用快进时间来加速 idle timeout
	now := time.Now()
	callCount := 0
	nowFn := func() time.Time {
		callCount++
		// 前几次调用返回正常时间（初始化阶段），之后快进
		if callCount <= 5 {
			return now
		}
		return now.Add(time.Hour) // 快进到超时
	}

	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		IdleTimeout: 2 * time.Second,
		Now:         nowFn,
	})
	require.NotNil(t, relayExit, "应因 idle timeout 退出")
	require.Equal(t, "idle_timeout", relayExit.Stage)
	require.Equal(t, "gpt-4o", result.RequestModel)
}

func TestRelay_IdleTimeoutDoesNotCloseClientOnError(t *testing.T) {
	t.Parallel()

	clientConn := &closeSpyFrameConn{}
	upstreamConn := &closeSpyFrameConn{}

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	callCount := 0
	nowFn := func() time.Time {
		callCount++
		if callCount <= 5 {
			return now
		}
		return now.Add(time.Hour)
	}

	_, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		IdleTimeout: 2 * time.Second,
		Now:         nowFn,
	})
	require.NotNil(t, relayExit, "应因 idle timeout 退出")
	require.Equal(t, "idle_timeout", relayExit.Stage)
	require.Zero(t, clientConn.CloseCalls(), "错误路径不应提前关闭客户端连接，交给上层决定 close code")
	require.GreaterOrEqual(t, upstreamConn.CloseCalls(), int32(1))
}

func TestRelay_NilConnections(t *testing.T) {
	t.Parallel()

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx := context.Background()

	t.Run("nil client conn", func(t *testing.T) {
		upstreamConn := newPassthroughTestFrameConn(nil, true)
		_, relayExit := Relay(ctx, nil, upstreamConn, firstPayload, RelayOptions{})
		require.NotNil(t, relayExit)
		require.Equal(t, "relay_init", relayExit.Stage)
		require.Contains(t, relayExit.Err.Error(), "nil")
	})

	t.Run("nil upstream conn", func(t *testing.T) {
		clientConn := newPassthroughTestFrameConn(nil, true)
		_, relayExit := Relay(ctx, clientConn, nil, firstPayload, RelayOptions{})
		require.NotNil(t, relayExit)
		require.Equal(t, "relay_init", relayExit.Stage)
		require.Contains(t, relayExit.Err.Error(), "nil")
	})
}

func TestRelay_MultipleUpstreamMessages(t *testing.T) {
	t.Parallel()

	// 上游发送多个事件（delta + completed），验证多帧中继和 usage 聚合
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_multi"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.output_text.delta","delta":"Hello"}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.output_text.delta","delta":" world"}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_multi","usage":{"input_tokens":10,"output_tokens":5,"input_tokens_details":{"cached_tokens":3}}}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[{"type":"input_text","text":"hi"}]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	require.Nil(t, relayExit)
	require.Equal(t, "resp_multi", result.RequestID)
	require.Equal(t, "response.completed", result.TerminalEventType)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.NotNil(t, result.FirstTokenMs)

	// 验证所有 4 个上游帧都转发给了客户端
	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 4)
}

func TestRelay_OnTurnComplete_ConsumesDuplicateTerminalExactlyOnce(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_turn_1"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_turn_1","usage":{"input_tokens":2,"output_tokens":1}}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_turn_1","usage":{"input_tokens":200,"output_tokens":100}}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	turns := make([]RelayTurnResult, 0, 1)
	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		OnTurnComplete: func(turn RelayTurnResult) {
			turns = append(turns, turn)
		},
	})
	require.Nil(t, relayExit)
	require.Len(t, turns, 1)
	require.Equal(t, "resp_turn_1", turns[0].RequestID)
	require.Equal(t, "response.completed", turns[0].TerminalEventType)
	require.Equal(t, 2, turns[0].Usage.InputTokens)
	require.Equal(t, 1, turns[0].Usage.OutputTokens)
	require.Equal(t, 2, result.Usage.InputTokens)
	require.Equal(t, 1, result.Usage.OutputTokens)
	require.Len(t, clientConn.Writes(), 2, "duplicate terminal must not be forwarded")
}

func TestRelay_IDLessFailureTerminalForwardsCallbacksAndCloses(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{"response.failed", "response.incomplete"} {
		eventType := eventType
		t.Run(eventType, func(t *testing.T) {
			t.Parallel()

			payload := []byte(`{"type":"` + eventType + `","response":{"usage":{"input_tokens":3,"output_tokens":1}}}`)
			clientConn := newPassthroughTestFrameConn(nil, false)
			upstreamBase := newPassthroughTestFrameConn([]passthroughTestFrame{
				{msgType: coderws.MessageText, payload: payload},
			}, true)
			upstreamConn := &closeTrackingFrameConn{FrameConn: upstreamBase}
			turns := make([]RelayTurnResult, 0, 1)

			result, relayExit := Relay(
				context.Background(),
				clientConn,
				upstreamConn,
				[]byte(`{"type":"response.create","model":"gpt-failure"}`),
				RelayOptions{OnTurnComplete: func(turn RelayTurnResult) {
					turns = append(turns, turn)
				}},
			)

			require.Nil(t, relayExit)
			require.Equal(t, eventType, result.TerminalEventType)
			require.Empty(t, result.RequestID)
			require.Equal(t, 3, result.Usage.InputTokens)
			require.Equal(t, "gpt-failure", result.RequestModel)
			require.Len(t, turns, 1)
			require.Empty(t, turns[0].RequestID)
			require.Equal(t, eventType, turns[0].TerminalEventType)
			require.Equal(t, "gpt-failure", turns[0].RequestModel)
			require.Equal(t, 3, turns[0].Usage.InputTokens)
			require.Len(t, clientConn.Writes(), 1)
			require.GreaterOrEqual(t, upstreamConn.closeCalls.Load(), int32(1), "failure terminal must close the physical upstream")
		})
	}
}

func TestRelay_CancelledTerminalForwardsUsageToBillingCallback(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{"response.cancelled", "response.canceled"} {
		eventType := eventType
		t.Run(eventType, func(t *testing.T) {
			t.Parallel()

			payload := []byte(`{"type":"` + eventType + `","response":{"id":"resp-cancelled","usage":{"input_tokens":119404,"input_tokens_details":{"cached_tokens":119040},"output_tokens":301,"total_tokens":119705}}}`)
			clientConn := newPassthroughTestFrameConn(nil, false)
			upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
				{msgType: coderws.MessageText, payload: payload},
			}, true)
			turns := make([]RelayTurnResult, 0, 1)

			result, relayExit := Relay(
				context.Background(),
				clientConn,
				upstreamConn,
				[]byte(`{"type":"response.create","model":"gpt-5.6-sol"}`),
				RelayOptions{OnTurnComplete: func(turn RelayTurnResult) {
					turns = append(turns, turn)
				}},
			)

			require.Nil(t, relayExit)
			require.Len(t, turns, 1)
			require.Equal(t, eventType, turns[0].TerminalEventType)
			require.Equal(t, "resp-cancelled", turns[0].RequestID)
			require.Equal(t, 119404, turns[0].Usage.InputTokens)
			require.Equal(t, 119040, turns[0].Usage.CacheReadInputTokens)
			require.Equal(t, 301, turns[0].Usage.OutputTokens)
			require.Equal(t, turns[0].Usage, result.Usage)
		})
	}
}

func TestRelay_IDLessCancelledTerminalStillForwardsUsageToBillingCallback(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.cancelled","response":{"usage":{"input_tokens":8,"output_tokens":1}}}`)
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: payload},
	}, true)
	turns := make([]RelayTurnResult, 0, 1)

	result, relayExit := Relay(
		context.Background(),
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"gpt-5.6-sol"}`),
		RelayOptions{OnTurnComplete: func(turn RelayTurnResult) {
			turns = append(turns, turn)
		}},
	)

	require.Nil(t, relayExit)
	require.Len(t, turns, 1)
	require.Empty(t, turns[0].RequestID)
	require.Equal(t, "response.cancelled", turns[0].TerminalEventType)
	require.Equal(t, 8, turns[0].Usage.InputTokens)
	require.Equal(t, 1, turns[0].Usage.OutputTokens)
	require.Equal(t, turns[0].Usage, result.Usage)
}

func TestRelay_TopLevelErrorTerminalState(t *testing.T) {
	t.Parallel()

	t.Run("in flight finalizes once and closes", func(t *testing.T) {
		t.Parallel()

		errorPayload := []byte(`{"type":"error","id":"evt_error","error":{"type":"server_error","message":"failed"}}`)
		clientConn := newPassthroughTestFrameConn(nil, false)
		upstreamBase := newPassthroughTestFrameConn([]passthroughTestFrame{
			{msgType: coderws.MessageText, payload: errorPayload},
		}, true)
		upstreamConn := &closeTrackingFrameConn{FrameConn: upstreamBase}
		turns := make([]RelayTurnResult, 0, 1)

		result, relayExit := Relay(
			context.Background(),
			clientConn,
			upstreamConn,
			[]byte(`{"type":"response.create","model":"gpt-error"}`),
			RelayOptions{OnTurnComplete: func(turn RelayTurnResult) {
				turns = append(turns, turn)
			}},
		)

		require.Nil(t, relayExit)
		require.Equal(t, "error", result.TerminalEventType)
		require.Empty(t, result.RequestID, "top-level error id is an event id, not a response id")
		require.Len(t, turns, 1)
		require.Equal(t, "error", turns[0].TerminalEventType)
		require.Empty(t, turns[0].RequestID)
		require.Equal(t, "gpt-error", turns[0].RequestModel)
		require.Equal(t, errorPayload, clientConn.Writes()[0].payload)
		require.GreaterOrEqual(t, upstreamConn.closeCalls.Load(), int32(1))
	})

	t.Run("idle error is connection level only", func(t *testing.T) {
		t.Parallel()

		clientConn := newPassthroughTestFrameConn(nil, false)
		upstreamBase := newPassthroughTestFrameConn([]passthroughTestFrame{
			{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_done"}}`)},
			{msgType: coderws.MessageText, payload: []byte(`{"type":"response.completed","response":{"id":"resp_done","usage":{"input_tokens":2,"output_tokens":1}}}`)},
			{msgType: coderws.MessageText, payload: []byte(`{"type":"error","id":"evt_idle","error":{"type":"server_error"}}`)},
		}, true)
		upstreamConn := &closeTrackingFrameConn{FrameConn: upstreamBase}
		turns := make([]RelayTurnResult, 0, 1)
		beforeCalls := 0

		result, relayExit := Relay(
			context.Background(),
			clientConn,
			upstreamConn,
			[]byte(`{"type":"response.create","model":"gpt-idle"}`),
			RelayOptions{
				OnTurnComplete: func(turn RelayTurnResult) {
					turns = append(turns, turn)
				},
				BeforeWriteClient: func(_ coderws.MessageType, _ []byte, _ bool) error {
					beforeCalls++
					return nil
				},
			},
		)

		require.Nil(t, relayExit)
		require.Equal(t, "response.completed", result.TerminalEventType)
		require.Equal(t, "resp_done", result.RequestID)
		require.Equal(t, 2, result.Usage.InputTokens)
		require.Len(t, turns, 1, "idle connection error must not invent a new turn")
		require.Equal(t, "response.completed", turns[0].TerminalEventType)
		require.Equal(t, 2, beforeCalls, "idle connection error must bypass step failover hooks")
		require.Len(t, clientConn.Writes(), 3, "connection-level error is still forwarded")
		require.GreaterOrEqual(t, upstreamConn.closeCalls.Load(), int32(1))
	})
}

func TestRelay_OnTurnComplete_UsesGenerationRequestModel(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn(nil, false)
	transformed := make(chan struct{}, 2)
	feederDone := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sendTurn := func(responseID string, inputTokens int) bool {
		for _, frame := range []passthroughTestFrame{
			{
				msgType: coderws.MessageText,
				payload: []byte(`{"type":"response.created","response":{"id":"` + responseID + `"}}`),
			},
			{
				msgType: coderws.MessageText,
				payload: []byte(`{"type":"response.completed","response":{"id":"` + responseID + `","usage":{"input_tokens":` + strconv.Itoa(inputTokens) + `,"output_tokens":1}}}`),
			},
		} {
			select {
			case upstreamConn.readCh <- frame:
			case <-ctx.Done():
				return false
			}
		}
		return true
	}
	go func() {
		defer close(feederDone)
		defer func() { _ = upstreamConn.Close() }()
		if !sendTurn("resp_model_1", 1) {
			return
		}
		for _, turn := range []struct {
			responseID string
			tokens     int
		}{
			{responseID: "resp_model_2", tokens: 2},
			{responseID: "resp_model_3", tokens: 3},
		} {
			select {
			case <-transformed:
			case <-ctx.Done():
				return
			}
			if !sendTurn(turn.responseID, turn.tokens) {
				return
			}
		}
	}()

	turns := make([]RelayTurnResult, 0, 3)
	result, relayExit := Relay(
		ctx,
		clientConn,
		upstreamConn,
		[]byte(`{"type":"response.create","model":"model-a"}`),
		RelayOptions{
			StartClientAfterFirstDownstream: true,
			TransformResponseCreate: func(payload []byte, _ ClientEnvelope) ([]byte, error) {
				transformed <- struct{}{}
				return payload, nil
			},
			OnTurnComplete: func(turn RelayTurnResult) {
				turns = append(turns, turn)
				var nextPayload []byte
				switch len(turns) {
				case 1:
					nextPayload = []byte(`{"type":"response.create","model":"model-b"}`)
				case 2:
					nextPayload = []byte(`{"type":"response.create"}`)
				}
				if len(nextPayload) > 0 {
					select {
					case clientConn.readCh <- passthroughTestFrame{msgType: coderws.MessageText, payload: nextPayload}:
					case <-ctx.Done():
					}
				}
			},
		},
	)
	<-feederDone

	require.Nil(t, relayExit)
	require.Len(t, turns, 3)
	require.Equal(t, []string{"model-a", "model-b", "model-b"}, []string{
		turns[0].RequestModel,
		turns[1].RequestModel,
		turns[2].RequestModel,
	})
	require.Equal(t, []string{"resp_model_1", "resp_model_2", "resp_model_3"}, []string{
		turns[0].RequestID,
		turns[1].RequestID,
		turns[2].RequestID,
	})
	for _, turn := range turns {
		require.NotNil(t, turn.FirstByteMs)
		require.GreaterOrEqual(t, *turn.FirstByteMs, 0)
	}
	require.Equal(t, "model-b", result.RequestModel)
	require.Equal(t, 6, result.Usage.InputTokens)
	require.Len(t, upstreamConn.Writes(), 3)
}

func TestRelay_OnTurnComplete_ProvidesTurnMetrics(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_metric"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.output_text.delta","response_id":"resp_metric","delta":"hi"}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_metric","usage":{"input_tokens":2,"output_tokens":1}}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	base := time.Unix(0, 0)
	var nowTick atomic.Int64
	nowFn := func() time.Time {
		step := nowTick.Add(1)
		return base.Add(time.Duration(step) * 5 * time.Millisecond)
	}

	var turn RelayTurnResult
	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		Now: nowFn,
		OnTurnComplete: func(current RelayTurnResult) {
			turn = current
		},
	})
	require.Nil(t, relayExit)
	require.Equal(t, "resp_metric", turn.RequestID)
	require.Equal(t, "response.completed", turn.TerminalEventType)
	require.NotNil(t, turn.FirstTokenMs)
	require.GreaterOrEqual(t, *turn.FirstTokenMs, 0)
	require.NotNil(t, turn.FirstByteMs)
	require.GreaterOrEqual(t, *turn.FirstByteMs, 0)
	require.Greater(t, turn.Duration.Milliseconds(), int64(0))
	require.NotNil(t, result.FirstTokenMs)
	require.NotNil(t, result.FirstByteMs)
	require.Greater(t, result.Duration.Milliseconds(), int64(0))
}

func TestRelay_OnTurnComplete_FirstByteSurvivesClientWriteFailure(t *testing.T) {
	t.Parallel()

	clientConn := &errorOnWriteFrameConn{}
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.failed","response":{"id":"resp_write_failed"}}`),
		},
	}, true)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var callbacks int
	var turn RelayTurnResult
	result, relayExit := Relay(ctx, clientConn, upstreamConn, []byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[]}`), RelayOptions{
		OnTurnComplete: func(current RelayTurnResult) {
			callbacks++
			turn = current
		},
	})

	require.NotNil(t, relayExit)
	require.Equal(t, "write_client", relayExit.Stage)
	require.Equal(t, 1, callbacks)
	require.NotNil(t, turn.FirstByteMs)
	require.GreaterOrEqual(t, *turn.FirstByteMs, 0)
	require.NotNil(t, result.FirstByteMs)
	require.GreaterOrEqual(t, *result.FirstByteMs, 0)
}

func TestRelay_BinaryFramePassthrough(t *testing.T) {
	t.Parallel()

	binaryPayload := []byte{0x00, 0x01, 0x02, 0x03}
	createdPayload := []byte(`{"type":"response.created","response":{"id":"resp_binary"}}`)
	terminalPayload := []byte(`{"type":"response.completed","response":{"id":"resp_binary"}}`)
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageBinary,
			payload: binaryPayload,
		},
		{
			msgType: coderws.MessageText,
			payload: createdPayload,
		},
		{
			msgType: coderws.MessageText,
			payload: terminalPayload,
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var turn RelayTurnResult
	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		OnTurnComplete: func(current RelayTurnResult) { turn = current },
	})
	require.Nil(t, relayExit)
	require.Equal(t, 0, result.Usage.InputTokens)
	require.NotNil(t, turn.FirstByteMs)

	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 3)
	require.Equal(t, coderws.MessageBinary, clientWrites[0].msgType)
	require.Equal(t, binaryPayload, clientWrites[0].payload)
	require.Equal(t, createdPayload, clientWrites[1].payload)
	require.Equal(t, terminalPayload, clientWrites[2].payload)
}

func TestRelay_BinaryJSONFrameSkipsObservation(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageBinary,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_binary","usage":{"input_tokens":7,"output_tokens":3}}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	require.NotNil(t, relayExit)
	require.Equal(t, "read_upstream", relayExit.Stage)
	require.Equal(t, 0, result.Usage.InputTokens)
	require.Equal(t, "", result.RequestID)
	require.Equal(t, "", result.TerminalEventType)

	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 1)
	require.Equal(t, coderws.MessageBinary, clientWrites[0].msgType)
}

func TestRelay_UpstreamErrorEventPassthroughRaw(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	errorEvent := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"No tool call found"}}`)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: errorEvent,
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	require.Nil(t, relayExit)

	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 1)
	require.Equal(t, coderws.MessageText, clientWrites[0].msgType)
	require.Equal(t, errorEvent, clientWrites[0].payload)
}

func TestRelay_PreservesFirstMessageType(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn(nil, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		FirstMessageType: coderws.MessageBinary,
	})
	require.NotNil(t, relayExit)
	require.Equal(t, "read_upstream", relayExit.Stage)

	upstreamWrites := upstreamConn.Writes()
	require.Len(t, upstreamWrites, 1)
	require.Equal(t, coderws.MessageBinary, upstreamWrites[0].msgType)
	require.Equal(t, firstPayload, upstreamWrites[0].payload)
}

func TestRelay_UsageParseFailureDoesNotBlockRelay(t *testing.T) {
	baseline := SnapshotMetrics().UsageParseFailureTotal

	// 上游发送无效 JSON（非 usage 格式），不应影响透传
	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_bad"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_bad","usage":"not_an_object"}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	require.Nil(t, relayExit)
	// usage 解析失败，值为 0 但不影响透传
	require.Equal(t, 0, result.Usage.InputTokens)
	require.Equal(t, "response.completed", result.TerminalEventType)

	// 帧仍然被转发
	clientWrites := clientConn.Writes()
	require.Len(t, clientWrites, 2)
	require.GreaterOrEqual(t, SnapshotMetrics().UsageParseFailureTotal, baseline+1)
}

func TestRelay_WriteUpstreamFirstMessageFails(t *testing.T) {
	t.Parallel()

	// 上游连接立即关闭，首包写入失败
	upstreamConn := newPassthroughTestFrameConn(nil, true)
	_ = upstreamConn.Close()

	// 覆盖 WriteFrame 使其返回错误
	errConn := &errorOnWriteFrameConn{}
	clientConn := newPassthroughTestFrameConn(nil, false)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, relayExit := Relay(ctx, clientConn, errConn, firstPayload, RelayOptions{})
	require.NotNil(t, relayExit)
	require.Equal(t, "write_upstream", relayExit.Stage)
}

func TestRelay_ContextCanceled(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn(nil, false)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)

	// 立即取消 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{})
	// context 取消导致写首包失败
	require.NotNil(t, relayExit)
}

func TestRelay_TraceEvents_ContainsLifecycleStages(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_trace"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_trace","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stages := make([]string, 0, 8)
	var stagesMu sync.Mutex
	_, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		OnTrace: func(event RelayTraceEvent) {
			stagesMu.Lock()
			stages = append(stages, event.Stage)
			stagesMu.Unlock()
		},
	})
	require.Nil(t, relayExit)
	stagesMu.Lock()
	capturedStages := append([]string(nil), stages...)
	stagesMu.Unlock()
	require.Contains(t, capturedStages, "relay_start")
	require.Contains(t, capturedStages, "write_first_message_ok")
	require.Contains(t, capturedStages, "first_exit")
	require.Contains(t, capturedStages, "relay_complete")
}

func TestRelay_TraceEvents_IdleTimeout(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn(nil, false)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	callCount := 0
	nowFn := func() time.Time {
		callCount++
		if callCount <= 5 {
			return now
		}
		return now.Add(time.Hour)
	}

	stages := make([]string, 0, 8)
	var stagesMu sync.Mutex
	_, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		IdleTimeout: 2 * time.Second,
		Now:         nowFn,
		OnTrace: func(event RelayTraceEvent) {
			stagesMu.Lock()
			stages = append(stages, event.Stage)
			stagesMu.Unlock()
		},
	})
	require.NotNil(t, relayExit)
	require.Equal(t, "idle_timeout", relayExit.Stage)
	stagesMu.Lock()
	capturedStages := append([]string(nil), stages...)
	stagesMu.Unlock()
	require.Contains(t, capturedStages, "idle_timeout_triggered")
	require.Contains(t, capturedStages, "relay_exit")
}

// errorOnWriteFrameConn 是一个写入总是失败的 FrameConn 实现，用于测试首包写入失败。
type errorOnWriteFrameConn struct{}

func (c *errorOnWriteFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	<-ctx.Done()
	return coderws.MessageText, nil, ctx.Err()
}

func (c *errorOnWriteFrameConn) WriteFrame(_ context.Context, _ coderws.MessageType, _ []byte) error {
	return errors.New("write failed: connection refused")
}

func (c *errorOnWriteFrameConn) Close() error {
	return nil
}

func TestRelay_OnTurnComplete_RealOpenAIStream_FirstTokenMs(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_real"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.output_text.delta","delta":"He"}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.output_text.delta","delta":"llo"}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.output_text.delta","delta":" world"}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_real","usage":{"input_tokens":2,"output_tokens":3}}}`),
		},
	}, true)

	firstPayload := []byte(`{"type":"response.create","model":"gpt-5.3-codex","input":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	base := time.Unix(0, 0)
	var nowTick atomic.Int64
	nowFn := func() time.Time {
		step := nowTick.Add(1)
		return base.Add(time.Duration(step) * 10 * time.Millisecond)
	}

	var turn RelayTurnResult
	result, relayExit := Relay(ctx, clientConn, upstreamConn, firstPayload, RelayOptions{
		Now: nowFn,
		OnTurnComplete: func(current RelayTurnResult) {
			turn = current
		},
	})
	require.Nil(t, relayExit)
	require.Equal(t, "resp_real", turn.RequestID)
	require.Equal(t, "response.completed", turn.TerminalEventType)

	require.NotNil(t, turn.FirstTokenMs, "per-turn FirstTokenMs must be captured for real OpenAI streams")
	require.Greater(t, turn.Duration.Milliseconds(), int64(0))

	require.Less(t,
		int64(*turn.FirstTokenMs),
		turn.Duration.Milliseconds(),
		"per-turn FirstTokenMs (%dms) should be strictly less than Duration (%dms); "+
			"equality indicates the bug where first_token is mistakenly stamped on the terminal event",
		*turn.FirstTokenMs, turn.Duration.Milliseconds(),
	)

	require.NotNil(t, result.FirstTokenMs)
	require.Greater(t, *result.FirstTokenMs, 0)
}
