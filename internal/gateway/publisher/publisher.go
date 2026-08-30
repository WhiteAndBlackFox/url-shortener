package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"URLShortener/internal/platform/rabbitmq"
	"URLShortener/internal/platform/requestid"
	"URLShortener/internal/stats"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ClickPublisher publishes click events onto a RabbitMQ queue. It is used
// fire-and-forget from the Gateway's redirect handler — see
// transport/http/link.go — so a slow or unavailable broker never delays the
// redirect response itself.
//
// Unlike the Stat Service side (internal/platform/rabbitmq.RunResilientConsumer,
// which needs a continuously-live subscription and so watches for and
// repairs disconnects proactively in the background), publishing is a
// sporadic, one-off action — so ClickPublisher instead reconnects lazily:
// it owns its connection and only re-dials when a publish actually fails,
// retrying that one event once before giving up on it.
type ClickPublisher struct {
	url   string
	queue string

	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

// New dials RabbitMQ and declares the queue up front, so misconfiguration
// (bad URL, unreachable broker) fails fast at startup like every other
// dependency this project connects to.
func New(url, queue string) (*ClickPublisher, error) {
	p := &ClickPublisher{url: url, queue: queue}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *ClickPublisher) connect() error {
	conn, ch, err := rabbitmq.Dial(p.url)
	if err != nil {
		return err
	}
	if _, err := rabbitmq.DeclareQueue(ch, p.queue, rabbitmq.DefaultDeliveryLimit); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	p.mu.Lock()
	// Replacing (not leaking) any previous connection: connect() is also
	// called from Publish's reconnect-on-failure path, where the old
	// connection is already dead but not yet explicitly closed.
	oldConn := p.conn
	p.conn, p.ch = conn, ch
	p.mu.Unlock()

	if oldConn != nil {
		_ = oldConn.Close()
	}
	return nil
}

func (p *ClickPublisher) Publish(ctx context.Context, ev stats.ClickEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("publisher: marshal click event: %w", err)
	}

	publishing := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent, // survive a broker restart, matching the durable queue
		Body:         body,
	}
	if id := requestid.FromContext(ctx); id != "" {
		publishing.Headers = amqp.Table{requestid.Key: id}
	}

	if err := p.publishOnce(ctx, publishing); err == nil {
		return nil
	}

	// The channel/connection may have died since the last successful
	// publish (RabbitMQ restart, network blip). Publish is already
	// best-effort fire-and-forget from the caller's perspective, so one
	// reconnect-and-retry is enough before giving up on this one event.
	if err := p.connect(); err != nil {
		return fmt.Errorf("publisher: reconnect: %w", err)
	}
	return p.publishOnce(ctx, publishing)
}

func (p *ClickPublisher) publishOnce(ctx context.Context, publishing amqp.Publishing) error {
	p.mu.Lock()
	ch := p.ch
	p.mu.Unlock()

	if ch == nil {
		return errors.New("publisher: not connected")
	}

	err := ch.PublishWithContext(ctx,
		"",      // default exchange: routes directly to the queue named by routing key
		p.queue, // routing key = queue name
		false,   // mandatory
		false,   // immediate
		publishing,
	)
	if err != nil {
		return fmt.Errorf("publisher: publish: %w", err)
	}
	return nil
}

// Close releases the underlying connection. Safe to call once during
// graceful shutdown, after the caller is sure no more Publish calls will
// arrive (see Handler.WaitPublishers in transport/http/link.go).
func (p *ClickPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
