package queue

import (
	"context"
	"errors"
	"sync"
	"time"
)

// DeadJob é um job que foi para a DLQ.
type DeadJob struct {
	Job    Job
	Reason string
}

// Memory implementa Queue em memória, para testes, com a semântica que importa
// do RabbitMQ: ack manual, reentrega do que não foi confirmado e DLQ.
type Memory struct {
	mu        sync.Mutex
	ready     []*memoryMessage
	dead      []DeadJob
	delays    []time.Duration
	available bool
	wake      chan struct{}
}

type memoryMessage struct {
	job         Job
	redelivered bool
}

func NewMemory() *Memory {
	return &Memory{available: true, wake: make(chan struct{}, 1)}
}

// SetAvailable simula o broker fora do ar.
func (m *Memory) SetAvailable(v bool) {
	m.mu.Lock()
	m.available = v
	m.mu.Unlock()
}

// Dead devolve os jobs na DLQ.
func (m *Memory) Dead() []DeadJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]DeadJob(nil), m.dead...)
}

// Delays devolve os atrasos pedidos em PublishDelayed, em ordem.
func (m *Memory) Delays() []time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]time.Duration(nil), m.delays...)
}

// Ready devolve quantas mensagens aguardam um worker.
func (m *Memory) Ready() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.ready)
}

func (m *Memory) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Memory) push(msg *memoryMessage, front bool) {
	m.mu.Lock()
	if front {
		m.ready = append([]*memoryMessage{msg}, m.ready...)
	} else {
		m.ready = append(m.ready, msg)
	}
	m.mu.Unlock()
	m.signal()
}

func (m *Memory) pop() *memoryMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.ready) == 0 {
		return nil
	}
	msg := m.ready[0]
	m.ready = m.ready[1:]
	return msg
}

func (m *Memory) isAvailable() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.available
}

func (m *Memory) Publish(ctx context.Context, job Job) error {
	if !m.isAvailable() {
		return ErrUnavailable
	}
	m.push(&memoryMessage{job: job}, false)
	return nil
}

func (m *Memory) PublishDelayed(ctx context.Context, job Job, delay time.Duration) error {
	if !m.isAvailable() {
		return ErrUnavailable
	}
	m.mu.Lock()
	m.delays = append(m.delays, delay)
	m.mu.Unlock()
	time.AfterFunc(delay, func() { m.push(&memoryMessage{job: job}, false) })
	return nil
}

func (m *Memory) DeadLetter(ctx context.Context, job Job, reason string) error {
	if !m.isAvailable() {
		return ErrUnavailable
	}
	m.mu.Lock()
	m.dead = append(m.dead, DeadJob{Job: job, Reason: reason})
	m.mu.Unlock()
	return nil
}

// Consume entrega mensagens até ctx terminar. Ao terminar, o que não foi
// confirmado volta para a fila na hora, mesmo que o handler ainda esteja
// rodando, como acontece quando a conexão de um worker morto cai.
func (m *Memory) Consume(ctx context.Context, prefetch int, handle Handler) error {
	slots := make(chan struct{}, prefetch)
	var mu sync.Mutex
	unacked := make(map[*memoryMessage]bool)

	settle := func(msg *memoryMessage) bool {
		mu.Lock()
		defer mu.Unlock()
		if !unacked[msg] {
			return false
		}
		delete(unacked, msg)
		return true
	}
	errSettled := errors.New("entrega já encerrada ou consumidor desconectado")

	defer func() {
		mu.Lock()
		pending := make([]*memoryMessage, 0, len(unacked))
		for msg := range unacked {
			pending = append(pending, msg)
		}
		mu.Unlock()
		for _, msg := range pending {
			if settle(msg) {
				m.push(&memoryMessage{job: msg.job, redelivered: true}, true)
			}
		}
	}()

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case slots <- struct{}{}:
		}

		var msg *memoryMessage
		for msg == nil {
			if msg = m.pop(); msg != nil {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-m.wake:
			case <-ticker.C:
			}
		}

		mu.Lock()
		unacked[msg] = true
		mu.Unlock()

		current := msg
		d := &Delivery{
			Job:         current.job,
			Redelivered: current.redelivered,
			ack: func() error {
				if !settle(current) {
					return errSettled
				}
				return nil
			},
			nack: func() error {
				if !settle(current) {
					return errSettled
				}
				m.push(&memoryMessage{job: current.job, redelivered: true}, true)
				return nil
			},
		}
		go func() {
			defer func() { <-slots }()
			handle(ctx, d)
		}()
	}
}
