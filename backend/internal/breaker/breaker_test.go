package breaker

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

var (
	errFalha   = errors.New("falha")
	errIgnorar = errors.New("ignorar")
)

type relogio struct{ t time.Time }

func (r *relogio) now() time.Time         { return r.t }
func (r *relogio) avanca(d time.Duration) { r.t = r.t.Add(d) }

func falha() error { return errFalha }
func ok() error    { return nil }

func novoBreaker(r *relogio, n int) *Breaker {
	return New(Config{
		MaxFailures: n,
		Cooldown:    time.Minute,
		Now:         r.now,
		Classify: func(err error) Outcome {
			switch {
			case err == nil:
				return Success
			case errors.Is(err, errIgnorar):
				return Ignored
			}
			return Failure
		},
	})
}

func TestAbreDepoisDeNFalhasSeguidas(t *testing.T) {
	b := novoBreaker(&relogio{t: time.Now()}, 3)
	for i := 0; i < 3; i++ {
		if b.State() != Closed {
			t.Fatalf("antes da falha %d: estado %s", i+1, b.State())
		}
		if err := b.Execute(falha); !errors.Is(err, errFalha) {
			t.Fatalf("falha %d: erro %v", i+1, err)
		}
	}
	if b.State() != Open {
		t.Fatalf("esperado open, obtido %s", b.State())
	}

	chamou := false
	err := b.Execute(func() error { chamou = true; return nil })
	if !errors.Is(err, ErrOpen) || chamou {
		t.Fatalf("aberto deve recusar sem chamar: err=%v chamou=%v", err, chamou)
	}
}

func TestSucessoZeraAContagem(t *testing.T) {
	b := novoBreaker(&relogio{t: time.Now()}, 3)
	for _, fn := range []func() error{falha, falha, ok, falha, falha} {
		_ = b.Execute(fn)
	}
	if b.State() != Closed {
		t.Fatalf("as falhas não foram seguidas: esperado closed, obtido %s", b.State())
	}
}

func TestErroIgnoradoNaoContaNemZera(t *testing.T) {
	b := novoBreaker(&relogio{t: time.Now()}, 3)
	ignorado := func() error { return errIgnorar }
	for _, fn := range []func() error{falha, ignorado, falha, ignorado} {
		_ = b.Execute(fn)
	}
	if b.State() != Closed {
		t.Fatalf("esperado closed, obtido %s", b.State())
	}
	_ = b.Execute(falha)
	if b.State() != Open {
		t.Fatalf("esperado open na 3ª falha, obtido %s", b.State())
	}
}

func TestHalfOpenFechaQuandoOTesteDaCerto(t *testing.T) {
	r := &relogio{t: time.Now()}
	b := novoBreaker(r, 1)
	_ = b.Execute(falha)

	r.avanca(59 * time.Second)
	if err := b.Execute(ok); !errors.Is(err, ErrOpen) {
		t.Fatalf("antes do cooldown deveria recusar, obtido %v", err)
	}
	r.avanca(time.Second)
	if err := b.Execute(ok); err != nil {
		t.Fatalf("teste do half-open: %v", err)
	}
	if b.State() != Closed {
		t.Fatalf("esperado closed, obtido %s", b.State())
	}
}

func TestHalfOpenReabreEReiniciaOCooldown(t *testing.T) {
	r := &relogio{t: time.Now()}
	b := novoBreaker(r, 1)
	_ = b.Execute(falha)
	r.avanca(time.Minute)
	if err := b.Execute(falha); !errors.Is(err, errFalha) {
		t.Fatalf("o teste do half-open deveria executar, obtido %v", err)
	}
	if b.State() != Open {
		t.Fatalf("esperado open, obtido %s", b.State())
	}
	r.avanca(59 * time.Second)
	if err := b.Execute(ok); !errors.Is(err, ErrOpen) {
		t.Fatalf("o cooldown deveria ter reiniciado, obtido %v", err)
	}
}

func TestHalfOpenDeixaPassarUmaUnicaChamada(t *testing.T) {
	r := &relogio{t: time.Now()}
	b := novoBreaker(r, 1)
	_ = b.Execute(falha)
	r.avanca(time.Minute)

	entrou, libera := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = b.Execute(func() error { close(entrou); <-libera; return nil })
	}()
	<-entrou
	if b.State() != HalfOpen {
		t.Fatalf("esperado half-open, obtido %s", b.State())
	}
	for i := 0; i < 5; i++ {
		if err := b.Execute(ok); !errors.Is(err, ErrOpen) {
			t.Fatalf("chamada paralela deveria ser recusada, obtido %v", err)
		}
	}
	close(libera)
	wg.Wait()
	if b.State() != Closed {
		t.Fatalf("esperado closed, obtido %s", b.State())
	}
}

func TestHalfOpenComErroIgnoradoPermiteNovoTeste(t *testing.T) {
	r := &relogio{t: time.Now()}
	b := novoBreaker(r, 1)
	_ = b.Execute(falha)
	r.avanca(time.Minute)
	_ = b.Execute(func() error { return errIgnorar })
	if b.State() != HalfOpen {
		t.Fatalf("esperado half-open, obtido %s", b.State())
	}
	if err := b.Execute(ok); err != nil || b.State() != Closed {
		t.Fatalf("novo teste: err=%v estado=%s", err, b.State())
	}
}

// Uma chamada lenta, iniciada com o circuito fechado, termina bem depois de ele
// ter aberto e passado para half-open: o resultado antigo não fecha o circuito.
func TestResultadoAtrasadoNaoMudaOEstadoNovo(t *testing.T) {
	r := &relogio{t: time.Now()}
	b := novoBreaker(r, 1)

	entrou, libera, saiu := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		_ = b.Execute(func() error { close(entrou); <-libera; return nil })
		close(saiu)
	}()
	<-entrou
	_ = b.Execute(falha)
	r.avanca(time.Minute)
	_ = b.Execute(func() error { return errIgnorar })
	if b.State() != HalfOpen {
		t.Fatalf("esperado half-open, obtido %s", b.State())
	}
	close(libera)
	<-saiu
	if b.State() != HalfOpen {
		t.Fatalf("o resultado atrasado mudou o estado para %s", b.State())
	}
}

func TestPanicContaComoFalha(t *testing.T) {
	b := novoBreaker(&relogio{t: time.Now()}, 1)
	func() {
		defer func() { _ = recover() }()
		_ = b.Execute(func() error { panic("x") })
	}()
	if b.State() != Open {
		t.Fatalf("esperado open, obtido %s", b.State())
	}
}

func TestOnStateChange(t *testing.T) {
	r := &relogio{t: time.Now()}
	var got [][2]State
	b := New(Config{MaxFailures: 1, Cooldown: time.Minute, Now: r.now,
		OnStateChange: func(from, to State) { got = append(got, [2]State{from, to}) }})
	_ = b.Execute(falha)
	r.avanca(time.Minute)
	_ = b.Execute(falha)
	r.avanca(time.Minute)
	_ = b.Execute(ok)

	want := [][2]State{{Closed, Open}, {Open, HalfOpen}, {HalfOpen, Open}, {Open, HalfOpen}, {HalfOpen, Closed}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("transições: %v, esperado %v", got, want)
	}
}
