package service

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSClientFrameReader_AttemptCancellationDoesNotCancelPhysicalRead(t *testing.T) {
	lifetime, cancelLifetime := context.WithCancel(context.Background())
	defer cancelLifetime()

	physicalFrames := make(chan openAIWSClientFrameRead)
	physicalReadStarted := make(chan struct{})
	var readCalls atomic.Int32
	reader := newOpenAIWSClientFrameReader(
		lifetime,
		func(ctx context.Context) (coderws.MessageType, []byte, error) {
			if readCalls.Add(1) == 1 {
				close(physicalReadStarted)
			}
			select {
			case frame := <-physicalFrames:
				return frame.messageType, frame.payload, frame.err
			case <-ctx.Done():
				return coderws.MessageText, nil, ctx.Err()
			}
		},
	)

	attempt, cancelAttempt := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, _, err := reader.ReadFrame(attempt)
		firstResult <- err
	}()
	select {
	case <-physicalReadStarted:
	case <-time.After(time.Second):
		t.Fatal("physical read did not start")
	}
	cancelAttempt()
	require.ErrorIs(t, <-firstResult, context.Canceled)
	require.Equal(t, int32(1), readCalls.Load(), "attempt cancellation must not restart or cancel the physical read")

	nextPayload := []byte(`{"type":"response.create","model":"gpt-5","input":"next"}`)
	physicalFrames <- openAIWSClientFrameRead{messageType: coderws.MessageText, payload: nextPayload}
	readCtx, cancelRead := context.WithTimeout(context.Background(), time.Second)
	defer cancelRead()
	messageType, payload, err := reader.ReadFrame(readCtx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, messageType)
	require.Equal(t, nextPayload, payload)

	physicalFrames <- openAIWSClientFrameRead{messageType: coderws.MessageText, err: io.EOF}
	_, _, err = reader.ReadFrame(readCtx)
	require.ErrorIs(t, err, io.EOF)
}

func TestOpenAIWSClientFrameReader_DoesNotPrefetchBeforeFirstConsumer(t *testing.T) {
	readStarted := make(chan struct{})
	frames := make(chan openAIWSClientFrameRead, 1)
	var readCalls atomic.Int32
	reader := newOpenAIWSClientFrameReader(context.Background(), func(context.Context) (coderws.MessageType, []byte, error) {
		if readCalls.Add(1) == 1 {
			close(readStarted)
		} else {
			return coderws.MessageText, nil, io.EOF
		}
		frame := <-frames
		return frame.messageType, frame.payload, frame.err
	})

	select {
	case <-readStarted:
		t.Fatal("physical websocket read started during cold admission")
	case <-time.After(20 * time.Millisecond):
	}

	frames <- openAIWSClientFrameRead{messageType: coderws.MessageText, payload: []byte(`{"type":"session.update"}`)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := reader.ReadFrame(ctx)
	require.NoError(t, err)
	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("physical websocket read did not start for the first consumer")
	}
}
