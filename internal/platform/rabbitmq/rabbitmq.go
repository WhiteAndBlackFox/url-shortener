package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DefaultDeliveryLimit bounds how many times RabbitMQ will redeliver a
// nacked-and-requeued message before routing it to the dead-letter queue
// instead — see DeclareQueue. Without this, a permanent failure (e.g.
// Postgres unreachable) turns worker.Pool's Nack(requeue=true) into an
// unbounded tight loop: the same batch fails, gets requeued, gets
// redelivered immediately, fails again, forever.
const DefaultDeliveryLimit = 5

// Dial opens a connection and a channel to RabbitMQ.
func Dial(url string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("rabbitmq: open channel: %w", err)
	}

	return conn, ch, nil
}

// DeclareQueue declares "name" as a durable quorum queue bounded to
// deliveryLimit redeliveries, dead-lettering to "<name>.dlx"/"<name>.dlq"
// once that limit is exceeded. Quorum queues (unlike classic queues)
// natively track each message's delivery count and enforce the limit
// server-side — worker.Pool's Nack(requeue=true) on a write failure doesn't
// need to change at all; RabbitMQ itself stops the redelivery loop once a
// message has genuinely failed deliveryLimit times, moving it to the DLQ
// for manual inspection instead of spinning forever.
//
// Safe to call from both the producer (Gateway) and the consumer (Stat
// Service) side, as long as both pass the same deliveryLimit — RabbitMQ
// treats redeclaring a queue with identical arguments as a no-op, but
// errors if the arguments differ from what's already declared.
func DeclareQueue(ch *amqp.Channel, name string, deliveryLimit int) (amqp.Queue, error) {
	dlxName := name + ".dlx"
	dlqName := name + ".dlq"

	if err := ch.ExchangeDeclare(dlxName, "fanout", true, false, false, false, nil); err != nil {
		return amqp.Queue{}, fmt.Errorf("rabbitmq: declare dead-letter exchange %q: %w", dlxName, err)
	}

	dlq, err := ch.QueueDeclare(dlqName, true, false, false, false, nil)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("rabbitmq: declare dead-letter queue %q: %w", dlqName, err)
	}

	if err := ch.QueueBind(dlq.Name, "", dlxName, false, nil); err != nil {
		return amqp.Queue{}, fmt.Errorf("rabbitmq: bind dead-letter queue %q to %q: %w", dlqName, dlxName, err)
	}

	q, err := ch.QueueDeclare(name, true, false, false, false, amqp.Table{
		"x-queue-type":           "quorum",
		"x-delivery-limit":       int32(deliveryLimit),
		"x-dead-letter-exchange": dlxName,
	})
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("rabbitmq: declare queue %q: %w", name, err)
	}
	return q, nil
}
