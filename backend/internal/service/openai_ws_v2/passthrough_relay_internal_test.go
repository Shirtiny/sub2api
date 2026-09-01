package openai_ws_v2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRunEntry_DelegatesRelay(t *testing.T) {
	t.Parallel()

	clientConn := newPassthroughTestFrameConn(nil, false)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.created","response":{"id":"resp_entry"}}`),
		},
		{
			msgType: coderws.MessageText,
			payload: []byte(`{"type":"response.completed","response":{"id":"resp_entry","usage":{"input_tokens":1,"output_tokens":1}}}`),
		},
	}, true)

	result, relayExit := RunEntry(EntryInput{
		Ctx:                context.Background(),
		ClientConn:         clientConn,
		UpstreamConn:       upstreamConn,
		FirstClientMessage: []byte(`{"type":"response.create","model":"gpt-4o","input":[]}`),
	})
	require.Nil(t, relayExit)
	require.Equal(t, "resp_entry", result.RequestID)
}

func TestRunClientToUpstream_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("read client eof", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		runClientToUpstream(
			context.Background(),
			newPassthroughTestFrameConn(nil, true),
			nil,
			func(_ coderws.MessageType, _ []byte) error { return nil },
			func() {},
			nil,
			nil,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "read_client", sig.stage)
		require.True(t, sig.graceful)
	})

	t.Run("write upstream failed", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		runClientToUpstream(
			context.Background(),
			newPassthroughTestFrameConn([]passthroughTestFrame{
				{msgType: coderws.MessageText, payload: []byte(`{"x":1}`)},
			}, true),
			nil,
			func(_ coderws.MessageType, _ []byte) error { return errors.New("boom") },
			func() {},
			nil,
			nil,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "write_upstream", sig.stage)
		require.False(t, sig.graceful)
	})

	t.Run("forwarded counter and trace callback", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		forwarded := &atomic.Int64{}
		traces := make([]RelayTraceEvent, 0, 2)
		runClientToUpstream(
			context.Background(),
			newPassthroughTestFrameConn([]passthroughTestFrame{
				{msgType: coderws.MessageText, payload: []byte(`{"x":1}`)},
			}, true),
			nil,
			func(_ coderws.MessageType, _ []byte) error { return nil },
			func() {},
			forwarded,
			func(event RelayTraceEvent) {
				traces = append(traces, event)
			},
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "read_client", sig.stage)
		require.Equal(t, int64(1), forwarded.Load())
		require.NotEmpty(t, traces)
	})
}

func TestRunUpstreamToClient_ErrorAndDropPaths(t *testing.T) {
	t.Parallel()

	t.Run("read upstream eof", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		drop := &atomic.Bool{}
		drop.Store(false)
		runUpstreamToClient(
			context.Background(),
			newPassthroughTestFrameConn(nil, true),
			func(_ coderws.MessageType, _ []byte) error { return nil },
			time.Now(),
			time.Now,
			&relayState{},
			nil,
			nil,
			nil,
			nil,
			drop,
			nil,
			nil,
			func() {},
			nil,
			nil,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "read_upstream", sig.stage)
		require.True(t, sig.graceful)
	})

	t.Run("write client failed", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		drop := &atomic.Bool{}
		drop.Store(false)
		runUpstreamToClient(
			context.Background(),
			newPassthroughTestFrameConn([]passthroughTestFrame{
				{msgType: coderws.MessageText, payload: []byte(`{"type":"response.output_text.delta","delta":"x"}`)},
			}, true),
			func(_ coderws.MessageType, _ []byte) error { return errors.New("write failed") },
			time.Now(),
			time.Now,
			&relayState{},
			nil,
			nil,
			nil,
			nil,
			drop,
			nil,
			nil,
			func() {},
			nil,
			nil,
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "write_client", sig.stage)
	})

	t.Run("drop downstream and stop on terminal", func(t *testing.T) {
		t.Parallel()

		exitCh := make(chan relayExitSignal, 1)
		drop := &atomic.Bool{}
		drop.Store(true)
		dropped := &atomic.Int64{}
		runUpstreamToClient(
			context.Background(),
			newPassthroughTestFrameConn([]passthroughTestFrame{
				{
					msgType: coderws.MessageText,
					payload: []byte(`{"type":"response.created","response":{"id":"resp_drop"}}`),
				},
				{
					msgType: coderws.MessageText,
					payload: []byte(`{"type":"response.completed","response":{"id":"resp_drop","usage":{"input_tokens":1,"output_tokens":1}}}`),
				},
			}, true),
			func(_ coderws.MessageType, _ []byte) error { return nil },
			time.Now(),
			time.Now,
			&relayState{},
			nil,
			nil,
			nil,
			nil,
			drop,
			nil,
			dropped,
			func() {},
			nil,
			newResponseStepGate([]byte(`{"type":"response.create","model":"gpt-5"}`)),
			exitCh,
		)
		sig := <-exitCh
		require.Equal(t, "drain_terminal", sig.stage)
		require.True(t, sig.graceful)
		require.Equal(t, int64(2), dropped.Load())
	})
}

func TestRunUpstreamToClient_TerminalWriteFailureStillCommitsExactlyOnce(t *testing.T) {
	t.Parallel()

	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_write_fail"}}`)},
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.completed","response":{"id":"resp_write_fail","usage":{"input_tokens":4,"output_tokens":2}}}`)},
	}, true)
	gate := newResponseStepGate([]byte(`{"type":"response.create","model":"gpt-write"}`))
	state := &relayState{}
	drop := &atomic.Bool{}
	exitCh := make(chan relayExitSignal, 1)
	writeCalls := 0
	mutexFreeDuringWrite := false
	mutexFreeDuringCallback := false
	callbackCalls := 0
	var turn RelayTurnResult

	runUpstreamToClient(
		context.Background(),
		upstreamConn,
		func(_ coderws.MessageType, _ []byte) error {
			writeCalls++
			if writeCalls != 2 {
				return nil
			}
			mutexFreeDuringWrite = gate.mu.TryLock()
			if mutexFreeDuringWrite {
				gate.mu.Unlock()
			}
			return errors.New("terminal write failed")
		},
		time.Now(),
		time.Now,
		state,
		nil,
		func(current RelayTurnResult) {
			callbackCalls++
			turn = current
			mutexFreeDuringCallback = gate.mu.TryLock()
			if mutexFreeDuringCallback {
				gate.mu.Unlock()
			}
		},
		nil,
		nil,
		drop,
		nil,
		nil,
		func() {},
		nil,
		gate,
		exitCh,
	)

	exit := <-exitCh
	require.Equal(t, "write_client", exit.stage)
	require.Equal(t, 2, writeCalls)
	require.Equal(t, 1, callbackCalls)
	require.True(t, mutexFreeDuringWrite, "gate mutex must not span downstream writes")
	require.True(t, mutexFreeDuringCallback, "gate mutex must not span terminal callbacks")
	require.Equal(t, "resp_write_fail", turn.RequestID)
	require.Equal(t, "gpt-write", turn.RequestModel)
	require.Equal(t, 4, turn.Usage.InputTokens)
	require.Equal(t, 4, state.usage.InputTokens)
	started, err := gate.begin(coderws.MessageText, []byte(`{"type":"response.create"}`))
	require.False(t, started)
	require.ErrorIs(t, err, errResponseStepClosed)
}

func TestRunUpstreamToClient_RejectedTopLevelErrorDoesNotDoubleFinalize(t *testing.T) {
	t.Parallel()

	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: []byte(`{"type":"error","error":{"code":"rate_limit_exceeded"}}`)},
	}, true)
	gate := newResponseStepGate([]byte(`{"type":"response.create","model":"gpt-error"}`))
	state := &relayState{}
	drop := &atomic.Bool{}
	exitCh := make(chan relayExitSignal, 1)
	rejectedErr := errors.New("safe failover")
	beforeCalls := 0
	callbackCalls := 0

	runUpstreamToClient(
		context.Background(),
		upstreamConn,
		func(_ coderws.MessageType, _ []byte) error { return nil },
		time.Now(),
		time.Now,
		state,
		nil,
		func(RelayTurnResult) { callbackCalls++ },
		func(_ coderws.MessageType, _ []byte, _ bool) error {
			beforeCalls++
			return rejectedErr
		},
		nil,
		drop,
		nil,
		nil,
		func() {},
		nil,
		gate,
		exitCh,
	)

	exit := <-exitCh
	require.Equal(t, "upstream_message", exit.stage)
	require.ErrorIs(t, exit.err, rejectedErr)
	require.Equal(t, 1, beforeCalls)
	require.Zero(t, callbackCalls, "adapter error finalization and relay terminal callback must be mutually exclusive")
	require.Empty(t, state.terminalEventType)
	started, err := gate.begin(coderws.MessageText, []byte(`{"type":"response.create"}`))
	require.False(t, started)
	require.ErrorIs(t, err, errResponseStepClosed)
}

func TestResponseStepGate_CompletedRequiresCreatedProvenance(t *testing.T) {
	t.Parallel()

	gate := newResponseStepGate([]byte(`{"type":"response.create","model":"model-a"}`))
	completedA := observedUpstreamEvent{terminal: true, eventType: "response.completed", responseID: "resp_a"}
	decision := gate.observe(completedA)
	require.ErrorIs(t, decision.err, errResponseCompletedWithoutCreated)
	decision = gate.observe(observedUpstreamEvent{eventType: "response.in_progress", responseID: "resp_a"})
	require.ErrorIs(t, decision.err, errResponseEventWithoutCreated)
	decision = gate.observe(observedUpstreamEvent{eventType: "response.output_text.delta", responseID: "resp_a"})
	require.ErrorIs(t, decision.err, errResponseEventWithoutCreated)

	decision = gate.observe(observedUpstreamEvent{eventType: "response.created"})
	require.ErrorIs(t, decision.err, errResponseCreatedMissingID)

	decision = gate.observe(observedUpstreamEvent{eventType: "response.created", responseID: "resp_a"})
	require.NoError(t, decision.err)
	require.False(t, decision.consume)
	require.NoError(t, gate.observe(observedUpstreamEvent{eventType: "response.output_text.delta", responseID: "resp_a"}).err)
	require.ErrorIs(t,
		gate.observe(observedUpstreamEvent{eventType: "response.output_text.delta", responseID: "resp_b"}).err,
		errResponseStepIDMismatch,
	)
	require.ErrorIs(t,
		gate.observe(observedUpstreamEvent{eventType: "response.output_item.added", responseID: "resp_b"}).err,
		errResponseStepIDMismatch,
	)
	duplicateCreated := gate.observe(observedUpstreamEvent{eventType: "response.created", responseID: "resp_a"})
	require.True(t, duplicateCreated.consume)

	mismatch := gate.observe(observedUpstreamEvent{terminal: true, eventType: "response.completed", responseID: "resp_b"})
	require.ErrorIs(t, mismatch.err, errResponseStepIDMismatch)
	decision = gate.observe(completedA)
	require.True(t, decision.terminalClaimed)
	require.Equal(t, uint64(1), decision.generation)
	require.Equal(t, "model-a", decision.requestModel)
	gate.finishTerminal(false)

	require.True(t, gate.observe(completedA).consume, "settled terminal must be consumed")
	require.True(t, gate.observe(observedUpstreamEvent{eventType: "response.created", responseID: "resp_a"}).consume)
	require.True(t, gate.observe(observedUpstreamEvent{eventType: "response.output_text.delta", responseID: "resp_a"}).consume)

	started, err := gate.begin(coderws.MessageText, []byte(`{"type":"response.create","model":"model-b"}`))
	require.True(t, started)
	require.NoError(t, err)
	require.NoError(t, gate.dispatchPrepared(time.Now()))
	require.False(t, gate.observe(observedUpstreamEvent{eventType: "response.created", responseID: "resp_b"}).consume)
	decision = gate.observe(observedUpstreamEvent{terminal: true, eventType: "response.completed", responseID: "resp_b"})
	require.True(t, decision.terminalClaimed)
	require.Equal(t, uint64(2), decision.generation)
	require.Equal(t, "model-b", decision.requestModel)
	gate.finishTerminal(false)

	started, err = gate.begin(coderws.MessageText, []byte(`{"type":"response.create"}`))
	require.True(t, started)
	require.NoError(t, err)
	require.NoError(t, gate.dispatchPrepared(time.Now()))
	gate.observe(observedUpstreamEvent{eventType: "response.created", responseID: "resp_c"})
	decision = gate.observe(observedUpstreamEvent{terminal: true, eventType: "response.completed", responseID: "resp_c"})
	require.True(t, decision.terminalClaimed)
	require.Equal(t, uint64(3), decision.generation)
	require.Equal(t, "model-b", decision.requestModel, "missing model must inherit the session model")
	gate.finishTerminal(false)
}

func TestResponseStepGateRejectsOversizedOrNonASCIIResponseIDs(t *testing.T) {
	t.Parallel()

	gate := newResponseStepGate([]byte(`{"type":"response.create","model":"model-a"}`))
	oversized := "resp_" + strings.Repeat("x", responseStepIDMaxBytes)
	decision := gate.observe(observedUpstreamEvent{eventType: "response.created", responseID: oversized})
	require.ErrorIs(t, decision.err, errResponseProtocolIdentifierInvalid)

	decision = gate.observe(observedUpstreamEvent{eventType: "response.created", responseID: "resp_非ascii"})
	require.ErrorIs(t, decision.err, errResponseProtocolIdentifierInvalid)
}

func TestObserveUpstreamMessageRejectsLargeIdentifiersBeforeRetention(t *testing.T) {
	t.Parallel()

	oversized := "resp_" + strings.Repeat("x", responseStepIDMaxBytes)
	message := []byte(`{"type":"response.created","response":{"id":"` + oversized + `"}}`)
	observed := observeUpstreamMessage(nil, message, time.Time{}, time.Now, nil)
	require.ErrorIs(t, observed.protocolErr, errResponseProtocolIdentifierInvalid)
	require.Empty(t, observed.responseID)

	valid := observeUpstreamMessage(nil, []byte(`{"type":"response.created","response":{"id":"resp_ok"}}`), time.Time{}, time.Now, nil)
	require.NoError(t, valid.protocolErr)
	require.Equal(t, "resp_ok", valid.responseID)
}

func BenchmarkObserveUpstreamMessageDelta(b *testing.B) {
	benchmarkObserveUpstreamMessageDelta(b, "response.output_text.delta")
}

func BenchmarkObserveUpstreamMessageReasoningTextDelta(b *testing.B) {
	benchmarkObserveUpstreamMessageDelta(b, "response.reasoning_text.delta")
}

func BenchmarkObserveUpstreamMessageCustomToolCallInputDelta(b *testing.B) {
	benchmarkObserveUpstreamMessageDelta(b, "response.custom_tool_call_input.delta")
}

func benchmarkObserveUpstreamMessageDelta(b *testing.B, eventType string) {
	for _, size := range []int{128, 4 * 1024} {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			prefix := []byte(`{"type":"` + eventType + `","response_id":"resp_bench","delta":"`)
			suffix := []byte(`"}`)
			payload := make([]byte, 0, size)
			payload = append(payload, prefix...)
			payload = append(payload, bytes.Repeat([]byte{'x'}, size-len(prefix)-len(suffix))...)
			payload = append(payload, suffix...)
			now := time.Unix(100, 0)
			nowFn := func() time.Time { return now }
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				observed := observeUpstreamMessage(nil, payload, time.Time{}, nowFn, nil)
				if observed.eventType != eventType || len(observed.responseIDRaw) == 0 {
					b.Fatal("unexpected delta envelope")
				}
			}
		})
	}
}

func TestResponseStepGate_PreparingConsumesDelayedTerminal(t *testing.T) {
	t.Parallel()

	gate := newResponseStepGate([]byte(`{"type":"response.create","model":"model-a"}`))
	require.NoError(t, gate.observe(observedUpstreamEvent{eventType: "response.created", responseID: "resp_a"}).err)
	completed := gate.observe(observedUpstreamEvent{terminal: true, eventType: "response.completed", responseID: "resp_a"})
	require.True(t, completed.terminalClaimed)
	gate.finishTerminal(false)

	started, err := gate.begin(coderws.MessageText, []byte(`{"type":"response.create","model":"model-a"}`))
	require.True(t, started)
	require.NoError(t, err)

	for _, event := range []observedUpstreamEvent{
		{eventType: "response.created", responseID: "resp_late"},
		{terminal: true, eventType: "response.failed"},
		{terminal: true, eventType: "error"},
	} {
		decision := gate.observe(event)
		require.True(t, decision.consume)
		require.False(t, decision.terminalClaimed)
		require.NoError(t, decision.err)
	}

	require.NoError(t, gate.dispatchPrepared(time.Now()))
	created := gate.observe(observedUpstreamEvent{eventType: "response.created", responseID: "resp_b"})
	require.False(t, created.consume)
	require.NoError(t, created.err)
}

func TestResponseStepGate_IDLessFailureClosesGate(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{"response.failed", "response.incomplete"} {
		t.Run(eventType, func(t *testing.T) {
			gate := newResponseStepGate([]byte(`{"type":"response.create","model":"gpt-failure"}`))
			decision := gate.observe(observedUpstreamEvent{terminal: true, eventType: eventType})
			require.NoError(t, decision.err)
			require.True(t, decision.terminalClaimed)
			require.True(t, decision.closeGate)
			require.Equal(t, "gpt-failure", decision.requestModel)
			gate.finishTerminal(decision.closeGate)

			started, err := gate.begin(coderws.MessageText, []byte(`{"type":"response.create"}`))
			require.False(t, started)
			require.ErrorIs(t, err, errResponseStepClosed)
		})
	}
}

func TestRunIdleWatchdog_NoTimeoutWhenDisabled(t *testing.T) {
	t.Parallel()

	exitCh := make(chan relayExitSignal, 1)
	lastActivity := &atomic.Int64{}
	lastActivity.Store(time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runIdleWatchdog(ctx, time.Now, 0, lastActivity, nil, exitCh)
	select {
	case <-exitCh:
		t.Fatal("unexpected idle timeout signal")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestHelperFunctionsCoverage(t *testing.T) {
	t.Parallel()

	require.Equal(t, "text", relayMessageTypeString(coderws.MessageText))
	require.Equal(t, "binary", relayMessageTypeString(coderws.MessageBinary))
	require.Contains(t, relayMessageTypeString(coderws.MessageType(99)), "unknown(")

	require.Equal(t, "", relayErrorString(nil))
	require.Equal(t, "x", relayErrorString(errors.New("x")))

	require.True(t, isDisconnectError(io.EOF))
	require.True(t, isDisconnectError(net.ErrClosed))
	require.True(t, isDisconnectError(context.Canceled))
	require.True(t, isDisconnectError(coderws.CloseError{Code: coderws.StatusGoingAway}))
	require.True(t, isDisconnectError(errors.New("broken pipe")))
	require.False(t, isDisconnectError(errors.New("unrelated")))

	require.True(t, isTokenEvent("response.output_text.delta"))
	require.True(t, isTokenEvent("response.output_audio.delta"))
	require.True(t, isTokenEvent("response.completed"))
	require.False(t, isTokenEvent(""))
	require.False(t, isTokenEvent("response.created"))

	require.Equal(t, 2*time.Second, minDuration(2*time.Second, 5*time.Second))
	require.Equal(t, 2*time.Second, minDuration(5*time.Second, 2*time.Second))
	require.Equal(t, 5*time.Second, minDuration(0, 5*time.Second))
	require.Equal(t, 2*time.Second, minDuration(2*time.Second, 0))

	ch := make(chan relayExitSignal, 1)
	ch <- relayExitSignal{stage: "ok"}
	sig, ok := waitRelayExit(ch, 10*time.Millisecond)
	require.True(t, ok)
	require.Equal(t, "ok", sig.stage)
	ch <- relayExitSignal{stage: "ok2"}
	sig, ok = waitRelayExit(ch, 0)
	require.True(t, ok)
	require.Equal(t, "ok2", sig.stage)
	_, ok = waitRelayExit(ch, 10*time.Millisecond)
	require.False(t, ok)

	n, ok := parseUsageIntField(gjson.Get(`{"n":3}`, "n"), true)
	require.True(t, ok)
	require.Equal(t, 3, n)
	_, ok = parseUsageIntField(gjson.Get(`{"n":"x"}`, "n"), true)
	require.False(t, ok)
	n, ok = parseUsageIntField(gjson.Result{}, false)
	require.True(t, ok)
	require.Equal(t, 0, n)
	_, ok = parseUsageIntField(gjson.Result{}, true)
	require.False(t, ok)
}

func TestParseUsageAndEnrichCoverage(t *testing.T) {
	t.Parallel()

	state := &relayState{}
	parseUsageAndAccumulate(state, []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":"bad"}}}`), "response.completed", nil)
	require.Equal(t, 0, state.usage.InputTokens)

	parseUsageAndAccumulate(
		state,
		[]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":9,"output_tokens":"bad","input_tokens_details":{"cached_tokens":2}}}}`),
		"response.completed",
		nil,
	)
	require.Equal(t, 0, state.usage.InputTokens, "部分字段解析失败时不应累加 usage")
	require.Equal(t, 0, state.usage.OutputTokens)
	require.Equal(t, 0, state.usage.CacheReadInputTokens)

	parseUsageAndAccumulate(
		state,
		[]byte(`{"type":"response.completed","response":{"usage":{"input_tokens_details":{"cached_tokens":2}}}}`),
		"response.completed",
		nil,
	)
	require.Equal(t, 0, state.usage.InputTokens, "必填 usage 字段缺失时不应累加 usage")
	require.Equal(t, 0, state.usage.OutputTokens)
	require.Equal(t, 0, state.usage.CacheReadInputTokens)

	parseUsageAndAccumulate(state, []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":2,"output_tokens":1,"input_tokens_details":{"cached_tokens":1,"cache_write_tokens":4},"output_tokens_details":{"image_tokens":3}}}}`), "response.completed", nil)
	require.Equal(t, 2, state.usage.InputTokens)
	require.Equal(t, 1, state.usage.OutputTokens)
	require.Equal(t, 1, state.usage.CacheReadInputTokens)
	require.Equal(t, 4, state.usage.CacheCreationInputTokens)
	require.Equal(t, 3, state.usage.ImageOutputTokens)

	result := &RelayResult{}
	enrichResult(result, state, 5*time.Millisecond)
	require.Equal(t, state.usage.InputTokens, result.Usage.InputTokens)
	require.Equal(t, state.usage.CacheCreationInputTokens, result.Usage.CacheCreationInputTokens)
	require.Equal(t, state.usage.ImageOutputTokens, result.Usage.ImageOutputTokens)
	require.Equal(t, 5*time.Millisecond, result.Duration)
	parseUsageAndAccumulate(state, []byte(`{"type":"response.in_progress","response":{"usage":{"input_tokens":9}}}`), "response.in_progress", nil)
	require.Equal(t, 2, state.usage.InputTokens)
	enrichResult(nil, state, 0)
}

func TestParseUsageAndAccumulateIgnoresUnpinnedTerminalAlias(t *testing.T) {
	t.Parallel()

	state := &relayState{}
	got := parseUsageAndAccumulate(
		state,
		[]byte(`{"type":"response.done","response":{"usage":{"prompt_tokens":12,"completion_tokens":6,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"image_tokens":2}}}}`),
		"response.done",
		nil,
	)
	require.Zero(t, got.InputTokens)
	require.Zero(t, got.OutputTokens)
	require.Zero(t, got.CacheReadInputTokens)
	require.Zero(t, got.ImageOutputTokens)
	require.Equal(t, got, state.usage)
}

func TestOpenAICacheCreationTokensFromUsageNestedZeroWins(t *testing.T) {
	t.Parallel()

	usage := gjson.Parse(`{"input_tokens_details":{"cache_write_tokens":0},"cache_creation_input_tokens":19}`)
	require.Zero(t, openAICacheCreationTokensFromUsage(usage))
}

func TestEmitTurnCompleteCoverage(t *testing.T) {
	t.Parallel()

	// 非 terminal 事件不应触发。
	called := 0
	emitTurnComplete(func(turn RelayTurnResult) {
		called++
	}, &relayState{requestModel: "gpt-5"}, observedUpstreamEvent{
		terminal:   false,
		eventType:  "response.output_text.delta",
		responseID: "resp_ignored",
		usage:      Usage{InputTokens: 1},
	})
	require.Equal(t, 0, called)

	// 缺少 response_id 时不应触发。
	emitTurnComplete(func(turn RelayTurnResult) {
		called++
	}, &relayState{requestModel: "gpt-5"}, observedUpstreamEvent{
		terminal:  true,
		eventType: "response.completed",
	})
	require.Equal(t, 0, called)

	// terminal 且 response_id 存在，应该触发；state=nil 时 model 为空串。
	var got RelayTurnResult
	emitTurnComplete(func(turn RelayTurnResult) {
		called++
		got = turn
	}, nil, observedUpstreamEvent{
		terminal:   true,
		eventType:  "response.completed",
		responseID: "resp_emit",
		usage:      Usage{InputTokens: 2, OutputTokens: 3},
	})
	require.Equal(t, 1, called)
	require.Equal(t, "resp_emit", got.RequestID)
	require.Equal(t, "response.completed", got.TerminalEventType)
	require.Equal(t, 2, got.Usage.InputTokens)
	require.Equal(t, 3, got.Usage.OutputTokens)
	require.Equal(t, "", got.RequestModel)

	emitTurnComplete(func(turn RelayTurnResult) {
		called++
		got = turn
	}, &relayState{requestModel: "gpt-5"}, observedUpstreamEvent{
		terminal:  true,
		eventType: "response.failed",
		usage:     Usage{InputTokens: 1},
	})
	require.Equal(t, 2, called)
	require.Empty(t, got.RequestID)
	require.Equal(t, "response.failed", got.TerminalEventType)
	require.Equal(t, "gpt-5", got.RequestModel)
}

func TestMarkFirstUpstreamEventKeepsTurnTimingIndependent(t *testing.T) {
	t.Parallel()

	base := time.Unix(0, 0)
	firstTiming := &relayTurnTiming{startAt: base}
	secondTiming := &relayTurnTiming{startAt: base.Add(40 * time.Millisecond)}
	state := &relayState{}

	first := observedUpstreamEvent{
		eventType:     "response.created",
		turnStartedAt: firstTiming.startAt,
		turnTiming:    firstTiming,
	}
	markFirstUpstreamEvent(state, &first, base, base.Add(10*time.Millisecond))
	require.NotNil(t, firstTiming.firstByteMs)
	require.Equal(t, 10, *firstTiming.firstByteMs)

	second := observedUpstreamEvent{
		eventType:     "response.created",
		turnStartedAt: secondTiming.startAt,
		turnTiming:    secondTiming,
	}
	markFirstUpstreamEvent(state, &second, base, base.Add(50*time.Millisecond))
	require.NotNil(t, secondTiming.firstByteMs)
	require.Equal(t, 10, *secondTiming.firstByteMs)
	require.NotNil(t, state.firstByteMs)
	require.Equal(t, 10, *state.firstByteMs)

	markFirstUpstreamEvent(state, &second, base, base.Add(100*time.Millisecond))
	require.Equal(t, 10, *secondTiming.firstByteMs)
}

func TestRunUpstreamToClient_BinaryFirstFrameSetsTurnFirstByte(t *testing.T) {
	t.Parallel()

	base := time.Unix(100, 0)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageBinary, payload: []byte{0x00, 0x01}},
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.created","response":{"id":"resp_binary_first"}}`)},
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.completed","response":{"id":"resp_binary_first"}}`)},
	}, true)
	clientConn := newPassthroughTestFrameConn(nil, false)
	state := &relayState{}
	gate := newResponseStepGateWithModel("gpt-binary")
	gate.activeStartedAt = base
	drop := &atomic.Bool{}
	exitCh := make(chan relayExitSignal, 1)
	now := base
	nowFn := func() time.Time {
		now = now.Add(5 * time.Millisecond)
		return now
	}
	var turn RelayTurnResult

	runUpstreamToClient(
		context.Background(),
		upstreamConn,
		func(msgType coderws.MessageType, payload []byte) error {
			return clientConn.WriteFrame(context.Background(), msgType, payload)
		},
		base,
		nowFn,
		state,
		nil,
		func(current RelayTurnResult) { turn = current },
		nil,
		nil,
		drop,
		nil,
		nil,
		func() {},
		nil,
		gate,
		exitCh,
	)

	exit := <-exitCh
	require.Equal(t, "read_upstream", exit.stage)
	require.NotNil(t, turn.FirstByteMs)
	require.Equal(t, 5, *turn.FirstByteMs)
	require.Equal(t, 15*time.Millisecond, turn.Duration)
	require.Len(t, clientConn.Writes(), 3)
	require.Equal(t, coderws.MessageBinary, clientConn.Writes()[0].msgType)
}

func TestRunUpstreamToClient_TerminalFirstUsesUpstreamObservation(t *testing.T) {
	t.Parallel()

	base := time.Unix(100, 0)
	upstreamConn := newPassthroughTestFrameConn([]passthroughTestFrame{
		{msgType: coderws.MessageText, payload: []byte(`{"type":"response.failed","response":{"id":"resp_terminal_first"}}`)},
	}, true)
	state := &relayState{}
	gate := newResponseStepGateWithModel("gpt-terminal")
	gate.activeStartedAt = base
	drop := &atomic.Bool{}
	exitCh := make(chan relayExitSignal, 1)
	now := base
	nowFn := func() time.Time {
		now = now.Add(5 * time.Millisecond)
		return now
	}
	var turn RelayTurnResult

	runUpstreamToClient(
		context.Background(),
		upstreamConn,
		func(_ coderws.MessageType, _ []byte) error { return nil },
		base,
		nowFn,
		state,
		nil,
		func(current RelayTurnResult) { turn = current },
		nil,
		nil,
		drop,
		nil,
		nil,
		func() {},
		nil,
		gate,
		exitCh,
	)

	exit := <-exitCh
	require.Equal(t, "terminal_failure", exit.stage)
	require.NotNil(t, turn.FirstByteMs)
	require.Equal(t, 5, *turn.FirstByteMs)
	require.Equal(t, 5*time.Millisecond, turn.Duration)
	require.GreaterOrEqual(t, turn.Duration.Milliseconds(), int64(*turn.FirstByteMs))
}

func TestIsDisconnectErrorCoverage_CloseStatusesAndMessageBranches(t *testing.T) {
	t.Parallel()

	require.True(t, isDisconnectError(coderws.CloseError{Code: coderws.StatusNormalClosure}))
	require.True(t, isDisconnectError(coderws.CloseError{Code: coderws.StatusNoStatusRcvd}))
	require.True(t, isDisconnectError(coderws.CloseError{Code: coderws.StatusAbnormalClosure}))
	require.True(t, isDisconnectError(errors.New("connection reset by peer")))
	require.False(t, isDisconnectError(errors.New("   ")))
}

func TestIsTokenEventCoverageBranches(t *testing.T) {
	t.Parallel()

	require.False(t, isTokenEvent("response.in_progress"))
	require.False(t, isTokenEvent("response.output_item.added"))
	require.True(t, isTokenEvent("response.output_audio.delta"))
	require.True(t, isTokenEvent("response.output"))
	require.False(t, isTokenEvent("response.done"))
}

func TestShouldParseUsageTerminalEvents(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{
		"response.completed",
		"response.failed",
		"response.incomplete",
		"response.cancelled",
		"response.canceled",
	} {
		require.True(t, shouldParseUsage(eventType), eventType)
	}
	require.True(t, isTerminalEvent("error"))
	require.True(t, isFailureTerminalEvent("error"))
	require.False(t, shouldParseUsage("error"))
	require.False(t, shouldParseUsage("response.done"))
	require.True(t, isTerminalEvent("response.done"))
	require.False(t, shouldParseUsage("response.output_text.delta"))
	require.False(t, shouldParseUsage(""))
}

func TestRelayTurnTimingHelpersCoverage(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	// nil state
	require.Nil(t, openAIWSRelayGetOrInitTurnTiming(nil, "resp_nil", now))
	_, ok := openAIWSRelayDeleteTurnTiming(nil, "resp_nil")
	require.False(t, ok)

	state := &relayState{}
	timing := openAIWSRelayGetOrInitTurnTiming(state, "resp_a", now)
	require.NotNil(t, timing)
	require.Equal(t, now, timing.startAt)

	// 再次获取返回同一条 timing
	timing2 := openAIWSRelayGetOrInitTurnTiming(state, "resp_a", now.Add(5*time.Second))
	require.NotNil(t, timing2)
	require.Equal(t, now, timing2.startAt)

	// 删除存在键
	deleted, ok := openAIWSRelayDeleteTurnTiming(state, "resp_a")
	require.True(t, ok)
	require.Equal(t, now, deleted.startAt)

	// 删除不存在键
	_, ok = openAIWSRelayDeleteTurnTiming(state, "resp_a")
	require.False(t, ok)
}

func TestObserveUpstreamMessage_ResponseIDFallbackPolicy(t *testing.T) {
	t.Parallel()

	state := &relayState{requestModel: "gpt-5"}
	startAt := time.Unix(0, 0)
	now := startAt
	nowFn := func() time.Time {
		now = now.Add(5 * time.Millisecond)
		return now
	}

	// 非 terminal：仅有顶层 id，不应把 event id 当成 response_id。
	observed := observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.output_text.delta","id":"evt_123","delta":"hi"}`),
		startAt,
		nowFn,
		nil,
	)
	require.False(t, observed.terminal)
	require.Equal(t, "", observed.responseID)

	// terminal：允许兜底用顶层 id（用于兼容少数字段变体）。
	observed = observeUpstreamMessage(
		state,
		[]byte(`{"type":"response.completed","id":"resp_fallback","response":{"usage":{"input_tokens":1,"output_tokens":1}}}`),
		startAt,
		nowFn,
		nil,
	)
	require.True(t, observed.terminal)
	require.Equal(t, "resp_fallback", observed.responseID)
}
