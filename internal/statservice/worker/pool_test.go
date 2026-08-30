package worker_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"URLShortener/internal/stats"
	"URLShortener/internal/statservice/worker"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeAcknowledger implements amqp.Acknowledger without a real broker
// connection, so amqp.Delivery values can be constructed directly in tests.
type fakeAcknowledger struct {
	mu      sync.Mutex
	acked   []uint64
	nacked  []uint64
	requeue []bool
}

func (f *fakeAcknowledger) Ack(tag uint64, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acked = append(f.acked, tag)
	return nil
}

func (f *fakeAcknowledger) Nack(tag uint64, _ bool, requeue bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nacked = append(f.nacked, tag)
	f.requeue = append(f.requeue, requeue)
	return nil
}

func (f *fakeAcknowledger) Reject(tag uint64, requeue bool) error {
	return f.Nack(tag, false, requeue)
}

func (f *fakeAcknowledger) counts() (acked, nacked int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.acked), len(f.nacked)
}

func newDelivery(t *testing.T, ack *fakeAcknowledger, tag uint64, ev stats.ClickEvent) amqp.Delivery {
	t.Helper()
	body, err := json.Marshal(ev)
	require.NoError(t, err)
	return amqp.Delivery{Acknowledger: ack, DeliveryTag: tag, Body: body}
}

// fakeRecorder records every batch it was asked to write, optionally
// failing the first N calls to exercise the requeue-on-error path.
type fakeRecorder struct {
	mu         sync.Mutex
	batches    [][]stats.ClickEvent
	failCalls  int
	callsSoFar int
}

func (r *fakeRecorder) RecordClicks(_ context.Context, events []stats.ClickEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callsSoFar++
	if r.callsSoFar <= r.failCalls {
		return context.DeadlineExceeded
	}
	batch := make([]stats.ClickEvent, len(events))
	copy(batch, events)
	r.batches = append(r.batches, batch)
	return nil
}

func (r *fakeRecorder) totalRecorded() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, b := range r.batches {
		n += len(b)
	}
	return n
}

func TestPool_FlushesOnBatchSize(t *testing.T) {
	recorder := &fakeRecorder{}
	pool := worker.NewPool(1, 3, time.Hour, recorder, zap.NewNop()) // long interval: only size-based flush should fire

	deliveries := make(chan amqp.Delivery)
	wg := pool.Start(deliveries)

	ack := &fakeAcknowledger{}
	for i := uint64(1); i <= 3; i++ {
		deliveries <- newDelivery(t, ack, i, stats.ClickEvent{Code: "abc1234", OccurredAt: time.Now()})
	}

	require.Eventually(t, func() bool {
		acked, _ := ack.counts()
		return acked == 3
	}, time.Second, 10*time.Millisecond)

	close(deliveries)
	wg.Wait()

	require.Equal(t, 3, recorder.totalRecorded())
}

func TestPool_FlushesOnInterval(t *testing.T) {
	recorder := &fakeRecorder{}
	pool := worker.NewPool(1, 100, 20*time.Millisecond, recorder, zap.NewNop()) // huge batch size: only interval-based flush should fire

	deliveries := make(chan amqp.Delivery)
	wg := pool.Start(deliveries)

	ack := &fakeAcknowledger{}
	deliveries <- newDelivery(t, ack, 1, stats.ClickEvent{Code: "abc1234", OccurredAt: time.Now()})

	require.Eventually(t, func() bool {
		acked, _ := ack.counts()
		return acked == 1
	}, time.Second, 10*time.Millisecond)

	close(deliveries)
	wg.Wait()

	require.Equal(t, 1, recorder.totalRecorded())
}

func TestPool_NacksAndRequeuesOnWriteFailure(t *testing.T) {
	recorder := &fakeRecorder{failCalls: 1} // first flush attempt fails, second succeeds
	pool := worker.NewPool(1, 1, time.Hour, recorder, zap.NewNop())

	deliveries := make(chan amqp.Delivery)
	wg := pool.Start(deliveries)

	ack := &fakeAcknowledger{}
	deliveries <- newDelivery(t, ack, 1, stats.ClickEvent{Code: "abc1234", OccurredAt: time.Now()})

	require.Eventually(t, func() bool {
		_, nacked := ack.counts()
		return nacked == 1
	}, time.Second, 10*time.Millisecond)

	ack.mu.Lock()
	require.True(t, ack.requeue[0], "a transient write failure must requeue, not drop, the message")
	ack.mu.Unlock()

	close(deliveries)
	wg.Wait()
}

// panicThenSucceedRecorder simulates an unexpected bug in the write path
// (as opposed to an ordinary error) on its first call, then behaves
// normally — proving both that a panic can't crash the whole statservice
// process, and that it doesn't leak the batch's deliveries or permanently
// kill the worker either: the worker must nack-and-requeue the panicking
// batch and keep processing later ones.
type panicThenSucceedRecorder struct {
	mu         sync.Mutex
	panicCalls int
	callsSoFar int
	batches    [][]stats.ClickEvent
}

func (r *panicThenSucceedRecorder) RecordClicks(_ context.Context, events []stats.ClickEvent) error {
	r.mu.Lock()
	r.callsSoFar++
	shouldPanic := r.callsSoFar <= r.panicCalls
	r.mu.Unlock()

	if shouldPanic {
		panic("boom")
	}

	r.mu.Lock()
	batch := make([]stats.ClickEvent, len(events))
	copy(batch, events)
	r.batches = append(r.batches, batch)
	r.mu.Unlock()
	return nil
}

func TestPool_SurvivesPanicInRecorder(t *testing.T) {
	recorder := &panicThenSucceedRecorder{panicCalls: 1}
	pool := worker.NewPool(1, 1, time.Hour, recorder, zap.NewNop())

	deliveries := make(chan amqp.Delivery)
	wg := pool.Start(deliveries)

	// First delivery: the recorder panics while handling it. If the panic
	// weren't recovered close to the point of failure, it would either crash
	// this whole test binary (an unrecovered goroutine panic takes down the
	// process) or leak this delivery unacknowledged forever.
	ack1 := &fakeAcknowledger{}
	deliveries <- newDelivery(t, ack1, 1, stats.ClickEvent{Code: "abc1234", OccurredAt: time.Now()})

	require.Eventually(t, func() bool {
		_, nacked := ack1.counts()
		return nacked == 1
	}, time.Second, 10*time.Millisecond, "the panicking batch's delivery must be nacked, not leaked")

	ack1.mu.Lock()
	require.True(t, ack1.requeue[0], "a panic is treated like any other transient write failure: requeue it")
	ack1.mu.Unlock()

	// Second delivery, sent after the panic: proves the worker is still
	// alive and processing, not just that the process didn't crash.
	ack2 := &fakeAcknowledger{}
	deliveries <- newDelivery(t, ack2, 2, stats.ClickEvent{Code: "def5678", OccurredAt: time.Now()})

	require.Eventually(t, func() bool {
		acked, _ := ack2.counts()
		return acked == 1
	}, time.Second, 10*time.Millisecond, "the worker must keep processing after recovering from a panic")

	close(deliveries)
	wg.Wait()
}

func TestPool_DiscardsMalformedMessageWithoutRequeue(t *testing.T) {
	recorder := &fakeRecorder{}
	pool := worker.NewPool(1, 1, time.Hour, recorder, zap.NewNop())

	deliveries := make(chan amqp.Delivery)
	wg := pool.Start(deliveries)

	ack := &fakeAcknowledger{}
	deliveries <- amqp.Delivery{Acknowledger: ack, DeliveryTag: 1, Body: []byte("not json")}

	require.Eventually(t, func() bool {
		_, nacked := ack.counts()
		return nacked == 1
	}, time.Second, 10*time.Millisecond)

	ack.mu.Lock()
	require.False(t, ack.requeue[0], "a poison message must not be requeued, or it would loop forever")
	ack.mu.Unlock()

	close(deliveries)
	wg.Wait()
	require.Equal(t, 0, recorder.totalRecorded())
}
