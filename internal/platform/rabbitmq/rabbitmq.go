package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

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

// DeclareQueue declares a durable queue. Safe to call from both the
// producer (Gateway) and the consumer (Stat Service) side — RabbitMQ treats
// redeclaring a queue with identical parameters as a no-op, so whichever
// service starts first creates it and the other just confirms it matches.
func DeclareQueue(ch *amqp.Channel, name string) (amqp.Queue, error) {
	q, err := ch.QueueDeclare(
		name,
		true,  // durable: survives a broker restart
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // args
	)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("rabbitmq: declare queue %q: %w", name, err)
	}
	return q, nil
}
