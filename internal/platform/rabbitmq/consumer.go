package rabbitmq

import (
	"context"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const (
	initialBackoff = time.Second
	maxBackoff     = 30 * time.Second
)

// RunResilientConsumer maintains a RabbitMQ consumer across connection
// drops: it dials, declares the queue and starts consuming; if the
// connection or channel is lost, it backs off (capped exponential) and
// retries indefinitely, re-declaring and re-consuming once reconnected.
// Callers see a single, stable delivery channel (out) — reconnects are
// invisible to them. This is what makes worker.Pool oblivious to RabbitMQ
// outages entirely: it just reads from a channel that happens to keep
// producing deliveries across restarts of the broker.
//
// Call this in its own goroutine. It blocks until ctx is canceled, at which
// point it gracefully cancels the AMQP consumer, drains whatever deliveries
// were already in flight into out (so a message that arrived right before
// shutdown isn't lost), closes the channel/connection, closes out, and
// returns — the caller can wait for out to close (or for a downstream
// consumer of out, like worker.Pool's WaitGroup, to finish) to know
// shutdown completed.
func RunResilientConsumer(ctx context.Context, url, queue, tag string, log *zap.Logger, out chan<- amqp.Delivery) {
	defer close(out)

	backoff := initialBackoff
	for {
		if ctx.Err() != nil {
			return
		}

		conn, ch, deliveries, err := connectAndConsume(url, queue, tag)
		if err != nil {
			log.Error("rabbitmq consumer: connect failed, retrying", zap.Error(err), zap.Duration("backoff", backoff))
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		log.Info("rabbitmq consumer connected")
		backoff = initialBackoff

		connectionLost := forward(ctx, ch, tag, deliveries, out)
		_ = ch.Close()
		_ = conn.Close()

		if !connectionLost {
			return // ctx was canceled; forward already drained and shut down cleanly
		}
		log.Error("rabbitmq consumer: connection lost, reconnecting")
	}
}

func connectAndConsume(url, queue, tag string) (*amqp.Connection, *amqp.Channel, <-chan amqp.Delivery, error) {
	conn, ch, err := Dial(url)
	if err != nil {
		return nil, nil, nil, err
	}

	if _, err := DeclareQueue(ch, queue, DefaultDeliveryLimit); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, nil, nil, err
	}

	deliveries, err := ch.Consume(queue, tag, false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, nil, nil, err
	}

	return conn, ch, deliveries, nil
}

// forward relays deliveries from in to out until either ctx is canceled
// (returns false, after gracefully canceling the consumer and draining
// whatever was already buffered) or in closes on its own because the
// channel/connection dropped (returns true, telling the caller to reconnect).
func forward(ctx context.Context, ch *amqp.Channel, tag string, in <-chan amqp.Delivery, out chan<- amqp.Delivery) (connectionLost bool) {
	for {
		select {
		case <-ctx.Done():
			cancelAndDrain(ch, tag, in, out)
			return false

		case d, ok := <-in:
			if !ok {
				return true
			}
			select {
			case out <- d:
			case <-ctx.Done():
				_ = d.Nack(false, true) // shutting down before this could be forwarded: put it back for whoever picks the queue up next
				cancelAndDrain(ch, tag, in, out)
				return false
			}
		}
	}
}

// cancelAndDrain stops the broker from sending further deliveries, then
// forwards whatever was already buffered on in before it closes.
func cancelAndDrain(ch *amqp.Channel, tag string, in <-chan amqp.Delivery, out chan<- amqp.Delivery) {
	_ = ch.Cancel(tag, false)
	for d := range in {
		out <- d
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

func nextBackoff(cur time.Duration) time.Duration {
	cur *= 2
	if cur > maxBackoff {
		return maxBackoff
	}
	return cur
}
