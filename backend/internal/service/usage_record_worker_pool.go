package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/alitto/pond/v2"
	"go.uber.org/zap"
)

const (
	defaultUsageRecordWorkerCount          = 128
	defaultUsageRecordQueueSize            = 16384
	defaultUsageRecordTaskTimeoutSeconds   = 5
	defaultUsageRecordRequiredReserveWait  = 100 * time.Millisecond
	defaultUsageRecordOverflowPolicy       = config.UsageRecordOverflowPolicySample
	defaultUsageRecordOverflowSampleRatio  = 10
	defaultUsageRecordAutoScaleEnabled     = true
	defaultUsageRecordAutoScaleMinWorkers  = 128
	defaultUsageRecordAutoScaleMaxWorkers  = 512
	defaultUsageRecordAutoScaleUpPercent   = 70
	defaultUsageRecordAutoScaleDownPercent = 15
	defaultUsageRecordAutoScaleUpStep      = 32
	defaultUsageRecordAutoScaleDownStep    = 16
	defaultUsageRecordAutoScaleInterval    = 3 * time.Second
	defaultUsageRecordAutoScaleCooldown    = 10 * time.Second
	defaultUsageRecordStopTimeout          = 10 * time.Second
	usageRecordDropLogInterval             = 5 * time.Second
)

// UsageRecordTask 是提交到使用量记录池的任务。
// 任务实现应自行处理业务错误日志；池本身只负责调度与超时控制。
type UsageRecordTask func(ctx context.Context)

// UsageRecordSubmitMode 表示任务提交结果。
type UsageRecordSubmitMode string

const (
	UsageRecordSubmitModeEnqueued UsageRecordSubmitMode = "enqueued"
	UsageRecordSubmitModeDropped  UsageRecordSubmitMode = "dropped"
	UsageRecordSubmitModeSync     UsageRecordSubmitMode = "sync_fallback"
)

// UsageRecordWorkerPoolOptions 使用量记录池配置。
type UsageRecordWorkerPoolOptions struct {
	WorkerCount            int
	QueueSize              int
	TaskTimeout            time.Duration
	RequiredReserveTimeout time.Duration
	OverflowPolicy         string
	OverflowSamplePercent  int
	AutoScaleEnabled       bool
	AutoScaleMinWorkers    int
	AutoScaleMaxWorkers    int
	AutoScaleUpPercent     int
	AutoScaleDownPercent   int
	AutoScaleUpStep        int
	AutoScaleDownStep      int
	AutoScaleInterval      time.Duration
	AutoScaleCooldown      time.Duration
}

// UsageRecordWorkerPoolStats 使用量记录池运行时统计。
type UsageRecordWorkerPoolStats struct {
	MaxConcurrency            int
	RunningWorkers            int64
	WaitingTasks              uint64
	SubmittedTasks            uint64
	CompletedTasks            uint64
	SuccessfulTasks           uint64
	FailedTasks               uint64
	DroppedTasks              uint64
	DroppedQueueFull          uint64
	DroppedPoolStopped        uint64
	SyncFallbackTasks         uint64
	RequiredOutstanding       int
	RequiredCapacity          int
	RequiredFinalizerQueued   int
	RequiredPersistenceQueued int
	RequiredReserveWaits      uint64
	RequiredReserveTimeouts   uint64
	RequiredReserveWaitNanos  uint64
}

// UsageRecordWorkerPool 提供“有界队列 + 固定 worker”的异步执行器。
// 用于替代请求路径里的直接 goroutine，避免高并发时无界堆积。
type UsageRecordWorkerPool struct {
	pool                     pond.Pool
	taskTimeout              time.Duration
	requiredReserveTimeout   time.Duration
	overflowPolicy           string
	overflowSamplePercent    int
	overflowCounter          atomic.Uint64
	droppedQueueFull         atomic.Uint64
	droppedPoolStopped       atomic.Uint64
	syncFallback             atomic.Uint64
	stopping                 atomic.Bool
	lastDropLogNanos         atomic.Int64
	autoScaleEnabled         bool
	autoScaleMinWorkers      int
	autoScaleMaxWorkers      int
	autoScaleUpPercent       int
	autoScaleDownPercent     int
	autoScaleUpStep          int
	autoScaleDownStep        int
	autoScaleInterval        time.Duration
	autoScaleCooldown        time.Duration
	lastScaleNanos           atomic.Int64
	autoScaleCancel          context.CancelFunc
	lifecycleWg              sync.WaitGroup
	stopOnce                 sync.Once
	stopDone                 chan struct{}
	requiredFinalizerQueue   chan usageRecordRequiredTask
	requiredPersistenceQueue chan UsageRecordTask
	requiredSlots            chan struct{}
	requiredStop             chan struct{}
	requiredMu               sync.Mutex
	requiredStopping         bool
	requiredOutstanding      sync.WaitGroup
	requiredFinalizerWg      sync.WaitGroup
	requiredPersistenceWg    sync.WaitGroup
	requiredReserveWaits     atomic.Uint64
	requiredReserveTimeouts  atomic.Uint64
	requiredReserveWaitNanos atomic.Uint64
}

var ErrUsageRecordWorkerPoolStopped = errors.New("usage record worker pool stopped")
var ErrUsageRecordRequiredCapacity = errors.New("required usage record capacity unavailable")

const (
	usageRecordReservationPending uint32 = iota
	usageRecordReservationCommitted
	usageRecordReservationAborted
)

// UsageRecordReservation reserves bounded finalizer capacity before an
// upstream request can execute. Commit is an O(1) channel send and never waits
// for the worker pool or database, so provider terminal processing stays off
// the persistence backpressure path.
type UsageRecordReservation struct {
	pool  *UsageRecordWorkerPool
	state atomic.Uint32
}

type usageRecordRequiredTask struct {
	finalize    func()
	persistence UsageRecordTask
}

// NewUsageRecordWorkerPool 从配置构建使用量记录池。
func NewUsageRecordWorkerPool(cfg *config.Config) *UsageRecordWorkerPool {
	opts := usageRecordPoolOptionsFromConfig(cfg)
	return NewUsageRecordWorkerPoolWithOptions(opts)
}

// NewUsageRecordWorkerPoolWithOptions 根据给定参数构建使用量记录池。
func NewUsageRecordWorkerPoolWithOptions(opts UsageRecordWorkerPoolOptions) *UsageRecordWorkerPool {
	opts = normalizeUsageRecordPoolOptions(opts)
	requiredCapacity := opts.WorkerCount + opts.QueueSize

	p := &UsageRecordWorkerPool{
		taskTimeout:              opts.TaskTimeout,
		requiredReserveTimeout:   opts.RequiredReserveTimeout,
		overflowPolicy:           opts.OverflowPolicy,
		overflowSamplePercent:    opts.OverflowSamplePercent,
		autoScaleEnabled:         opts.AutoScaleEnabled,
		autoScaleMinWorkers:      opts.AutoScaleMinWorkers,
		autoScaleMaxWorkers:      opts.AutoScaleMaxWorkers,
		autoScaleUpPercent:       opts.AutoScaleUpPercent,
		autoScaleDownPercent:     opts.AutoScaleDownPercent,
		autoScaleUpStep:          opts.AutoScaleUpStep,
		autoScaleDownStep:        opts.AutoScaleDownStep,
		autoScaleInterval:        opts.AutoScaleInterval,
		autoScaleCooldown:        opts.AutoScaleCooldown,
		requiredFinalizerQueue:   make(chan usageRecordRequiredTask, requiredCapacity),
		requiredPersistenceQueue: make(chan UsageRecordTask, requiredCapacity),
		requiredSlots:            make(chan struct{}, requiredCapacity),
		requiredStop:             make(chan struct{}),
		stopDone:                 make(chan struct{}),
	}

	p.pool = pond.NewPool(
		opts.WorkerCount,
		pond.WithQueueSize(opts.QueueSize),
	)
	if p.autoScaleEnabled {
		p.startAutoScaler()
	}
	p.startRequiredWorkers(opts.WorkerCount)
	return p
}

// Submit 提交一个使用量记录任务。
// 提交失败（队列满）时按 overflowPolicy 执行降级策略：drop/sample/sync。
func (p *UsageRecordWorkerPool) Submit(task UsageRecordTask) UsageRecordSubmitMode {
	if p == nil || task == nil {
		return UsageRecordSubmitModeDropped
	}
	if p.pool == nil || p.stopping.Load() || p.pool.Stopped() {
		p.droppedPoolStopped.Add(1)
		p.logDrop("stopped")
		return UsageRecordSubmitModeDropped
	}

	_, ok := p.pool.TrySubmit(func() {
		p.execute(task)
	})
	if ok {
		return UsageRecordSubmitModeEnqueued
	}

	if p.stopping.Load() || p.pool.Stopped() {
		p.droppedPoolStopped.Add(1)
		p.logDrop("stopped")
		return UsageRecordSubmitModeDropped
	}

	switch p.overflowPolicy {
	case config.UsageRecordOverflowPolicySync:
		p.syncFallback.Add(1)
		p.execute(task)
		return UsageRecordSubmitModeSync
	case config.UsageRecordOverflowPolicySample:
		if p.shouldSyncFallback() {
			p.syncFallback.Add(1)
			p.execute(task)
			return UsageRecordSubmitModeSync
		}
	}

	p.droppedQueueFull.Add(1)
	p.logDrop("full")
	return UsageRecordSubmitModeDropped
}

// SubmitRequired applies bounded queue backpressure instead of dropping or
// executing the task synchronously. Long-lived relay protocols use this path
// so usage finalization never runs on the frame-processing goroutine.
func (p *UsageRecordWorkerPool) SubmitRequired(task UsageRecordTask) UsageRecordSubmitMode {
	reservation, err := p.ReserveRequired(context.Background())
	if err != nil {
		if p != nil {
			if errors.Is(err, ErrUsageRecordRequiredCapacity) {
				p.droppedQueueFull.Add(1)
				p.logDrop("required_full")
			} else {
				p.droppedPoolStopped.Add(1)
				p.logDrop("stopped")
			}
		}
		return UsageRecordSubmitModeDropped
	}
	if !reservation.Commit(task) {
		p.droppedPoolStopped.Add(1)
		p.logDrop("stopped")
		return UsageRecordSubmitModeDropped
	}
	return UsageRecordSubmitModeEnqueued
}

// ReserveRequired applies bounded backpressure before provider execution. A
// caller must eventually call Commit or Abort exactly once.
func (p *UsageRecordWorkerPool) ReserveRequired(ctx context.Context) (*UsageRecordReservation, error) {
	if p == nil || p.pool == nil || p.requiredSlots == nil || p.requiredStop == nil {
		return nil, ErrUsageRecordWorkerPoolStopped
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case p.requiredSlots <- struct{}{}:
		return p.finishRequiredReservation()
	default:
	}

	p.requiredReserveWaits.Add(1)
	waitStarted := time.Now()
	timer := time.NewTimer(p.requiredReserveTimeout)
	defer timer.Stop()
	select {
	case p.requiredSlots <- struct{}{}:
		p.requiredReserveWaitNanos.Add(uint64(time.Since(waitStarted)))
		return p.finishRequiredReservation()
	case <-timer.C:
		p.requiredReserveWaitNanos.Add(uint64(time.Since(waitStarted)))
		p.requiredReserveTimeouts.Add(1)
		return nil, ErrUsageRecordRequiredCapacity
	case <-ctx.Done():
		p.requiredReserveWaitNanos.Add(uint64(time.Since(waitStarted)))
		return nil, ctx.Err()
	case <-p.requiredStop:
		p.requiredReserveWaitNanos.Add(uint64(time.Since(waitStarted)))
		return nil, ErrUsageRecordWorkerPoolStopped
	}
}
func (p *UsageRecordWorkerPool) finishRequiredReservation() (*UsageRecordReservation, error) {
	p.requiredMu.Lock()
	if p.requiredStopping || p.pool.Stopped() {
		p.requiredMu.Unlock()
		<-p.requiredSlots
		return nil, ErrUsageRecordWorkerPoolStopped
	}
	p.requiredOutstanding.Add(1)
	p.requiredMu.Unlock()
	return &UsageRecordReservation{pool: p}, nil
}

// Commit transfers the reserved slot to the required persistence pipeline.
// The reservation invariant guarantees that this buffered send has capacity.
func (r *UsageRecordReservation) Commit(task UsageRecordTask) bool {
	return r.CommitFinalizer(nil, task)
}

// CommitFinalizer transfers a two-phase required task to dedicated workers.
// finalize runs first and independently of both optional usage work and
// required persistence. Commit itself is O(1): it only changes reservation
// ownership and sends to a pre-reserved buffered queue.
func (r *UsageRecordReservation) CommitFinalizer(finalize func(), persistence UsageRecordTask) bool {
	if r == nil || r.pool == nil || (finalize == nil && persistence == nil) {
		if r != nil {
			r.Abort()
		}
		return false
	}
	if !r.state.CompareAndSwap(usageRecordReservationPending, usageRecordReservationCommitted) {
		return false
	}
	r.pool.requiredFinalizerQueue <- usageRecordRequiredTask{
		finalize:    finalize,
		persistence: persistence,
	}
	return true
}

// Abort releases unused capacity. It is safe to call from deferred cleanup and
// is idempotent with Commit and other Abort calls.
func (r *UsageRecordReservation) Abort() bool {
	if r == nil || r.pool == nil || !r.state.CompareAndSwap(usageRecordReservationPending, usageRecordReservationAborted) {
		return false
	}
	r.pool.releaseRequiredSlot()
	return true
}

// Stats 返回当前池状态与计数器。
func (p *UsageRecordWorkerPool) Stats() UsageRecordWorkerPoolStats {
	if p == nil || p.pool == nil {
		return UsageRecordWorkerPoolStats{}
	}
	return UsageRecordWorkerPoolStats{
		MaxConcurrency:            p.pool.MaxConcurrency(),
		RunningWorkers:            p.pool.RunningWorkers(),
		WaitingTasks:              p.pool.WaitingTasks(),
		SubmittedTasks:            p.pool.SubmittedTasks(),
		CompletedTasks:            p.pool.CompletedTasks(),
		SuccessfulTasks:           p.pool.SuccessfulTasks(),
		FailedTasks:               p.pool.FailedTasks(),
		DroppedTasks:              p.pool.DroppedTasks(),
		DroppedQueueFull:          p.droppedQueueFull.Load(),
		DroppedPoolStopped:        p.droppedPoolStopped.Load(),
		SyncFallbackTasks:         p.syncFallback.Load(),
		RequiredOutstanding:       len(p.requiredSlots),
		RequiredCapacity:          cap(p.requiredSlots),
		RequiredFinalizerQueued:   len(p.requiredFinalizerQueue),
		RequiredPersistenceQueued: len(p.requiredPersistenceQueue),
		RequiredReserveWaits:      p.requiredReserveWaits.Load(),
		RequiredReserveTimeouts:   p.requiredReserveTimeouts.Load(),
		RequiredReserveWaitNanos:  p.requiredReserveWaitNanos.Load(),
	}
}

// Stop starts a graceful drain and waits for at most the default shutdown
// budget. A timed-out drain continues in the background so channels that can
// still receive a reserved task are never closed prematurely.
func (p *UsageRecordWorkerPool) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultUsageRecordStopTimeout)
	defer cancel()
	if err := p.StopContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.L().With(
			zap.String("component", "service.usage_record_worker_pool"),
			zap.Error(err),
		).Warn("usage_record.stop_timeout")
	}
}

// StopContext prevents new required reservations, starts a graceful drain,
// and waits until either the drain completes or ctx expires. On timeout it
// deliberately leaves both required queues open: an existing reservation may
// still Commit, and the background drain closes queues only after every
// reservation has committed and persisted or aborted.
func (p *UsageRecordWorkerPool) StopContext(ctx context.Context) error {
	if p == nil || p.pool == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.stopOnce.Do(func() {
		if p.autoScaleCancel != nil {
			p.autoScaleCancel()
		}
		p.stopping.Store(true)
		optionalStop := p.pool.Stop()
		p.requiredMu.Lock()
		p.requiredStopping = true
		close(p.requiredStop)
		p.requiredMu.Unlock()
		go p.finishStop(optionalStop)
	})

	select {
	case <-p.stopDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *UsageRecordWorkerPool) finishStop(optionalStop pond.Task) {
	p.lifecycleWg.Wait()
	p.requiredOutstanding.Wait()
	close(p.requiredFinalizerQueue)
	p.requiredFinalizerWg.Wait()
	close(p.requiredPersistenceQueue)
	p.requiredPersistenceWg.Wait()
	if optionalStop != nil {
		_ = optionalStop.Wait()
	}
	close(p.stopDone)
}

func (p *UsageRecordWorkerPool) startRequiredWorkers(workerCount int) {
	if p == nil || p.requiredFinalizerQueue == nil || p.requiredPersistenceQueue == nil || workerCount <= 0 {
		return
	}
	for range workerCount {
		p.requiredFinalizerWg.Add(1)
		go func() {
			defer p.requiredFinalizerWg.Done()
			for task := range p.requiredFinalizerQueue {
				p.executeRequiredFinalizer(task.finalize)
				if task.persistence == nil {
					p.releaseRequiredSlot()
					continue
				}
				// A task keeps its reserved slot until persistence completes.
				// Therefore at most cap(requiredSlots) tasks can reach this
				// queue, and this send cannot wait for queue capacity.
				p.requiredPersistenceQueue <- task.persistence
			}
		}()

		p.requiredPersistenceWg.Add(1)
		go func() {
			defer p.requiredPersistenceWg.Done()
			for task := range p.requiredPersistenceQueue {
				p.execute(task)
				p.releaseRequiredSlot()
			}
		}()
	}
}

func (p *UsageRecordWorkerPool) executeRequiredFinalizer(finalize func()) {
	if finalize == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "service.usage_record_worker_pool"),
				zap.Any("panic", recovered),
			).Error("usage_record.required_finalizer_panic")
		}
	}()
	finalize()
}

func (p *UsageRecordWorkerPool) releaseRequiredSlot() {
	if p == nil || p.requiredSlots == nil {
		return
	}
	<-p.requiredSlots
	p.requiredOutstanding.Done()
}

func (p *UsageRecordWorkerPool) startAutoScaler() {
	ctx, cancel := context.WithCancel(context.Background())
	p.autoScaleCancel = cancel

	p.lifecycleWg.Add(1)
	go func() {
		defer p.lifecycleWg.Done()

		ticker := time.NewTicker(p.autoScaleInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.autoScaleTick()
			}
		}
	}()
}

func (p *UsageRecordWorkerPool) autoScaleTick() {
	if p == nil || p.pool == nil || p.pool.Stopped() {
		return
	}
	queueSize := p.pool.QueueSize()
	if queueSize <= 0 {
		return
	}
	current := p.pool.MaxConcurrency()
	waiting := int(p.pool.WaitingTasks())
	running := int(p.pool.RunningWorkers())
	if current <= 0 || waiting < 0 {
		return
	}
	queuePercent := waiting * 100 / queueSize
	runningPercent := 0
	if current > 0 {
		runningPercent = running * 100 / current
	}

	now := time.Now()
	lastScaleNanos := p.lastScaleNanos.Load()
	if lastScaleNanos > 0 && now.Sub(time.Unix(0, lastScaleNanos)) < p.autoScaleCooldown {
		return
	}

	// 扩容优先：队列占用率超过阈值时，按步长提升并发上限。
	if queuePercent >= p.autoScaleUpPercent && current < p.autoScaleMaxWorkers {
		target := current + p.autoScaleUpStep
		if target > p.autoScaleMaxWorkers {
			target = p.autoScaleMaxWorkers
		}
		p.resizePool(current, target, queuePercent, waiting, runningPercent, queueSize, "scale_up")
		return
	}

	// 缩容：仅在队列为空且运行利用率低时收缩，避免高负载下“无排队误缩容”导致震荡。
	if queuePercent <= p.autoScaleDownPercent && waiting == 0 &&
		runningPercent <= p.autoScaleDownPercent &&
		current > p.autoScaleMinWorkers {
		target := current - p.autoScaleDownStep
		if target < p.autoScaleMinWorkers {
			target = p.autoScaleMinWorkers
		}
		p.resizePool(current, target, queuePercent, waiting, runningPercent, queueSize, "scale_down")
	}
}

func (p *UsageRecordWorkerPool) resizePool(current, target, queuePercent, waiting, runningPercent, queueSize int, action string) {
	if target == current {
		return
	}
	p.pool.Resize(target)
	p.lastScaleNanos.Store(time.Now().UnixNano())

	logger.L().With(
		zap.String("component", "service.usage_record_worker_pool"),
		zap.String("action", action),
		zap.Int("from_workers", current),
		zap.Int("to_workers", target),
		zap.Int("queue_percent", queuePercent),
		zap.Int("waiting_tasks", waiting),
		zap.Int("running_percent", runningPercent),
		zap.Int("queue_size", queueSize),
	).Info("usage_record.auto_scale")
}

func (p *UsageRecordWorkerPool) shouldSyncFallback() bool {
	if p.overflowSamplePercent <= 0 {
		return false
	}
	n := p.overflowCounter.Add(1)
	return int((n-1)%100) < p.overflowSamplePercent
}

func (p *UsageRecordWorkerPool) execute(task UsageRecordTask) {
	ctx, cancel := context.WithTimeout(context.Background(), p.taskTimeout)
	defer cancel()

	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "service.usage_record_worker_pool"),
				zap.Any("panic", recovered),
			).Error("usage_record.task_panic")
		}
	}()

	task(ctx)
}

func (p *UsageRecordWorkerPool) logDrop(reason string) {
	now := time.Now().UnixNano()
	last := p.lastDropLogNanos.Load()
	if now-last < int64(usageRecordDropLogInterval) {
		return
	}
	if !p.lastDropLogNanos.CompareAndSwap(last, now) {
		return
	}

	stats := p.Stats()
	logger.L().With(
		zap.String("component", "service.usage_record_worker_pool"),
		zap.String("reason", reason),
		zap.String("overflow_policy", p.overflowPolicy),
		zap.Int64("running_workers", stats.RunningWorkers),
		zap.Uint64("waiting_tasks", stats.WaitingTasks),
		zap.Uint64("dropped_queue_full", stats.DroppedQueueFull),
		zap.Uint64("dropped_pool_stopped", stats.DroppedPoolStopped),
		zap.Uint64("sync_fallback_tasks", stats.SyncFallbackTasks),
	).Warn("usage_record.task_dropped")
}

func usageRecordPoolOptionsFromConfig(cfg *config.Config) UsageRecordWorkerPoolOptions {
	opts := UsageRecordWorkerPoolOptions{
		WorkerCount:            defaultUsageRecordWorkerCount,
		QueueSize:              defaultUsageRecordQueueSize,
		TaskTimeout:            time.Duration(defaultUsageRecordTaskTimeoutSeconds) * time.Second,
		RequiredReserveTimeout: defaultUsageRecordRequiredReserveWait,
		OverflowPolicy:         defaultUsageRecordOverflowPolicy,
		OverflowSamplePercent:  defaultUsageRecordOverflowSampleRatio,
		AutoScaleEnabled:       defaultUsageRecordAutoScaleEnabled,
		AutoScaleMinWorkers:    defaultUsageRecordAutoScaleMinWorkers,
		AutoScaleMaxWorkers:    defaultUsageRecordAutoScaleMaxWorkers,
		AutoScaleUpPercent:     defaultUsageRecordAutoScaleUpPercent,
		AutoScaleDownPercent:   defaultUsageRecordAutoScaleDownPercent,
		AutoScaleUpStep:        defaultUsageRecordAutoScaleUpStep,
		AutoScaleDownStep:      defaultUsageRecordAutoScaleDownStep,
		AutoScaleInterval:      defaultUsageRecordAutoScaleInterval,
		AutoScaleCooldown:      defaultUsageRecordAutoScaleCooldown,
	}
	if cfg == nil {
		return opts
	}
	if cfg.Gateway.UsageRecord.WorkerCount > 0 {
		opts.WorkerCount = cfg.Gateway.UsageRecord.WorkerCount
	}
	if cfg.Gateway.UsageRecord.QueueSize > 0 {
		opts.QueueSize = cfg.Gateway.UsageRecord.QueueSize
	}
	if cfg.Gateway.UsageRecord.TaskTimeoutSeconds > 0 {
		opts.TaskTimeout = time.Duration(cfg.Gateway.UsageRecord.TaskTimeoutSeconds) * time.Second
	}
	if cfg.Gateway.UsageRecord.RequiredReserveTimeoutMS > 0 {
		opts.RequiredReserveTimeout = time.Duration(cfg.Gateway.UsageRecord.RequiredReserveTimeoutMS) * time.Millisecond
	}
	if policy := strings.TrimSpace(strings.ToLower(cfg.Gateway.UsageRecord.OverflowPolicy)); policy != "" {
		opts.OverflowPolicy = policy
	}
	if cfg.Gateway.UsageRecord.OverflowSamplePercent >= 0 {
		opts.OverflowSamplePercent = cfg.Gateway.UsageRecord.OverflowSamplePercent
	}
	opts.AutoScaleEnabled = cfg.Gateway.UsageRecord.AutoScaleEnabled
	if cfg.Gateway.UsageRecord.AutoScaleMinWorkers > 0 {
		opts.AutoScaleMinWorkers = cfg.Gateway.UsageRecord.AutoScaleMinWorkers
	}
	if cfg.Gateway.UsageRecord.AutoScaleMaxWorkers > 0 {
		opts.AutoScaleMaxWorkers = cfg.Gateway.UsageRecord.AutoScaleMaxWorkers
	}
	if cfg.Gateway.UsageRecord.AutoScaleUpQueuePercent > 0 {
		opts.AutoScaleUpPercent = cfg.Gateway.UsageRecord.AutoScaleUpQueuePercent
	}
	if cfg.Gateway.UsageRecord.AutoScaleDownQueuePercent >= 0 {
		opts.AutoScaleDownPercent = cfg.Gateway.UsageRecord.AutoScaleDownQueuePercent
	}
	if cfg.Gateway.UsageRecord.AutoScaleUpStep > 0 {
		opts.AutoScaleUpStep = cfg.Gateway.UsageRecord.AutoScaleUpStep
	}
	if cfg.Gateway.UsageRecord.AutoScaleDownStep > 0 {
		opts.AutoScaleDownStep = cfg.Gateway.UsageRecord.AutoScaleDownStep
	}
	if cfg.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds > 0 {
		opts.AutoScaleInterval = time.Duration(cfg.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds) * time.Second
	}
	if cfg.Gateway.UsageRecord.AutoScaleCooldownSeconds >= 0 {
		opts.AutoScaleCooldown = time.Duration(cfg.Gateway.UsageRecord.AutoScaleCooldownSeconds) * time.Second
	}
	return normalizeUsageRecordPoolOptions(opts)
}

func normalizeUsageRecordPoolOptions(opts UsageRecordWorkerPoolOptions) UsageRecordWorkerPoolOptions {
	if opts.WorkerCount <= 0 {
		opts.WorkerCount = defaultUsageRecordWorkerCount
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = defaultUsageRecordQueueSize
	}
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = time.Duration(defaultUsageRecordTaskTimeoutSeconds) * time.Second
	}
	if opts.RequiredReserveTimeout <= 0 {
		opts.RequiredReserveTimeout = defaultUsageRecordRequiredReserveWait
	}
	switch strings.ToLower(strings.TrimSpace(opts.OverflowPolicy)) {
	case config.UsageRecordOverflowPolicyDrop,
		config.UsageRecordOverflowPolicySample,
		config.UsageRecordOverflowPolicySync:
		opts.OverflowPolicy = strings.ToLower(strings.TrimSpace(opts.OverflowPolicy))
	default:
		opts.OverflowPolicy = defaultUsageRecordOverflowPolicy
	}
	if opts.OverflowSamplePercent < 0 {
		opts.OverflowSamplePercent = 0
	}
	if opts.OverflowSamplePercent > 100 {
		opts.OverflowSamplePercent = 100
	}
	if opts.OverflowPolicy == config.UsageRecordOverflowPolicySample && opts.OverflowSamplePercent == 0 {
		opts.OverflowSamplePercent = defaultUsageRecordOverflowSampleRatio
	}
	if opts.AutoScaleEnabled {
		if opts.AutoScaleMinWorkers <= 0 {
			opts.AutoScaleMinWorkers = defaultUsageRecordAutoScaleMinWorkers
		}
		if opts.AutoScaleMaxWorkers <= 0 {
			opts.AutoScaleMaxWorkers = defaultUsageRecordAutoScaleMaxWorkers
		}
		if opts.AutoScaleMaxWorkers < opts.AutoScaleMinWorkers {
			opts.AutoScaleMaxWorkers = opts.AutoScaleMinWorkers
		}
		if opts.WorkerCount < opts.AutoScaleMinWorkers {
			opts.WorkerCount = opts.AutoScaleMinWorkers
		}
		if opts.WorkerCount > opts.AutoScaleMaxWorkers {
			opts.WorkerCount = opts.AutoScaleMaxWorkers
		}
		if opts.AutoScaleUpPercent <= 0 || opts.AutoScaleUpPercent > 100 {
			opts.AutoScaleUpPercent = defaultUsageRecordAutoScaleUpPercent
		}
		if opts.AutoScaleDownPercent < 0 || opts.AutoScaleDownPercent >= 100 {
			opts.AutoScaleDownPercent = defaultUsageRecordAutoScaleDownPercent
		}
		if opts.AutoScaleDownPercent >= opts.AutoScaleUpPercent {
			opts.AutoScaleDownPercent = max(0, opts.AutoScaleUpPercent/2)
		}
		if opts.AutoScaleUpStep <= 0 {
			opts.AutoScaleUpStep = defaultUsageRecordAutoScaleUpStep
		}
		if opts.AutoScaleDownStep <= 0 {
			opts.AutoScaleDownStep = defaultUsageRecordAutoScaleDownStep
		}
		if opts.AutoScaleInterval <= 0 {
			opts.AutoScaleInterval = defaultUsageRecordAutoScaleInterval
		}
		if opts.AutoScaleCooldown < 0 {
			opts.AutoScaleCooldown = defaultUsageRecordAutoScaleCooldown
		}
	} else {
		opts.AutoScaleMinWorkers = opts.WorkerCount
		opts.AutoScaleMaxWorkers = opts.WorkerCount
	}
	return opts
}

func (m UsageRecordSubmitMode) String() string {
	return string(m)
}

func (s UsageRecordWorkerPoolStats) String() string {
	return fmt.Sprintf("running=%d waiting=%d submitted=%d dropped=%d", s.RunningWorkers, s.WaitingTasks, s.SubmittedTasks, s.DroppedTasks)
}
