package handler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newOpenAIWSTurnFinalizerTestPool(t *testing.T) *service.UsageRecordWorkerPool {
	t.Helper()
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        2,
		TaskTimeout:      time.Second,
		OverflowPolicy:   "drop",
		AutoScaleEnabled: false,
	})
	t.Cleanup(pool.Stop)
	return pool
}

func TestOpenAIWSTurnFinalizerCommitDoesNotWaitForReleaseOrUsage(t *testing.T) {
	pool := newOpenAIWSTurnFinalizerTestPool(t)
	finalizer := newOpenAIWSTurnFinalizer()
	releaseGate := make(chan struct{})
	usageGate := make(chan struct{})
	usageStarted := make(chan struct{})
	var releases atomic.Int32

	require.NoError(t, finalizer.InstallUserRelease(func() {
		<-releaseGate
		releases.Add(1)
	}))
	require.NoError(t, finalizer.InstallAccountRelease(func() {
		<-releaseGate
		releases.Add(1)
	}))
	require.NoError(t, finalizer.Reserve(context.Background(), pool))

	started := time.Now()
	require.True(t, finalizer.Commit(func(context.Context) {
		close(usageStarted)
		<-usageGate
	}))
	require.Less(t, time.Since(started), 50*time.Millisecond)

	waitResult := make(chan error, 1)
	go func() { waitResult <- finalizer.WaitPreviousRelease(context.Background()) }()
	select {
	case <-waitResult:
		t.Fatal("next turn must wait until both concurrency releases finish")
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseGate)
	require.NoError(t, <-waitResult)
	require.Equal(t, int32(2), releases.Load())
	select {
	case <-usageStarted:
	case <-time.After(time.Second):
		t.Fatal("usage task did not start after release phase")
	}

	// The next turn release barrier is independent of the slower DB task.
	require.NoError(t, finalizer.WaitPreviousRelease(context.Background()))
	close(usageGate)
	require.Eventually(t, func() bool {
		return pool.Stats().RequiredOutstanding == 0
	}, time.Second, time.Millisecond)
}

func TestOpenAIWSTurnFinalizerReleaseBypassesSaturatedOptionalUsageQueue(t *testing.T) {
	pool := service.NewUsageRecordWorkerPoolWithOptions(service.UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        16,
		TaskTimeout:      time.Second,
		OverflowPolicy:   "drop",
		AutoScaleEnabled: false,
	})
	t.Cleanup(pool.Stop)

	optionalGate := make(chan struct{})
	var optionalGateClosed atomic.Bool
	closeOptionalGate := func() {
		if optionalGateClosed.CompareAndSwap(false, true) {
			close(optionalGate)
		}
	}
	t.Cleanup(closeOptionalGate)
	optionalStarted := make(chan struct{})
	require.Equal(t, service.UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) {
		close(optionalStarted)
		<-optionalGate
	}))
	<-optionalStarted
	for i := 0; i < 16; i++ {
		require.Equal(t, service.UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) {
			<-optionalGate
		}))
	}
	require.Equal(t, service.UsageRecordSubmitModeDropped, pool.Submit(func(context.Context) {}))

	finalizer := newOpenAIWSTurnFinalizer()
	released := make(chan struct{})
	usageStarted := make(chan struct{})
	require.NoError(t, finalizer.InstallAccountRelease(func() { close(released) }))
	require.NoError(t, finalizer.Reserve(context.Background(), pool))

	commitStarted := time.Now()
	require.True(t, finalizer.Commit(func(context.Context) { close(usageStarted) }))
	require.Less(t, time.Since(commitStarted), 10*time.Millisecond)

	waitCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	require.NoError(t, finalizer.WaitPreviousRelease(waitCtx), "next turn barrier waited for optional usage work")
	select {
	case <-released:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("account release waited for optional usage work")
	}
	select {
	case <-usageStarted:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("required persistence waited for optional usage work")
	}

	closeOptionalGate()
}

func TestOpenAIWSTurnFinalizerReserveIsIdempotentAcrossInitialFailover(t *testing.T) {
	pool := newOpenAIWSTurnFinalizerTestPool(t)
	finalizer := newOpenAIWSTurnFinalizer()
	require.NoError(t, finalizer.Reserve(context.Background(), pool))
	require.NoError(t, finalizer.Reserve(context.Background(), pool))
	require.Equal(t, 1, pool.Stats().RequiredOutstanding)
	finalizer.AbortCurrent()
	require.Zero(t, pool.Stats().RequiredOutstanding)
}

func TestOpenAIWSTurnFinalizerInitialFailoverRetainsUserAndReservation(t *testing.T) {
	pool := newOpenAIWSTurnFinalizerTestPool(t)
	finalizer := newOpenAIWSTurnFinalizer()
	var userReleases atomic.Int32
	var firstAccountReleases atomic.Int32
	var secondAccountReleases atomic.Int32

	require.NoError(t, finalizer.Reserve(context.Background(), pool))
	require.NoError(t, finalizer.InstallUserRelease(func() { userReleases.Add(1) }))
	require.NoError(t, finalizer.InstallAccountRelease(func() { firstAccountReleases.Add(1) }))

	finalizer.ReleaseAccountNow()
	require.Equal(t, int32(1), firstAccountReleases.Load())
	require.Zero(t, userReleases.Load())
	require.True(t, finalizer.HasUserRelease())
	require.Equal(t, 1, pool.Stats().RequiredOutstanding)

	// The outer retry calls Reserve again, but it must reuse the original slot.
	require.NoError(t, finalizer.Reserve(context.Background(), pool))
	require.Equal(t, 1, pool.Stats().RequiredOutstanding)
	require.NoError(t, finalizer.InstallAccountRelease(func() { secondAccountReleases.Add(1) }))

	finalizer.AbortCurrent()
	require.Equal(t, int32(1), userReleases.Load())
	require.Equal(t, int32(1), secondAccountReleases.Load())
	require.Zero(t, pool.Stats().RequiredOutstanding)
}

func TestOpenAIWSTurnFinalizerHasActiveTurnStopsAtTerminalOwnershipTransfer(t *testing.T) {
	pool := newOpenAIWSTurnFinalizerTestPool(t)
	finalizer := newOpenAIWSTurnFinalizer()
	released := make(chan struct{})
	require.NoError(t, finalizer.InstallUserRelease(func() { close(released) }))
	require.NoError(t, finalizer.Reserve(context.Background(), pool))
	require.True(t, finalizer.HasActiveTurn())

	require.True(t, finalizer.Commit(nil))
	require.False(t, finalizer.HasActiveTurn(), "terminal commit transfers ownership to the worker")
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("release task did not run")
	}
}

func TestOpenAIWSTurnFinalizerAbortReleasesEveryResourceOnce(t *testing.T) {
	pool := newOpenAIWSTurnFinalizerTestPool(t)
	finalizer := newOpenAIWSTurnFinalizer()
	var userReleases atomic.Int32
	var accountReleases atomic.Int32
	require.NoError(t, finalizer.InstallUserRelease(func() { userReleases.Add(1) }))
	require.NoError(t, finalizer.InstallAccountRelease(func() { accountReleases.Add(1) }))
	require.NoError(t, finalizer.Reserve(context.Background(), pool))

	finalizer.AbortCurrent()
	finalizer.AbortCurrent()
	require.Equal(t, int32(1), userReleases.Load())
	require.Equal(t, int32(1), accountReleases.Load())
	require.Zero(t, pool.Stats().RequiredOutstanding)
}
