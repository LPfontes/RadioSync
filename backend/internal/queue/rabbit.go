package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	dialTimeout    = 5 * time.Second
	confirmTimeout = 5 * time.Second
	// deliveryLimit protege de mensagens que derrubam o worker: depois desse
	// número de reentregas sem ack a fila manda a mensagem para a DLQ.
	deliveryLimit = 5
)

// URLFromEnv lê AMQP_URL.
func URLFromEnv() string {
	if u := os.Getenv("AMQP_URL"); u != "" {
		return u
	}
	return "amqp://radiosync:radiosync@localhost:5672/"
}

// Rabbit implementa Queue no RabbitMQ.
//
// Topologia (tudo durável, pelo exchange padrão):
//
//	media.transcode            fila quorum; dead-letter para media.transcode.dlq
//	media.transcode.retry.<ms> TTL = atraso; dead-letter para media.transcode
//	media.transcode.dlq        jobs que esgotaram as tentativas
type Rabbit struct {
	url    string
	delays []time.Duration

	mu      sync.Mutex
	conn    *amqp.Connection
	publish *amqp.Channel
	closed  bool
}

// NewRabbit não conecta: a conexão é aberta no primeiro uso e refeita se cair.
// delays são os atrasos que PublishDelayed aceita (uma fila de retry por atraso).
func NewRabbit(url string, delays ...time.Duration) *Rabbit {
	return &Rabbit{url: url, delays: delays}
}

func retryQueueName(d time.Duration) string {
	return RetryQueuePrefix + strconv.FormatInt(d.Milliseconds(), 10)
}

func (r *Rabbit) declare(ch *amqp.Channel) error {
	if _, err := ch.QueueDeclare(DeadLetterQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declarar %s: %w", DeadLetterQueue, err)
	}
	if _, err := ch.QueueDeclare(TranscodeQueue, true, false, false, false, amqp.Table{
		"x-queue-type":              "quorum",
		"x-delivery-limit":          int32(deliveryLimit),
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": DeadLetterQueue,
	}); err != nil {
		return fmt.Errorf("declarar %s: %w", TranscodeQueue, err)
	}
	for _, d := range r.delays {
		name := retryQueueName(d)
		if _, err := ch.QueueDeclare(name, true, false, false, false, amqp.Table{
			"x-message-ttl":             d.Milliseconds(),
			"x-dead-letter-exchange":    "",
			"x-dead-letter-routing-key": TranscodeQueue,
		}); err != nil {
			return fmt.Errorf("declarar %s: %w", name, err)
		}
	}
	return nil
}

// channelLocked abre um canal novo, conectando e declarando a topologia se preciso.
// Chamar com o lock.
func (r *Rabbit) channelLocked() (*amqp.Channel, error) {
	if r.closed {
		return nil, ErrUnavailable
	}
	if r.conn == nil || r.conn.IsClosed() {
		conn, err := amqp.DialConfig(r.url, amqp.Config{
			Heartbeat: 10 * time.Second,
			Dial:      amqp.DefaultDial(dialTimeout),
		})
		if err != nil {
			return nil, fmt.Errorf("%w: conectar no RabbitMQ: %v", ErrUnavailable, err)
		}
		ch, err := conn.Channel()
		if err == nil {
			err = r.declare(ch)
			_ = ch.Close()
		}
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		r.conn, r.publish = conn, nil
	}
	ch, err := r.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("%w: abrir canal: %v", ErrUnavailable, err)
	}
	return ch, nil
}

func (r *Rabbit) openChannel() (*amqp.Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.channelLocked()
}

func (r *Rabbit) publishChannel() (*amqp.Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.publish != nil && !r.publish.IsClosed() {
		return r.publish, nil
	}
	ch, err := r.channelLocked()
	if err != nil {
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("%w: publisher confirms: %v", ErrUnavailable, err)
	}
	r.publish = ch
	return ch, nil
}

// send publica com publisher confirm: só retorna nil quando o broker gravou.
func (r *Rabbit) send(ctx context.Context, queue string, job Job, headers amqp.Table) error {
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	ch, err := r.publishChannel()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, confirmTimeout)
	defer cancel()

	confirm, err := ch.PublishWithDeferredConfirmWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    job.ID,
		Timestamp:    time.Now(),
		Headers:      headers,
		Body:         body,
	})
	if err != nil {
		return fmt.Errorf("%w: publicar em %s: %v", ErrUnavailable, queue, err)
	}
	acked, err := confirm.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("%w: sem confirmação de %s: %v", ErrUnavailable, queue, err)
	}
	if !acked {
		return fmt.Errorf("%w: %s recusou a mensagem", ErrUnavailable, queue)
	}
	return nil
}

func (r *Rabbit) Publish(ctx context.Context, job Job) error {
	return r.send(ctx, TranscodeQueue, job, nil)
}

func (r *Rabbit) PublishDelayed(ctx context.Context, job Job, delay time.Duration) error {
	for _, d := range r.delays {
		if d == delay {
			return r.send(ctx, retryQueueName(d), job, nil)
		}
	}
	return fmt.Errorf("não há fila de retry para o atraso %v", delay)
}

func (r *Rabbit) DeadLetter(ctx context.Context, job Job, reason string) error {
	return r.send(ctx, DeadLetterQueue, job, amqp.Table{
		"x-error":    reason,
		"x-attempts": int32(job.Attempt + 1), // #nosec G115 -- limitado pela política de retry
	})
}

// Connect conecta e declara a topologia, para falhar cedo na subida.
func (r *Rabbit) Connect() error {
	ch, err := r.openChannel()
	if err != nil {
		return err
	}
	return ch.Close()
}

// Connected informa se há conexão com o broker.
func (r *Rabbit) Connected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.conn != nil && !r.conn.IsClosed()
}

// Depths devolve quantas mensagens estão prontas em cada fila.
func (r *Rabbit) Depths() (map[string]int, error) {
	ch, err := r.openChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	names := []string{TranscodeQueue, DeadLetterQueue}
	for _, d := range r.delays {
		names = append(names, retryQueueName(d))
	}
	depths := make(map[string]int, len(names))
	for _, name := range names {
		q, err := ch.QueueDeclarePassive(name, true, false, false, false, nil)
		if err != nil {
			return nil, fmt.Errorf("consultar %s: %w", name, err)
		}
		depths[name] = q.Messages
	}
	return depths, nil
}

func (r *Rabbit) Consume(ctx context.Context, prefetch int, handle Handler) error {
	backoff := 500 * time.Millisecond
	for ctx.Err() == nil {
		started := time.Now()
		err := r.consumeSession(ctx, prefetch, handle)
		if ctx.Err() != nil {
			break
		}
		if time.Since(started) > time.Minute {
			backoff = 500 * time.Millisecond
		}
		log.Printf("[fila] consumidor desconectado (%v); reconectando em %v", err, backoff)
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
	return ctx.Err()
}

// consumeSession consome até ctx terminar ou o canal cair. Nos dois casos os
// handlers em andamento são cancelados: sem canal o ack deles não vale mais, e
// o RabbitMQ já devolveu as mensagens para a fila.
func (r *Rabbit) consumeSession(ctx context.Context, prefetch int, handle Handler) error {
	ch, err := r.openChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.Qos(prefetch, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(TranscodeQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	// Ordem: cancela os handlers, espera eles soltarem as entregas e só então
	// fecha o canal (defer acima).
	defer wg.Wait()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-deliveries:
			if !ok {
				return ErrUnavailable
			}
			var job Job
			if err := json.Unmarshal(msg.Body, &job); err != nil {
				log.Printf("[fila] mensagem ilegível enviada para a DLQ: %v", err)
				_ = msg.Reject(false)
				continue
			}
			d := &Delivery{
				Job:         job,
				Redelivered: msg.Redelivered,
				ack:         func() error { return msg.Ack(false) },
				nack:        func() error { return msg.Nack(false, true) },
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				handle(sessionCtx, d)
			}()
		}
	}
}

func (r *Rabbit) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.conn == nil || r.conn.IsClosed() {
		return nil
	}
	return r.conn.Close()
}
