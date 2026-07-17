package openai_ws_v2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteDeadlineContextResetHotPathDoesNotAllocate(t *testing.T) {
	ctx := newWriteDeadlineContext(context.Background())
	t.Cleanup(ctx.Stop)

	allocations := testing.AllocsPerRun(1000, func() {
		if err := ctx.Reset(time.Hour); err != nil {
			t.Fatal(err)
		}
		ctx.Disarm()
	})
	require.Zero(t, allocations)
}

func TestWriteDeadlineContextDisarmAllowsLongRetainedIdle(t *testing.T) {
	ctx := newWriteDeadlineContext(context.Background())
	defer ctx.Stop()

	require.NoError(t, ctx.Reset(10*time.Millisecond))
	ctx.Disarm()
	time.Sleep(30 * time.Millisecond)
	require.NoError(t, ctx.Err())
	require.NoError(t, ctx.Reset(time.Second))
	ctx.Disarm()
}

func TestWriteDeadlineContextExpiresAndObservesParentCancellation(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		ctx := newWriteDeadlineContext(context.Background())
		defer ctx.Stop()
		require.NoError(t, ctx.Reset(10*time.Millisecond))
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("write deadline did not expire")
		}
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	})

	t.Run("parent", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		ctx := newWriteDeadlineContext(parent)
		defer ctx.Stop()
		require.NoError(t, ctx.Reset(time.Hour))
		cancel()
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("parent cancellation was not observed")
		}
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	})

	t.Run("parent already cancelled", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		cancel()
		ctx := newWriteDeadlineContext(parent)
		defer ctx.Stop()
		require.ErrorIs(t, ctx.Reset(time.Hour), context.Canceled)
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	})
}
