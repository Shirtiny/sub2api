package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestUsageRecordWorkerPool_SubmitEnqueued(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             8,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicyDrop,
		OverflowSamplePercent: 0,
	})
	t.Cleanup(pool.Stop)

	done := make(chan struct{})
	mode := pool.Submit(func(ctx context.Context) {
		close(done)
	})
	require.Equal(t, UsageRecordSubmitModeEnqueued, mode)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("task not executed")
	}

	require.Eventually(t, func() bool {
		stats := pool.Stats()
		return stats.SubmittedTasks == 1 && stats.SuccessfulTasks == 1
	}, time.Second, 10*time.Millisecond)
}

func TestUsageRecordWorkerPool_OverflowDrop(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicyDrop,
		OverflowSamplePercent: 0,
	})
	t.Cleanup(pool.Stop)

	block := make(chan struct{})
	started := make(chan struct{})
	secondDone := make(chan struct{})

	require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
		close(started)
		<-block
	}))
	<-started

	require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
		close(secondDone)
	}))
	require.Equal(t, UsageRecordSubmitModeDropped, pool.Submit(func(ctx context.Context) {}))

	close(block)
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("queued task not executed")
	}

	require.Eventually(t, func() bool {
		return pool.Stats().DroppedQueueFull >= 1
	}, time.Second, 10*time.Millisecond)
}

func TestUsageRecordWorkerPool_OverflowSync(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicySync,
		OverflowSamplePercent: 0,
	})
	t.Cleanup(pool.Stop)

	block := make(chan struct{})
	started := make(chan struct{})
	secondDone := make(chan struct{})
	var syncExecuted atomic.Bool

	require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
		close(started)
		<-block
	}))
	<-started

	require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
		close(secondDone)
	}))

	mode := pool.Submit(func(ctx context.Context) {
		syncExecuted.Store(true)
	})
	require.Equal(t, UsageRecordSubmitModeSync, mode)
	require.True(t, syncExecuted.Load())

	close(block)
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("queued task not executed")
	}

	require.Eventually(t, func() bool {
		return pool.Stats().SyncFallbackTasks >= 1
	}, time.Second, 10*time.Millisecond)
}

func TestUsageRecordWorkerPool_OverflowSample(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicySample,
		OverflowSamplePercent: 1,
	})
	t.Cleanup(pool.Stop)

	block := make(chan struct{})
	started := make(chan struct{})
	secondDone := make(chan struct{})
	var syncExecuted atomic.Bool

	require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
		close(started)
		<-block
	}))
	<-started

	require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
		close(secondDone)
	}))

	firstOverflow := pool.Submit(func(ctx context.Context) {
		syncExecuted.Store(true)
	})
	require.Equal(t, UsageRecordSubmitModeSync, firstOverflow)
	require.True(t, syncExecuted.Load())

	secondOverflow := pool.Submit(func(ctx context.Context) {})
	require.Equal(t, UsageRecordSubmitModeDropped, secondOverflow)

	close(block)
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("queued task not executed")
	}

	require.Eventually(t, func() bool {
		stats := pool.Stats()
		return stats.SyncFallbackTasks >= 1 && stats.DroppedQueueFull >= 1
	}, time.Second, 10*time.Millisecond)
}

func TestUsageRecordWorkerPool_SubmitAfterStop(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicyDrop,
		OverflowSamplePercent: 0,
	})

	pool.Stop()
	mode := pool.Submit(func(ctx context.Context) {})
	require.Equal(t, UsageRecordSubmitModeDropped, mode)
	require.GreaterOrEqual(t, pool.Stats().DroppedPoolStopped, uint64(1))
}

func TestUsageRecordWorkerPool_AutoScaleUpAndDown(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           2,
		QueueSize:             8,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicyDrop,
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      true,
		AutoScaleMinWorkers:   1,
		AutoScaleMaxWorkers:   4,
		AutoScaleUpPercent:    40,
		AutoScaleDownPercent:  10,
		AutoScaleUpStep:       1,
		AutoScaleDownStep:     1,
		AutoScaleInterval:     20 * time.Millisecond,
		AutoScaleCooldown:     20 * time.Millisecond,
	})
	t.Cleanup(pool.Stop)

	block := make(chan struct{})

	// 填满运行槽位 + 队列，触发扩容阈值。
	for i := 0; i < 8; i++ {
		require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
			<-block
		}))
	}

	require.Eventually(t, func() bool {
		return pool.Stats().MaxConcurrency >= 3
	}, 2*time.Second, 20*time.Millisecond)

	close(block)

	require.Eventually(t, func() bool {
		return pool.Stats().CompletedTasks >= 8
	}, 2*time.Second, 20*time.Millisecond)

	require.Eventually(t, func() bool {
		return pool.Stats().MaxConcurrency == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestUsageRecordWorkerPool_AutoScaleDownRequiresLowRunningUtilization(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           2,
		QueueSize:             8,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicyDrop,
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      true,
		AutoScaleMinWorkers:   1,
		AutoScaleMaxWorkers:   2,
		AutoScaleUpPercent:    80,
		AutoScaleDownPercent:  50,
		AutoScaleUpStep:       1,
		AutoScaleDownStep:     1,
		AutoScaleInterval:     20 * time.Millisecond,
		AutoScaleCooldown:     20 * time.Millisecond,
	})
	t.Cleanup(pool.Stop)

	block := make(chan struct{})
	for i := 0; i < 2; i++ {
		require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
			<-block
		}))
	}

	// 虽然 waiting=0，但 running 利用率为 100%，不应缩容。
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, 2, pool.Stats().MaxConcurrency)

	close(block)
	require.Eventually(t, func() bool {
		return pool.Stats().MaxConcurrency == 1
	}, 2*time.Second, 20*time.Millisecond)
}

func TestUsageRecordWorkerPool_SubmitNilReceiverAndNilTask(t *testing.T) {
	var nilPool *UsageRecordWorkerPool
	require.Equal(t, UsageRecordSubmitModeDropped, nilPool.Submit(func(ctx context.Context) {}))

	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicyDrop,
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)

	require.Equal(t, UsageRecordSubmitModeDropped, pool.Submit(nil))
}

func TestUsageRecordWorkerPool_SubmitRequiredBackpressuresWithoutDropping(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           1,
		QueueSize:             1,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicyDrop,
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
	})
	t.Cleanup(pool.Stop)

	first, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)
	second, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, pool.Stats().RequiredCapacity)

	result := make(chan UsageRecordSubmitMode, 1)
	go func() {
		result <- pool.SubmitRequired(func(context.Context) {})
	}()
	select {
	case <-result:
		t.Fatal("required submit must wait while the bounded queue is full")
	case <-time.After(20 * time.Millisecond):
	}
	require.True(t, first.Abort())
	require.Equal(t, UsageRecordSubmitModeEnqueued, <-result)
	require.True(t, second.Abort())
	require.Zero(t, pool.Stats().DroppedQueueFull)
}

func TestUsageRecordWorkerPool_RequiredReservationFailsWithinBoundWhenFull(t *testing.T) {
	const reserveTimeout = 25 * time.Millisecond
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:            1,
		QueueSize:              1,
		TaskTimeout:            time.Second,
		RequiredReserveTimeout: reserveTimeout,
		OverflowPolicy:         config.UsageRecordOverflowPolicyDrop,
		AutoScaleEnabled:       false,
	})
	t.Cleanup(pool.Stop)

	first, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)
	second, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)
	defer first.Abort()
	defer second.Abort()

	started := time.Now()
	_, err = pool.ReserveRequired(context.Background())
	require.ErrorIs(t, err, ErrUsageRecordRequiredCapacity)
	require.GreaterOrEqual(t, time.Since(started), reserveTimeout-5*time.Millisecond)
	require.Less(t, time.Since(started), 250*time.Millisecond)
	stats := pool.Stats()
	require.Equal(t, uint64(1), stats.RequiredReserveWaits)
	require.Equal(t, uint64(1), stats.RequiredReserveTimeouts)
	require.GreaterOrEqual(t, stats.RequiredReserveWaitNanos, uint64(reserveTimeout-5*time.Millisecond))
}

func TestUsageRecordReservationCommitAndAbortAreIdempotent(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		TaskTimeout:      time.Second,
		OverflowPolicy:   config.UsageRecordOverflowPolicyDrop,
		AutoScaleEnabled: false,
	})
	t.Cleanup(pool.Stop)

	done := make(chan struct{})
	reservation, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)
	require.True(t, reservation.Commit(func(context.Context) { close(done) }))
	require.False(t, reservation.Commit(func(context.Context) {}))
	require.False(t, reservation.Abort())
	require.Eventually(t, func() bool { return pool.Stats().RequiredOutstanding == 0 }, time.Second, time.Millisecond)
	<-done

	aborted, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)
	require.True(t, aborted.Abort())
	require.False(t, aborted.Abort())
	require.False(t, aborted.Commit(func(context.Context) {}))
}

func TestUsageRecordReservationRequiredPipelineIsolatedFromOptionalQueue(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		TaskTimeout:      time.Second,
		OverflowPolicy:   config.UsageRecordOverflowPolicyDrop,
		AutoScaleEnabled: false,
	})
	t.Cleanup(pool.Stop)

	running := make(chan struct{})
	release := make(chan struct{})
	require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) {
		close(running)
		<-release
	}))
	<-running
	require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) {}))

	reservation, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)
	requiredDone := make(chan struct{})
	started := time.Now()
	require.True(t, reservation.Commit(func(context.Context) { close(requiredDone) }))
	require.Less(t, time.Since(started), 10*time.Millisecond)
	select {
	case <-requiredDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("required persistence waited for the saturated optional queue")
	}
	require.Eventually(t, func() bool { return pool.Stats().RequiredOutstanding == 0 }, time.Second, time.Millisecond)
	close(release)
}

func TestUsageRecordReservationFinalizersRunWhileBothPersistenceQueuesAreBlocked(t *testing.T) {
	const taskCount = 32
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        taskCount,
		TaskTimeout:      time.Second,
		OverflowPolicy:   config.UsageRecordOverflowPolicyDrop,
		AutoScaleEnabled: false,
	})
	t.Cleanup(pool.Stop)

	optionalGate := make(chan struct{})
	var optionalGateOnce atomic.Bool
	closeOptionalGate := func() {
		if optionalGateOnce.CompareAndSwap(false, true) {
			close(optionalGate)
		}
	}
	t.Cleanup(closeOptionalGate)
	optionalStarted := make(chan struct{})
	require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) {
		close(optionalStarted)
		<-optionalGate
	}))
	<-optionalStarted
	for i := 0; i < taskCount; i++ {
		require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(context.Context) {
			<-optionalGate
		}))
	}
	require.Equal(t, UsageRecordSubmitModeDropped, pool.Submit(func(context.Context) {}), "optional queue must be saturated")

	persistenceGate := make(chan struct{})
	var persistenceGateOnce atomic.Bool
	closePersistenceGate := func() {
		if persistenceGateOnce.CompareAndSwap(false, true) {
			close(persistenceGate)
		}
	}
	t.Cleanup(closePersistenceGate)

	reservations := make([]*UsageRecordReservation, 0, taskCount)
	for i := 0; i < taskCount; i++ {
		reservation, err := pool.ReserveRequired(context.Background())
		require.NoError(t, err)
		reservations = append(reservations, reservation)
	}

	allFinalized := make(chan struct{})
	persistenceStarted := make(chan struct{})
	var finalized atomic.Int32
	var startedPersistence atomic.Bool
	for _, reservation := range reservations {
		commitStarted := time.Now()
		require.True(t, reservation.CommitFinalizer(func() {
			if finalized.Add(1) == taskCount {
				close(allFinalized)
			}
		}, func(context.Context) {
			if startedPersistence.CompareAndSwap(false, true) {
				close(persistenceStarted)
			}
			<-persistenceGate
		}))
		require.Less(t, time.Since(commitStarted), 10*time.Millisecond)
	}

	select {
	case <-persistenceStarted:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("required persistence worker did not start")
	}
	select {
	case <-allFinalized:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("required finalizers waited for optional or required persistence")
	}
	require.Equal(t, int32(taskCount), finalized.Load())
	require.Equal(t, taskCount, pool.Stats().RequiredOutstanding)

	closePersistenceGate()
	require.Eventually(t, func() bool { return pool.Stats().RequiredOutstanding == 0 }, 2*time.Second, time.Millisecond)
	closeOptionalGate()
}

func TestUsageRecordWorkerPoolStopDrainsRequiredFinalizerAndPersistence(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		TaskTimeout:      time.Second,
		OverflowPolicy:   config.UsageRecordOverflowPolicyDrop,
		AutoScaleEnabled: false,
	})

	reservation, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)
	finalized := make(chan struct{})
	persistenceStarted := make(chan struct{})
	persistenceGate := make(chan struct{})
	persistenceDone := make(chan struct{})
	require.True(t, reservation.CommitFinalizer(func() {
		close(finalized)
	}, func(context.Context) {
		close(persistenceStarted)
		<-persistenceGate
		close(persistenceDone)
	}))

	stopDone := make(chan struct{})
	go func() {
		pool.Stop()
		close(stopDone)
	}()
	select {
	case <-finalized:
	case <-time.After(time.Second):
		t.Fatal("required finalizer was not drained during stop")
	}
	select {
	case <-persistenceStarted:
	case <-time.After(time.Second):
		t.Fatal("required persistence was not started during stop")
	}
	select {
	case <-stopDone:
		t.Fatal("stop returned before required persistence completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(persistenceGate)
	select {
	case <-persistenceDone:
	case <-time.After(time.Second):
		t.Fatal("required persistence was dropped during stop")
	}
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("stop did not finish after required persistence completed")
	}

	_, err = pool.ReserveRequired(context.Background())
	require.ErrorIs(t, err, ErrUsageRecordWorkerPoolStopped)
}

func TestUsageRecordWorkerPoolStopContextTimesOutWithoutClosingRequiredQueues(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		TaskTimeout:      time.Second,
		OverflowPolicy:   config.UsageRecordOverflowPolicyDrop,
		AutoScaleEnabled: false,
	})

	reservation, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		reservation.Abort()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = pool.StopContext(ctx)
	})

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 40*time.Millisecond)
	started := time.Now()
	err = pool.StopContext(stopCtx)
	cancelStop()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 500*time.Millisecond)

	_, err = pool.ReserveRequired(context.Background())
	require.ErrorIs(t, err, ErrUsageRecordWorkerPoolStopped)
	require.Equal(t, UsageRecordSubmitModeDropped, pool.Submit(func(context.Context) {}))

	persisted := make(chan struct{})
	require.NotPanics(t, func() {
		require.True(t, reservation.Commit(func(context.Context) {
			close(persisted)
		}))
	})
	select {
	case <-persisted:
	case <-time.After(time.Second):
		t.Fatal("reservation committed after stop timeout was not persisted")
	}

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), time.Second)
	defer cancelDrain()
	require.NoError(t, pool.StopContext(drainCtx))
}

func TestUsageRecordWorkerPoolRequiredTaskSharesDeadlineAcrossDetachedBillingContexts(t *testing.T) {
	const taskTimeout = 120 * time.Millisecond
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        1,
		TaskTimeout:      taskTimeout,
		OverflowPolicy:   config.UsageRecordOverflowPolicyDrop,
		AutoScaleEnabled: false,
	})
	t.Cleanup(pool.Stop)

	reservation, err := pool.ReserveRequired(context.Background())
	require.NoError(t, err)

	deadlines := make(chan time.Time, 3)
	nestedErr := make(chan error, 1)
	started := time.Now()
	require.True(t, reservation.Commit(func(workerCtx context.Context) {
		for attempt := 0; attempt < 3; attempt++ {
			billingCtx, cancel := detachedBillingContext(workerCtx)
			deadline, ok := billingCtx.Deadline()
			if !ok {
				cancel()
				nestedErr <- errors.New("detached billing context has no deadline")
				return
			}
			deadlines <- deadline
			if attempt < 2 {
				timer := time.NewTimer(10 * time.Millisecond)
				select {
				case <-timer.C:
				case <-billingCtx.Done():
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					cancel()
					nestedErr <- billingCtx.Err()
					return
				}
				cancel()
				continue
			}
			<-billingCtx.Done()
			nestedErr <- billingCtx.Err()
			cancel()
		}
	}))

	first := <-deadlines
	second := <-deadlines
	third := <-deadlines
	require.WithinDuration(t, first, second, 2*time.Millisecond)
	require.WithinDuration(t, first, third, 2*time.Millisecond)
	require.ErrorIs(t, <-nestedErr, context.DeadlineExceeded)
	require.GreaterOrEqual(t, time.Since(started), taskTimeout-25*time.Millisecond)
	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func TestUsageRecordWorkerPool_AutoScaleDisabledKeepsFixedConcurrency(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           2,
		QueueSize:             4,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicyDrop,
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      false,
		AutoScaleMinWorkers:   1,
		AutoScaleMaxWorkers:   4,
		AutoScaleUpPercent:    10,
		AutoScaleDownPercent:  1,
		AutoScaleUpStep:       2,
		AutoScaleDownStep:     2,
		AutoScaleInterval:     10 * time.Millisecond,
		AutoScaleCooldown:     10 * time.Millisecond,
	})
	t.Cleanup(pool.Stop)

	require.Equal(t, 2, pool.Stats().MaxConcurrency)

	block := make(chan struct{})
	for i := 0; i < 4; i++ {
		require.Equal(t, UsageRecordSubmitModeEnqueued, pool.Submit(func(ctx context.Context) {
			<-block
		}))
	}

	time.Sleep(120 * time.Millisecond)
	require.Equal(t, 2, pool.Stats().MaxConcurrency)
	close(block)
}

func TestUsageRecordWorkerPool_OptionsFromConfig_AutoScaleDisabled(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.UsageRecord.WorkerCount = 64
	cfg.Gateway.UsageRecord.QueueSize = 128
	cfg.Gateway.UsageRecord.TaskTimeoutSeconds = 7
	cfg.Gateway.UsageRecord.OverflowPolicy = config.UsageRecordOverflowPolicyDrop
	cfg.Gateway.UsageRecord.OverflowSamplePercent = 0
	cfg.Gateway.UsageRecord.AutoScaleEnabled = false
	cfg.Gateway.UsageRecord.AutoScaleMinWorkers = 1
	cfg.Gateway.UsageRecord.AutoScaleMaxWorkers = 512

	opts := usageRecordPoolOptionsFromConfig(cfg)
	require.False(t, opts.AutoScaleEnabled)
	require.Equal(t, 64, opts.WorkerCount)
	require.Equal(t, 64, opts.AutoScaleMinWorkers)
	require.Equal(t, 64, opts.AutoScaleMaxWorkers)
	require.Equal(t, 7*time.Second, opts.TaskTimeout)
}

func TestUsageRecordWorkerPool_StringHelpers(t *testing.T) {
	require.Equal(t, "enqueued", UsageRecordSubmitModeEnqueued.String())
	stats := UsageRecordWorkerPoolStats{RunningWorkers: 2, WaitingTasks: 3, SubmittedTasks: 5, DroppedTasks: 1}
	require.Contains(t, stats.String(), "running=2")
	require.Contains(t, stats.String(), "waiting=3")
}

func TestNewUsageRecordWorkerPool_FromConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.UsageRecord.WorkerCount = 3
	cfg.Gateway.UsageRecord.QueueSize = 16
	cfg.Gateway.UsageRecord.TaskTimeoutSeconds = 2
	cfg.Gateway.UsageRecord.OverflowPolicy = config.UsageRecordOverflowPolicyDrop
	cfg.Gateway.UsageRecord.AutoScaleEnabled = false

	pool := NewUsageRecordWorkerPool(cfg)
	t.Cleanup(pool.Stop)

	stats := pool.Stats()
	require.Equal(t, 3, stats.MaxConcurrency)
}

func TestUsageRecordWorkerPool_OptionsFromConfig_NilConfig(t *testing.T) {
	opts := usageRecordPoolOptionsFromConfig(nil)
	require.Equal(t, defaultUsageRecordWorkerCount, opts.WorkerCount)
	require.Equal(t, defaultUsageRecordQueueSize, opts.QueueSize)
	require.Equal(t, time.Duration(defaultUsageRecordTaskTimeoutSeconds)*time.Second, opts.TaskTimeout)
	require.Equal(t, defaultUsageRecordOverflowPolicy, opts.OverflowPolicy)
	require.Equal(t, defaultUsageRecordOverflowSampleRatio, opts.OverflowSamplePercent)
	require.True(t, opts.AutoScaleEnabled)
	require.Equal(t, defaultUsageRecordAutoScaleMinWorkers, opts.AutoScaleMinWorkers)
	require.Equal(t, defaultUsageRecordAutoScaleMaxWorkers, opts.AutoScaleMaxWorkers)
}

func TestUsageRecordWorkerPool_NormalizeOptions_BoundsAndDefaults(t *testing.T) {
	opts := normalizeUsageRecordPoolOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           0,
		QueueSize:             0,
		TaskTimeout:           0,
		OverflowPolicy:        "invalid",
		OverflowSamplePercent: 300,
		AutoScaleEnabled:      true,
		AutoScaleMinWorkers:   0,
		AutoScaleMaxWorkers:   0,
		AutoScaleUpPercent:    0,
		AutoScaleDownPercent:  100,
		AutoScaleUpStep:       0,
		AutoScaleDownStep:     0,
		AutoScaleInterval:     0,
		AutoScaleCooldown:     -time.Second,
	})

	require.Equal(t, defaultUsageRecordWorkerCount, opts.WorkerCount)
	require.Equal(t, defaultUsageRecordQueueSize, opts.QueueSize)
	require.Equal(t, time.Duration(defaultUsageRecordTaskTimeoutSeconds)*time.Second, opts.TaskTimeout)
	require.Equal(t, defaultUsageRecordOverflowPolicy, opts.OverflowPolicy)
	require.Equal(t, 100, opts.OverflowSamplePercent)
	require.Equal(t, defaultUsageRecordAutoScaleMinWorkers, opts.AutoScaleMinWorkers)
	require.Equal(t, defaultUsageRecordAutoScaleMaxWorkers, opts.AutoScaleMaxWorkers)
	require.Equal(t, defaultUsageRecordAutoScaleUpPercent, opts.AutoScaleUpPercent)
	require.Equal(t, defaultUsageRecordAutoScaleDownPercent, opts.AutoScaleDownPercent)
	require.Equal(t, defaultUsageRecordAutoScaleUpStep, opts.AutoScaleUpStep)
	require.Equal(t, defaultUsageRecordAutoScaleDownStep, opts.AutoScaleDownStep)
	require.Equal(t, defaultUsageRecordAutoScaleInterval, opts.AutoScaleInterval)
	require.Equal(t, defaultUsageRecordAutoScaleCooldown, opts.AutoScaleCooldown)
}

func TestUsageRecordWorkerPool_NormalizeOptions_SampleAndAutoScaleDisabled(t *testing.T) {
	sampleOpts := normalizeUsageRecordPoolOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:           32,
		QueueSize:             128,
		TaskTimeout:           time.Second,
		OverflowPolicy:        config.UsageRecordOverflowPolicySample,
		OverflowSamplePercent: 0,
		AutoScaleEnabled:      true,
		AutoScaleMinWorkers:   64,
		AutoScaleMaxWorkers:   48,
		AutoScaleUpPercent:    30,
		AutoScaleDownPercent:  40,
		AutoScaleUpStep:       1,
		AutoScaleDownStep:     1,
		AutoScaleInterval:     time.Second,
		AutoScaleCooldown:     time.Second,
	})
	require.Equal(t, defaultUsageRecordOverflowSampleRatio, sampleOpts.OverflowSamplePercent)
	require.Equal(t, 64, sampleOpts.AutoScaleMinWorkers)
	require.Equal(t, 64, sampleOpts.AutoScaleMaxWorkers)
	require.Equal(t, 64, sampleOpts.WorkerCount)
	require.Equal(t, 15, sampleOpts.AutoScaleDownPercent)

	fixedOpts := normalizeUsageRecordPoolOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:      20,
		AutoScaleEnabled: false,
	})
	require.Equal(t, 20, fixedOpts.AutoScaleMinWorkers)
	require.Equal(t, 20, fixedOpts.AutoScaleMaxWorkers)
}

func TestUsageRecordWorkerPool_ShouldSyncFallbackEdgeCases(t *testing.T) {
	pool := &UsageRecordWorkerPool{overflowSamplePercent: 0}
	require.False(t, pool.shouldSyncFallback())

	pool.overflowSamplePercent = 100
	require.True(t, pool.shouldSyncFallback())
	require.True(t, pool.shouldSyncFallback())
}

func TestUsageRecordWorkerPool_StatsAndStop_NilBranches(t *testing.T) {
	var nilPool *UsageRecordWorkerPool
	require.Equal(t, UsageRecordWorkerPoolStats{}, nilPool.Stats())
	require.NotPanics(t, func() { nilPool.Stop() })
	require.NoError(t, nilPool.StopContext(context.Background()))

	emptyPool := &UsageRecordWorkerPool{}
	require.Equal(t, UsageRecordWorkerPoolStats{}, emptyPool.Stats())
	require.NotPanics(t, func() { emptyPool.Stop() })
	require.NoError(t, emptyPool.StopContext(context.Background()))
}

func TestUsageRecordWorkerPool_Execute_PanicAndTimeout(t *testing.T) {
	pool := &UsageRecordWorkerPool{taskTimeout: 30 * time.Millisecond}

	require.NotPanics(t, func() {
		pool.execute(func(ctx context.Context) {
			panic("boom")
		})
	})

	done := make(chan struct{})
	pool.execute(func(ctx context.Context) {
		<-ctx.Done()
		close(done)
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout context not cancelled")
	}
}

func TestUsageRecordWorkerPool_ResizeAndLogDropBranches(t *testing.T) {
	pool := NewUsageRecordWorkerPoolWithOptions(UsageRecordWorkerPoolOptions{
		WorkerCount:      1,
		QueueSize:        8,
		TaskTimeout:      time.Second,
		OverflowPolicy:   config.UsageRecordOverflowPolicyDrop,
		AutoScaleEnabled: false,
	})
	t.Cleanup(pool.Stop)

	// 目标值与当前值相同，应该直接返回。
	pool.resizePool(1, 1, 0, 0, 0, 8, "noop")
	require.Equal(t, 1, pool.Stats().MaxConcurrency)

	// 在限流窗口内应静默返回。
	pool.lastDropLogNanos.Store(time.Now().UnixNano())
	require.NotPanics(t, func() {
		pool.logDrop("full")
	})
}
