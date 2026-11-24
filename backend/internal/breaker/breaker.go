// Package breaker implementa o Circuit Breaker usado nas chamadas ao YouTube
// (ADR-0002). Máquina de estados:
//
//	closed    -> open       N falhas seguidas
//	open      -> half-open  passou o cooldown e chegou uma chamada
//	half-open -> closed     a chamada de teste deu certo
//	half-open -> open       a chamada de teste falhou
//
// Em open as chamadas falham na hora com ErrOpen. Em half-open passa uma única
// chamada de teste; as demais continuam recebendo ErrOpen até ela terminar.
package breaker

import (
	"errors"
	"sync"
	"time"
)

// ErrOpen indica que a chamada foi recusada sem chegar ao serviço externo.
var ErrOpen = errors.New("circuit breaker aberto")

type State int

const (
	Closed State = iota
	HalfOpen
	Open
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case HalfOpen:
		return "half-open"
	default:
		return "open"
	}
}

// Outcome classifica o resultado de uma chamada.
type Outcome int

const (
	Success Outcome = iota
	Failure
	// Ignored não diz nada sobre a saúde do serviço externo (ex.: o próprio
	// worker foi desligado no meio da chamada).
	Ignored
)

type Config struct {
	// MaxFailures é o número de falhas seguidas que abre o circuito (padrão 5).
	MaxFailures int
	// Cooldown é o tempo aberto antes de testar a volta (padrão 60 s).
	Cooldown time.Duration
	// Classify diz o que conta como falha. Padrão: todo erro é Failure.
	Classify func(error) Outcome
	// OnStateChange é chamado a cada transição, fora do lock.
	OnStateChange func(from, to State)
	// Now permite controlar o relógio nos testes.
	Now func() time.Time
}

type Breaker struct {
	cfg Config

	mu       sync.Mutex
	state    State
	failures int
	openedAt time.Time
	probing  bool
	// generation muda a cada transição: o resultado de uma chamada iniciada
	// antes da transição não pode alterar o estado novo.
	generation int
}

func New(cfg Config) *Breaker {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 5
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 60 * time.Second
	}
	if cfg.Classify == nil {
		cfg.Classify = func(err error) Outcome {
			if err != nil {
				return Failure
			}
			return Success
		}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Breaker{cfg: cfg}
}

// State devolve o estado atual. Um circuito aberto só vira half-open quando a
// próxima chamada chega.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Execute roda fn se o circuito permitir, e registra o resultado.
func (b *Breaker) Execute(fn func() error) error {
	generation, err := b.before()
	if err != nil {
		return err
	}

	// Um panic em fn conta como falha e não deixa o teste do half-open preso.
	finished := false
	defer func() {
		if !finished {
			b.after(generation, Failure)
		}
	}()

	err = fn()
	finished = true
	b.after(generation, b.cfg.Classify(err))
	return err
}

func (b *Breaker) before() (int, error) {
	b.mu.Lock()
	var notify func()

	if b.state == Open && b.cfg.Now().Sub(b.openedAt) >= b.cfg.Cooldown {
		notify = b.transition(HalfOpen)
	}

	var err error
	switch {
	case b.state == Open, b.state == HalfOpen && b.probing:
		err = ErrOpen
	case b.state == HalfOpen:
		b.probing = true
	}
	generation := b.generation
	b.mu.Unlock()

	if notify != nil {
		notify()
	}
	return generation, err
}

func (b *Breaker) after(generation int, outcome Outcome) {
	b.mu.Lock()
	var notify func()

	if generation == b.generation {
		switch b.state {
		case Closed:
			switch outcome {
			case Success:
				b.failures = 0
			case Failure:
				b.failures++
				if b.failures >= b.cfg.MaxFailures {
					notify = b.transition(Open)
				}
			}
		case HalfOpen:
			switch outcome {
			case Success:
				notify = b.transition(Closed)
			case Failure:
				notify = b.transition(Open)
			case Ignored:
				b.probing = false
			}
		}
	}
	b.mu.Unlock()

	if notify != nil {
		notify()
	}
}

// transition troca o estado (com o lock) e devolve a notificação a ser chamada
// depois de soltar o lock.
func (b *Breaker) transition(to State) func() {
	from := b.state
	b.state = to
	b.generation++
	b.failures = 0
	b.probing = false
	if to == Open {
		b.openedAt = b.cfg.Now()
	}
	if b.cfg.OnStateChange == nil {
		return nil
	}
	return func() { b.cfg.OnStateChange(from, to) }
}
