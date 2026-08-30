package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"URLShortener/internal/platform/requestid"
	"URLShortener/internal/stats"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// Pool is a fixed-size worker pool that fans out from a single RabbitMQ
// delivery channel: every worker goroutine competes for deliveries on the
// same channel (a standard Go fan-out), decodes them, and accumulates its
// own local batch. A batch is flushed — one multi-row INSERT — whenever it
// reaches batchSize, or flushInterval elapses since the last flush,
// whichever comes first. Messages are only acked after their batch has been
// durably written, giving at-least-once delivery: a crash between receiving
// and flushing just means RabbitMQ redelivers the message later.
type Pool struct {
	workers       int
	batchSize     int
	flushInterval time.Duration
	recorder      Recorder
	log           *zap.Logger
}

// Recorder is the subset of stats.Service that the pool needs — kept
// narrow (consumer defines the interface) so the pool can be unit-tested
// against a fake without pulling in stats.Repository too.
type Recorder interface {
	RecordClicks(ctx context.Context, events []stats.ClickEvent) error
}

func NewPool(workers, batchSize int, flushInterval time.Duration, recorder Recorder, log *zap.Logger) *Pool {
	return &Pool{
		workers:       workers,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		recorder:      recorder,
		log:           log,
	}
}

// Start spawns the worker goroutines and returns immediately; the returned
// WaitGroup is done once every worker has drained and exited — which
// happens when deliveries is closed (the caller closes the RabbitMQ
// channel/connection on shutdown, which closes this channel).
func (p *Pool) Start(deliveries <-chan amqp.Delivery) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	for i := 0; i < p.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.run(deliveries)
		}()
	}
	return wg
}

type pending struct {
	event    stats.ClickEvent
	delivery amqp.Delivery
}

func (p *Pool) run(deliveries <-chan amqp.Delivery) {
	// A worker runs detached from any request-scoped recovery middleware, so
	// without this, a bug triggered by one bad message would crash the whole
	// process instead of just costing this one worker (the rest of the pool
	// keeps running; that degradation is loud in the logs, not silent).
	defer func() {
		if rec := recover(); rec != nil {
			p.log.Error("panic in worker, this worker is exiting", zap.Any("panic", rec))
		}
	}()

	batch := make([]pending, 0, p.batchSize)
	ticker := time.NewTicker(p.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case d, ok := <-deliveries:
			if !ok {
				p.flush(batch)
				return
			}

			var ev stats.ClickEvent
			if err := json.Unmarshal(d.Body, &ev); err != nil {
				p.log.Error("discarding malformed click event",
					zap.String("request_id", headerRequestID(d)),
					zap.Error(err),
				)
				_ = d.Nack(false, false) // don't requeue: it will never decode successfully
				continue
			}

			batch = append(batch, pending{event: ev, delivery: d})
			if len(batch) >= p.batchSize {
				batch = p.flush(batch)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				batch = p.flush(batch)
			}
		}
	}
}

// headerRequestID reads the x-request-id header Gateway attaches when
// publishing (see internal/gateway/publisher) off a single delivery. Only
// used for the per-message decode-failure log: once messages are grouped
// into a batch they typically span multiple unrelated requests, so a single
// "request_id" field stops being a meaningful thing to log per-batch.
func headerRequestID(d amqp.Delivery) string {
	v, _ := d.Headers[requestid.Key].(string)
	return v
}

// flush writes the batch to Postgres and acks/nacks every delivery in it
// accordingly, then returns an empty slice (reusing the backing array).
func (p *Pool) flush(batch []pending) []pending {
	if len(batch) == 0 {
		return batch
	}

	events := make([]stats.ClickEvent, len(batch))
	for i, item := range batch {
		events[i] = item.event
	}

	if err := p.recordClicks(events); err != nil {
		p.log.Error("batch write failed, requeueing", zap.Int("batch_size", len(batch)), zap.Error(err))
		for _, item := range batch {
			_ = item.delivery.Nack(false, true) // requeue: transient DB error, worth retrying
		}
		return batch[:0]
	}

	for _, item := range batch {
		_ = item.delivery.Ack(false)
	}
	p.log.Debug("flushed click batch", zap.Int("batch_size", len(batch)))
	return batch[:0]
}

// recordClicks calls the recorder with a bounded timeout, converting a
// panic (e.g. a driver/reflection bug) into a plain error so flush's
// nack-and-requeue path handles it exactly like any other write failure.
// Without this, a panic here would unwind past the ack/nack loop entirely,
// leaving the batch's deliveries permanently unacknowledged on the channel
// even though run()'s outer recover() stops the process from crashing.
func (p *Pool) recordClicks(events []stats.ClickEvent) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic: %v", rec)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.recorder.RecordClicks(ctx, events)
}
