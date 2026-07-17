package handler

import (
	"context"
	"errors"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var (
	errOpenAIWSTurnResourcesAlreadyInstalled = errors.New("openai websocket turn resources already installed")
	errOpenAIWSTurnFinalizerInvariant        = errors.New("openai websocket turn finalizer invariant violated")
)

// openAIWSTurnFinalizer transfers per-turn resource ownership from the relay
// goroutine to a pre-reserved worker task. The terminal path only detaches the
// handles and commits to a bounded channel; Redis and database I/O happen in
// the worker. Before the next provider write, WaitPreviousRelease ensures the
// preceding concurrency slots have actually been released.
type openAIWSTurnFinalizer struct {
	mu sync.Mutex

	userRelease            func()
	accountRelease         func()
	reservation            *service.UsageRecordReservation
	previousReleaseDone    chan struct{}
	failed                 bool
	orphanedUserRelease    func()
	orphanedAccountRelease func()
}

func newOpenAIWSTurnFinalizer() *openAIWSTurnFinalizer {
	done := make(chan struct{})
	close(done)
	return &openAIWSTurnFinalizer{previousReleaseDone: done}
}

func (f *openAIWSTurnFinalizer) InstallUserRelease(release func()) error {
	if f == nil {
		return errOpenAIWSTurnFinalizerInvariant
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.userRelease != nil {
		return errOpenAIWSTurnResourcesAlreadyInstalled
	}
	f.userRelease = release
	return nil
}

func (f *openAIWSTurnFinalizer) InstallAccountRelease(release func()) error {
	if f == nil {
		return errOpenAIWSTurnFinalizerInvariant
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.accountRelease != nil {
		return errOpenAIWSTurnResourcesAlreadyInstalled
	}
	f.accountRelease = release
	return nil
}

func (f *openAIWSTurnFinalizer) HasUserRelease() bool {
	if f == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.userRelease != nil
}

// HasActiveTurn reports whether this finalizer still owns any resource for a
// provider step. Connection-level exits after a completed terminal have no
// such resources and must not synthesize another failed turn.
func (f *openAIWSTurnFinalizer) HasActiveTurn() bool {
	if f == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reservation != nil || f.userRelease != nil || f.accountRelease != nil
}

func (f *openAIWSTurnFinalizer) Reserve(ctx context.Context, pool *service.UsageRecordWorkerPool) error {
	if f == nil || pool == nil {
		return errOpenAIWSTurnFinalizerInvariant
	}
	f.mu.Lock()
	if f.reservation != nil {
		f.mu.Unlock()
		return nil
	}
	f.mu.Unlock()
	reservation, err := pool.ReserveRequired(ctx)
	if err != nil {
		return err
	}
	f.mu.Lock()
	if f.reservation != nil {
		f.mu.Unlock()
		reservation.Abort()
		return errOpenAIWSTurnResourcesAlreadyInstalled
	}
	f.reservation = reservation
	f.mu.Unlock()
	return nil
}

func (f *openAIWSTurnFinalizer) WaitPreviousRelease(ctx context.Context) error {
	if f == nil {
		return errOpenAIWSTurnFinalizerInvariant
	}
	f.mu.Lock()
	done := f.previousReleaseDone
	failed := f.failed
	f.mu.Unlock()
	if failed {
		return errOpenAIWSTurnFinalizerInvariant
	}
	if done == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Commit detaches the active turn and schedules its finalizer. releaseDone is
// closed immediately after concurrency release by a dedicated finalizer
// worker. Usage persistence runs in a separate required worker, so neither
// optional queue saturation nor database latency delays the next turn.
func (f *openAIWSTurnFinalizer) Commit(afterRelease service.UsageRecordTask) bool {
	if f == nil {
		return false
	}
	f.mu.Lock()
	reservation := f.reservation
	userRelease := f.userRelease
	accountRelease := f.accountRelease
	done := make(chan struct{})
	f.reservation = nil
	f.userRelease = nil
	f.accountRelease = nil
	f.previousReleaseDone = done
	f.mu.Unlock()

	if reservation == nil {
		f.recordCommitFailure(userRelease, accountRelease)
		return false
	}
	finalize := func() {
		defer close(done)
		if accountRelease != nil {
			accountRelease()
		}
		if userRelease != nil {
			userRelease()
		}
	}
	if reservation.CommitFinalizer(finalize, afterRelease) {
		return true
	}
	f.recordCommitFailure(userRelease, accountRelease)
	return false
}

func (f *openAIWSTurnFinalizer) ReleaseAccountNow() {
	if f == nil {
		return
	}
	f.mu.Lock()
	release := f.accountRelease
	f.accountRelease = nil
	f.mu.Unlock()
	if release != nil {
		release()
	}
}

// AbortCurrent is for paths that are proven not to have executed upstream. It
// may perform Redis release synchronously because it is never called while a
// provider terminal gate is held.
func (f *openAIWSTurnFinalizer) AbortCurrent() {
	if f == nil {
		return
	}
	f.mu.Lock()
	reservation := f.reservation
	userRelease := f.userRelease
	accountRelease := f.accountRelease
	orphanedUser := f.orphanedUserRelease
	orphanedAccount := f.orphanedAccountRelease
	f.reservation = nil
	f.userRelease = nil
	f.accountRelease = nil
	f.orphanedUserRelease = nil
	f.orphanedAccountRelease = nil
	f.mu.Unlock()
	if reservation != nil {
		reservation.Abort()
	}
	if accountRelease != nil {
		accountRelease()
	}
	if userRelease != nil {
		userRelease()
	}
	if orphanedAccount != nil {
		orphanedAccount()
	}
	if orphanedUser != nil {
		orphanedUser()
	}
}

func (f *openAIWSTurnFinalizer) recordCommitFailure(userRelease, accountRelease func()) {
	f.mu.Lock()
	f.failed = true
	f.orphanedUserRelease = userRelease
	f.orphanedAccountRelease = accountRelease
	f.mu.Unlock()
}
