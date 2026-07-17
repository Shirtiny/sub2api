package service

import (
	"context"
	"errors"
	"io"
	"sync"

	coderws "github.com/coder/websocket"
)

var errOpenAIWSClientFrameReaderUnavailable = errors.New("openai websocket client frame reader is unavailable")

type openAIWSClientFrameRead struct {
	messageType coderws.MessageType
	payload     []byte
	err         error
}

// OpenAIWSClientFrameReader owns the single physical read loop for one retained
// client connection. Per-upstream-attempt cancellation only stops the consumer;
// it must not cancel coder/websocket.Read and close a connection that can still
// be reused for proven-not-executed account failover.
type OpenAIWSClientFrameReader struct {
	frames    <-chan openAIWSClientFrameRead
	startOnce sync.Once
	start     func()
}

func NewOpenAIWSClientFrameReader(lifetime context.Context, conn *coderws.Conn) *OpenAIWSClientFrameReader {
	if conn == nil {
		return newOpenAIWSClientFrameReader(lifetime, nil)
	}
	return newOpenAIWSClientFrameReader(lifetime, conn.Read)
}

func newOpenAIWSClientFrameReader(
	lifetime context.Context,
	readFrame func(context.Context) (coderws.MessageType, []byte, error),
) *OpenAIWSClientFrameReader {
	if lifetime == nil {
		lifetime = context.Background()
	}
	frames := make(chan openAIWSClientFrameRead)
	reader := &OpenAIWSClientFrameReader{frames: frames}
	if readFrame == nil {
		close(frames)
		return reader
	}
	reader.start = func() {
		go runOpenAIWSClientFrameReader(lifetime, readFrame, frames)
	}
	return reader
}

func runOpenAIWSClientFrameReader(
	lifetime context.Context,
	readFrame func(context.Context) (coderws.MessageType, []byte, error),
	frames chan<- openAIWSClientFrameRead,
) {
	defer close(frames)
	for {
		messageType, payload, err := readFrame(lifetime)
		frame := openAIWSClientFrameRead{messageType: messageType, payload: payload, err: err}
		select {
		case frames <- frame:
		case <-lifetime.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

func (r *OpenAIWSClientFrameReader) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	if r == nil || r.frames == nil {
		return coderws.MessageText, nil, errOpenAIWSClientFrameReaderUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.startOnce.Do(func() {
		if r.start != nil {
			r.start()
		}
	})
	select {
	case frame, ok := <-r.frames:
		if !ok {
			return coderws.MessageText, nil, io.EOF
		}
		return frame.messageType, frame.payload, frame.err
	case <-ctx.Done():
		return coderws.MessageText, nil, ctx.Err()
	}
}
