package publisher

import (
	"context"
	"encoding/json"
	"fmt"

	"URLShortener/internal/stats"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ClickPublisher publishes click events onto a RabbitMQ queue. It is used
// fire-and-forget from the Gateway's redirect handler — see
// transport/http/link.go — so a slow or unavailable broker never delays the
// redirect response itself.
type ClickPublisher struct {
	ch    *amqp.Channel
	queue string
}

func New(ch *amqp.Channel, queue string) *ClickPublisher {
	return &ClickPublisher{ch: ch, queue: queue}
}

func (p *ClickPublisher) Publish(ctx context.Context, ev stats.ClickEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("publisher: marshal click event: %w", err)
	}

	err = p.ch.PublishWithContext(ctx,
		"",      // default exchange: routes directly to the queue named by routing key
		p.queue, // routing key = queue name
		false,   // mandatory
		false,   // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // survive a broker restart, matching the durable queue
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("publisher: publish: %w", err)
	}
	return nil
}
