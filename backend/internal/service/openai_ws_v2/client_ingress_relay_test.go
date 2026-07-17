package openai_ws_v2

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type blockingUpstreamWriteConn struct {
	writeCalls       atomic.Int32
	observedDeadline atomic.Bool
	closed           chan struct{}
	closeOnce        sync.Once
}

type gatedClientFrameConn struct {
	base *passthroughTestFrameConn
	gate <-chan struct{}
	once sync.Once
}

func (c *gatedClientFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	var waitErr error
	c.once.Do(func() {
		select {
		case <-ctx.Done():
			waitErr = ctx.Err()
		case <-c.gate:
		}
	})
	if waitErr != nil {
		return coderws.MessageText, nil, waitErr
	}
	return c.base.ReadFrame(ctx)
}

func (c *gatedClientFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	return c.base.WriteFrame(ctx, msgType, payload)
}

func (c *gatedClientFrameConn) Close() error {
	return c.base.Close()
}

type scriptedUpstreamFrameConn struct {
	base    *passthroughTestFrameConn
	onWrite func(payload []byte)
}

type delayedCancellationReaderConn struct {
	started   chan struct{}
	exited    chan struct{}
	startOnce sync.Once
	exitOnce  sync.Once
	delay     time.Duration
}

func newDelayedCancellationReaderConn(delay time.Duration) *delayedCancellationReaderConn {
	return &delayedCancellationReaderConn{
		started: make(chan struct{}),
		exited:  make(chan struct{}),
		delay:   delay,
	}
}

func (c *delayedCancellationReaderConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	c.startOnce.Do(func() { close(c.started) })
	<-ctx.Done()
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	c.exitOnce.Do(func() { close(c.exited) })
	return coderws.MessageText, nil, ctx.Err()
}

func (c *delayedCancellationReaderConn) WriteFrame(context.Context, coderws.MessageType, []byte) error {
	return nil
}

func (c *delayedCancellationReaderConn) Close() error {
	return nil
}

func (c *scriptedUpstreamFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	return c.base.ReadFrame(ctx)
}

func (c *scriptedUpstreamFrameConn) WriteFrame(ctx context.Context, msgType coderws.MessageType, payload []byte) error {
	if err := c.base.WriteFrame(ctx, msgType, payload); err != nil {
		return err
	}
	if c.onWrite != nil {
		c.onWrite(payload)
	}
	return nil
}

func (c *scriptedUpstreamFrameConn) Close() error {
	return c.base.Close()
}

func newBlockingUpstreamWriteConn() *blockingUpstreamWriteConn {
	return &blockingUpstreamWriteConn{closed: make(chan struct{})}
}

func (c *blockingUpstreamWriteConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	select {
	case <-ctx.Done():
		return coderws.MessageText, nil, ctx.Err()
	case <-c.closed:
		return coderws.MessageText, nil, io.EOF
	}
}

func (c *blockingUpstreamWriteConn) WriteFrame(ctx context.Context, _ coderws.MessageType, _ []byte) error {
	c.writeCalls.Add(1)
	if _, ok := ctx.Deadline(); ok {
		c.observedDeadline.Store(true)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c *blockingUpstreamWriteConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func TestRelayRejectsDuplicateEnvelopeBeforeCallbacksOrWrite(t *testing.T) {
	t.Parallel()

	client := newPassthroughTestFrameConn([]passthroughTestFrame{{
		msgType: coderws.MessageText,
		payload: []byte(`{"type":"response.create","ty\u0070e":"response.create","model":"gpt-5"}`),
	}}, true)
	upstream := newPassthroughTestFrameConn(nil, false)
	var allFrameCalls atomic.Int32
	var admissionCalls atomic.Int32
	var transformCalls atomic.Int32

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, relayExit := Relay(ctx, client, upstream, []byte(`{"type":"response.create","model":"gpt-5"}`), RelayOptions{
		FirstMessageSent: true,
		BeforeWriteUpstreamFrame: func(coderws.MessageType, []byte, ClientEnvelope) error {
			allFrameCalls.Add(1)
			return nil
		},
		BeforeWriteResponseCreate: func(coderws.MessageType, []byte, string) error {
			admissionCalls.Add(1)
			return nil
		},
		TransformResponseCreate: func(payload []byte, _ ClientEnvelope) ([]byte, error) {
			transformCalls.Add(1)
			return payload, nil
		},
	})

	require.NotNil(t, relayExit)
	require.Equal(t, "write_upstream", relayExit.Stage)
	require.ErrorIs(t, relayExit.Err, errClientEnvelopeDuplicateField)
	require.Zero(t, allFrameCalls.Load())
	require.Zero(t, admissionCalls.Load())
	require.Zero(t, transformCalls.Load())
	require.Empty(t, upstream.Writes())
}

func TestRelayAppliesPreInspectLimitToSeventeenMiBNonResponseFrame(t *testing.T) {
	t.Parallel()

	const maxPayload = 16 * 1024 * 1024
	payload := make([]byte, maxPayload+1024*1024)
	client := newPassthroughTestFrameConn([]passthroughTestFrame{{
		msgType: coderws.MessageBinary,
		payload: payload,
	}}, true)
	upstream := newPassthroughTestFrameConn(nil, false)
	errTooLarge := errors.New("frame too large")
	var inspectedBytes atomic.Int64

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, relayExit := Relay(ctx, client, upstream, []byte(`{"type":"response.create","model":"gpt-5"}`), RelayOptions{
		FirstMessageSent: true,
		BeforeInspectUpstreamFrame: func(_ coderws.MessageType, frame []byte) error {
			inspectedBytes.Store(int64(len(frame)))
			if len(frame) > maxPayload {
				return errTooLarge
			}
			return nil
		},
	})

	require.NotNil(t, relayExit)
	require.Equal(t, "write_upstream", relayExit.Stage)
	require.ErrorIs(t, relayExit.Err, errTooLarge)
	require.Equal(t, int64(len(payload)), inspectedBytes.Load())
	require.Empty(t, upstream.Writes())
}

func TestRelayNonResponseWriteHonorsWriteTimeout(t *testing.T) {
	t.Parallel()

	client := newPassthroughTestFrameConn([]passthroughTestFrame{{
		msgType: coderws.MessageText,
		payload: []byte(`{"type":"response.cancel"}`),
	}}, true)
	upstream := newBlockingUpstreamWriteConn()

	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, relayExit := Relay(ctx, client, upstream, []byte(`{"type":"response.create","model":"gpt-5"}`), RelayOptions{
		FirstMessageSent: true,
		WriteTimeout:     25 * time.Millisecond,
	})

	require.NotNil(t, relayExit)
	require.Equal(t, "write_upstream", relayExit.Stage)
	require.ErrorIs(t, relayExit.Err, context.DeadlineExceeded)
	require.Equal(t, int32(1), upstream.writeCalls.Load())
	require.True(t, upstream.observedDeadline.Load())
	require.Less(t, time.Since(startedAt), 500*time.Millisecond)
}

func TestRelayFinalFenceRunsAfterTransformAndBeforeProviderWrite(t *testing.T) {
	t.Parallel()

	firstTurnDone := make(chan struct{})
	var firstTurnOnce sync.Once
	client := &gatedClientFrameConn{
		base: newPassthroughTestFrameConn([]passthroughTestFrame{{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.create","model":"gpt-5","input":[]}`),
		}}, true),
		gate: firstTurnDone,
	}
	upstreamBase := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_1"}}`)},
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":1,"output_tokens":1}}}`)},
	}, false)

	var orderMu sync.Mutex
	order := make([]string, 0, 4)
	var admissionOriginal string
	var fenceOriginal string
	var fencePayload []byte
	var writtenPayload []byte
	record := func(step string) {
		orderMu.Lock()
		order = append(order, step)
		orderMu.Unlock()
	}
	upstream := &scriptedUpstreamFrameConn{
		base: upstreamBase,
		onWrite: func(payload []byte) {
			record("write")
			orderMu.Lock()
			writtenPayload = append([]byte(nil), payload...)
			orderMu.Unlock()
			upstreamBase.readCh <- passthroughTestFrame{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_2"}}`)}
			upstreamBase.readCh <- passthroughTestFrame{msgType: coderws.MessageText, payload: []byte(`{"type":"response.completed","response":{"id":"resp_2","usage":{"input_tokens":1,"output_tokens":1}}}`)}
			upstreamBase.once.Do(func() { close(upstreamBase.readCh) })
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, relayExit := Relay(ctx, client, upstream, []byte(`{"type":"response.create","model":"gpt-5"}`), RelayOptions{
		FirstMessageSent: true,
		BeforeWriteResponseCreate: func(_ coderws.MessageType, _ []byte, originalModel string) error {
			record("admission")
			orderMu.Lock()
			admissionOriginal = originalModel
			orderMu.Unlock()
			return nil
		},
		TransformResponseCreate: func([]byte, ClientEnvelope) ([]byte, error) {
			record("transform")
			return []byte(`{"type":"response.create","model":"gpt-5","transformed":true}`), nil
		},
		BeforeDispatchResponseCreate: func(_ coderws.MessageType, payload []byte, originalModel string) error {
			record("final_fence")
			orderMu.Lock()
			fenceOriginal = originalModel
			fencePayload = append([]byte(nil), payload...)
			orderMu.Unlock()
			return nil
		},
		OnTurnComplete: func(turn RelayTurnResult) {
			if turn.RequestID == "resp_1" {
				firstTurnOnce.Do(func() { close(firstTurnDone) })
			}
		},
	})

	if relayExit != nil {
		require.Equal(t, "client_disconnected", relayExit.Stage)
	}
	orderMu.Lock()
	gotOrder := append([]string(nil), order...)
	gotAdmissionOriginal := admissionOriginal
	gotFenceOriginal := fenceOriginal
	gotFencePayload := append([]byte(nil), fencePayload...)
	gotWrittenPayload := append([]byte(nil), writtenPayload...)
	orderMu.Unlock()
	require.Equal(t, []string{"admission", "transform", "final_fence", "write"}, gotOrder)
	require.Equal(t, "gpt-5", gotAdmissionOriginal)
	require.Equal(t, "gpt-5", gotFenceOriginal)
	require.JSONEq(t, `{"type":"response.create","model":"gpt-5","transformed":true}`, string(gotFencePayload))
	require.JSONEq(t, `{"type":"response.create","model":"gpt-5","transformed":true}`, string(gotWrittenPayload))
}

func TestRelayWaitsForCancelledUpstreamReaderBeforeReturn(t *testing.T) {
	t.Parallel()

	const readerExitDelay = 75 * time.Millisecond
	client := newPassthroughTestFrameConn(nil, true)
	upstream := newDelayedCancellationReaderConn(readerExitDelay)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	startedAt := time.Now()
	_, relayExit := Relay(ctx, client, upstream, []byte(`{"type":"response.create","model":"gpt-5"}`), RelayOptions{
		FirstMessageSent:     true,
		UpstreamDrainTimeout: 5 * time.Millisecond,
	})

	require.NotNil(t, relayExit)
	require.Equal(t, "client_disconnected", relayExit.Stage)
	select {
	case <-upstream.started:
	default:
		t.Fatal("upstream reader did not start")
	}
	select {
	case <-upstream.exited:
	default:
		t.Fatal("relay returned before the cancelled upstream reader exited")
	}
	require.GreaterOrEqual(t, time.Since(startedAt), readerExitDelay)
}
