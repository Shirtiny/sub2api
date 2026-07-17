package openai_ws_v2

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const (
	writeDeadlineActive uint32 = iota
	writeDeadlineExceeded
	writeDeadlineCancelled
)

// writeDeadlineContext is single-writer and reusable. It keeps high-frequency
// relay writes bounded without allocating a context and timer for every delta.
type writeDeadlineContext struct {
	parent context.Context
	done   chan struct{}
	state  atomic.Uint32

	deadlineNanos atomic.Int64
	closeOnce     sync.Once
	mu            sync.Mutex
	timer         *time.Timer
	armed         bool
	stopParent    func() bool
}

func newWriteDeadlineContext(parent context.Context) *writeDeadlineContext {
	if parent == nil {
		parent = context.Background()
	}
	ctx := &writeDeadlineContext{
		parent: parent,
		done:   make(chan struct{}),
	}
	ctx.stopParent = context.AfterFunc(parent, func() {
		ctx.finish(writeDeadlineCancelled)
	})
	return ctx
}

func (c *writeDeadlineContext) Reset(timeout time.Duration) error {
	if c == nil {
		return context.Canceled
	}
	if err := c.parent.Err(); err != nil {
		c.finish(writeDeadlineCancelled)
		return err
	}
	if err := c.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.Err(); err != nil {
		return err
	}
	c.deadlineNanos.Store(deadline.UnixNano())
	c.armed = true
	if c.timer == nil {
		c.timer = time.AfterFunc(timeout, c.expire)
	} else {
		c.timer.Reset(timeout)
	}
	return nil
}

func (c *writeDeadlineContext) expire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Load() != writeDeadlineActive || !c.armed {
		return
	}
	deadlineNanos := c.deadlineNanos.Load()
	remaining := time.Until(time.Unix(0, deadlineNanos))
	if remaining > 0 {
		c.timer.Reset(remaining)
		return
	}
	c.armed = false
	c.finish(writeDeadlineExceeded)
}

// Disarm stops the current write deadline without cancelling the reusable
// context. A retained connection may be idle much longer than WriteTimeout;
// only time spent inside an actual socket write is bounded by that timeout.
func (c *writeDeadlineContext) Disarm() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.armed = false
	c.deadlineNanos.Store(0)
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
}

func (c *writeDeadlineContext) finish(state uint32) {
	c.closeOnce.Do(func() {
		c.state.Store(state)
		close(c.done)
	})
}

func (c *writeDeadlineContext) Stop() {
	if c == nil {
		return
	}
	if c.stopParent != nil {
		c.stopParent()
	}
	c.mu.Lock()
	c.armed = false
	c.deadlineNanos.Store(0)
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
}

func (c *writeDeadlineContext) Deadline() (time.Time, bool) {
	if c == nil {
		return time.Time{}, false
	}
	deadlineNanos := c.deadlineNanos.Load()
	if deadlineNanos <= 0 {
		return time.Time{}, false
	}
	return time.Unix(0, deadlineNanos), true
}

func (c *writeDeadlineContext) Done() <-chan struct{} {
	if c == nil {
		return nil
	}
	return c.done
}

func (c *writeDeadlineContext) Err() error {
	if c == nil {
		return context.Canceled
	}
	switch c.state.Load() {
	case writeDeadlineExceeded:
		return context.DeadlineExceeded
	case writeDeadlineCancelled:
		if err := c.parent.Err(); err != nil {
			return err
		}
		return context.Canceled
	default:
		return nil
	}
}

func (c *writeDeadlineContext) Value(key any) any {
	if c == nil || c.parent == nil {
		return nil
	}
	return c.parent.Value(key)
}
