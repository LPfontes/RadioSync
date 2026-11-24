package queue

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRetryPolicy_BackoffEFim(t *testing.T) {
	p := DefaultRetryPolicy()
	want := []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}
	for attempt, d := range want {
		got, ok := p.Next(attempt)
		if !ok || got != d {
			t.Errorf("tentativa %d: (%v, %v), esperado (%v, true)", attempt, got, ok, d)
		}
		if attempt > 0 && got < 2*want[attempt-1] {
			t.Errorf("o atraso %d não cresce exponencialmente: %v depois de %v", attempt, got, want[attempt-1])
		}
	}
	for _, attempt := range []int{3, 4, 100, -1} {
		if d, ok := p.Next(attempt); ok {
			t.Errorf("tentativa %d deveria ir para a DLQ, obtido atraso %v", attempt, d)
		}
	}
}

func TestParseRetryPolicy(t *testing.T) {
	p, err := ParseRetryPolicy("100ms, 1s,2m")
	if err != nil || !reflect.DeepEqual(p.Delays, []time.Duration{100 * time.Millisecond, time.Second, 2 * time.Minute}) {
		t.Fatalf("obtido %v, %v", p, err)
	}
	if p, err := ParseRetryPolicy(""); err != nil || !reflect.DeepEqual(p, DefaultRetryPolicy()) {
		t.Fatalf("vazio deveria usar o padrão, obtido %v, %v", p, err)
	}
	for _, s := range []string{"5", "5s,", "-1s", "abc", "0s"} {
		if _, err := ParseRetryPolicy(s); err == nil {
			t.Errorf("%q deveria ser recusado", s)
		}
	}
}

func TestJobValidate(t *testing.T) {
	validos := []Job{
		{ID: "a", StationID: "S", Kind: KindUpload, SourceKey: "originals/a.mp3"},
		{ID: "a", StationID: "S", Kind: KindYouTube, VideoURL: "https://youtu.be/x", VideoID: "x"},
	}
	for _, j := range validos {
		if err := j.Validate(); err != nil {
			t.Errorf("%+v deveria ser válido: %v", j, err)
		}
	}
	invalidos := []Job{
		{},
		{ID: "a", Kind: KindUpload, SourceKey: "k"},
		{ID: "a", StationID: "S", Kind: KindUpload},
		{ID: "a", StationID: "S", Kind: KindYouTube, VideoURL: "https://youtu.be/x"},
		{ID: "a", StationID: "S", Kind: "outro"},
	}
	for _, j := range invalidos {
		if j.Validate() == nil {
			t.Errorf("%+v deveria ser inválido", j)
		}
	}
}

func TestMemory_NaoConfirmadaVoltaQuandoOConsumidorMorre(t *testing.T) {
	q := NewMemory()
	_ = q.Publish(context.Background(), Job{ID: "j1"})

	ctx, kill := context.WithCancel(context.Background())
	got := make(chan *Delivery, 1)
	go func() { _ = q.Consume(ctx, 1, func(_ context.Context, d *Delivery) { got <- d }) }()
	first := <-got
	kill()

	ctx2, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = q.Consume(ctx2, 1, func(_ context.Context, d *Delivery) { got <- d }) }()
	second := <-got
	if second.Job.ID != "j1" || !second.Redelivered {
		t.Fatalf("esperada a reentrega de j1, obtido %+v", second)
	}
	if first.Ack() == nil {
		t.Fatal("o ack do consumidor morto não deveria valer")
	}
	if err := second.Ack(); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------- RabbitMQ real

// testRabbit roda contra um RabbitMQ real se TEST_AMQP_URL estiver definido.
// Use um vhost só para testes: as filas são esvaziadas.
func testRabbit(t *testing.T, delays ...time.Duration) *Rabbit {
	t.Helper()
	url := os.Getenv("TEST_AMQP_URL")
	if url == "" {
		t.Skip("TEST_AMQP_URL não definido")
	}
	r := NewRabbit(url, delays...)
	if err := r.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	ch, err := r.openChannel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	for _, name := range []string{TranscodeQueue, DeadLetterQueue} {
		if _, err := ch.QueuePurge(name, false); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func waitDepth(t *testing.T, r *Rabbit, queue string, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var got int
	for time.Now().Before(deadline) {
		depths, err := r.Depths()
		if err != nil {
			t.Fatal(err)
		}
		if got = depths[queue]; got == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s: %d mensagens, esperado %d", queue, got, want)
}

func receive(t *testing.T, ch <-chan *Delivery) *Delivery {
	t.Helper()
	select {
	case d := <-ch:
		return d
	case <-time.After(15 * time.Second):
		t.Fatal("nenhuma entrega em 15 s")
		return nil
	}
}

// RES-02 no broker real: a conexão do worker cai sem ack e outro worker recebe
// a mesma mensagem.
func TestRabbit_WorkerMortoReentrega(t *testing.T) {
	r := testRabbit(t)
	if err := r.Publish(context.Background(), Job{ID: "res02"}); err != nil {
		t.Fatal(err)
	}

	dying := NewRabbit(r.url)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan *Delivery, 2)
	ctxDone := make(chan struct{})
	go func() {
		_ = dying.Consume(ctx, 1, func(hctx context.Context, d *Delivery) {
			got <- d
			<-hctx.Done()
			close(ctxDone)
		})
	}()
	receive(t, got)

	// Derruba a conexão sem ack, como um kill -9.
	dying.mu.Lock()
	dying.closed = true
	_ = dying.conn.Close()
	dying.mu.Unlock()
	select {
	case <-ctxDone:
	case <-time.After(5 * time.Second):
		t.Fatal("o handler deveria ser cancelado quando a conexão cai")
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go func() { _ = r.Consume(ctx2, 1, func(_ context.Context, d *Delivery) { got <- d }) }()
	d := receive(t, got)
	if d.Job.ID != "res02" || !d.Redelivered {
		t.Fatalf("esperada a reentrega de res02, obtido %+v", d)
	}
	if err := d.Ack(); err != nil {
		t.Fatal(err)
	}
	waitDepth(t, r, TranscodeQueue, 0)
}

func TestRabbit_AtrasoVoltaParaAFila(t *testing.T) {
	r := testRabbit(t, 300*time.Millisecond)
	start := time.Now()
	if err := r.PublishDelayed(context.Background(), Job{ID: "atrasado", Attempt: 2}, 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan *Delivery, 1)
	go func() { _ = r.Consume(ctx, 1, func(_ context.Context, d *Delivery) { got <- d }) }()
	d := receive(t, got)
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Errorf("voltou cedo demais: %v", elapsed)
	}
	if d.Job.ID != "atrasado" || d.Job.Attempt != 2 {
		t.Errorf("job alterado no caminho: %+v", d.Job)
	}
	_ = d.Ack()

	if err := r.PublishDelayed(context.Background(), Job{ID: "x"}, time.Hour); err == nil {
		t.Error("um atraso sem fila deveria dar erro, não perder a mensagem")
	}
}

func TestRabbit_DeadLetterEMensagemIlegivel(t *testing.T) {
	r := testRabbit(t)
	if err := r.DeadLetter(context.Background(), Job{ID: "morto"}, "motivo"); err != nil {
		t.Fatal(err)
	}
	waitDepth(t, r, DeadLetterQueue, 1)

	// Corpo inválido: o consumidor rejeita e a fila manda para a DLQ.
	ch, err := r.openChannel()
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.PublishWithContext(context.Background(), "", TranscodeQueue, false, false, amqp.Publishing{Body: []byte("{inválido")}); err != nil {
		t.Fatal(err)
	}
	_ = ch.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = r.Consume(ctx, 1, func(_ context.Context, d *Delivery) { t.Errorf("não deveria entregar: %+v", d) })
	}()
	waitDepth(t, r, DeadLetterQueue, 2)
}

// Uma mensagem que derruba o worker toda vez (poison message) vai para a DLQ
// pelo x-delivery-limit, em vez de circular para sempre.
func TestRabbit_LimiteDeReentregas(t *testing.T) {
	r := testRabbit(t)
	if err := r.Publish(context.Background(), Job{ID: "veneno"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entregas := make(chan struct{}, 100)
	go func() {
		_ = r.Consume(ctx, 1, func(_ context.Context, d *Delivery) {
			entregas <- struct{}{}
			_ = d.Requeue()
		})
	}()
	waitDepth(t, r, DeadLetterQueue, 1)
	if n := len(entregas); n != deliveryLimit+1 {
		t.Errorf("%d entregas, esperado %d", n, deliveryLimit+1)
	}
}

func TestRabbit_ForaDoArFalhaComErrUnavailable(t *testing.T) {
	r := NewRabbit("amqp://radiosync:radiosync@127.0.0.1:1/")
	err := r.Publish(context.Background(), Job{ID: "x"})
	if err == nil || !strings.Contains(err.Error(), ErrUnavailable.Error()) {
		t.Fatalf("esperado ErrUnavailable, obtido %v", err)
	}
	if r.Connected() {
		t.Fatal("não deveria constar como conectado")
	}
}

// O contexto do trace viaja no corpo do Job (e não em cabeçalho AMQP) para
// sobreviver a retry e DLQ, que republicam o Job.
func TestJob_TraceViajaNoJSONESomeQuandoVazio(t *testing.T) {
	semTrace, err := json.Marshal(Job{ID: "j1", Kind: KindUpload, StationID: "S"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(semTrace), `"trace"`) {
		t.Fatalf("Job sem trace não deveria ter o campo no JSON: %s", semTrace)
	}

	com := Job{ID: "j1", Kind: KindUpload, StationID: "S", Trace: map[string]string{"traceparent": "00-aaaa-bbbb-01"}}
	raw, err := json.Marshal(com)
	if err != nil {
		t.Fatal(err)
	}
	var volta Job
	if err := json.Unmarshal(raw, &volta); err != nil {
		t.Fatal(err)
	}
	if volta.Trace["traceparent"] != "00-aaaa-bbbb-01" {
		t.Fatalf("o trace se perdeu na ida e volta pelo JSON: %+v", volta.Trace)
	}

	// mensagem antiga, publicada antes de existir o campo, continua válida
	var antigo Job
	if err := json.Unmarshal([]byte(`{"jobId":"j0","kind":"upload","stationId":"S","sourceKey":"k"}`), &antigo); err != nil {
		t.Fatal(err)
	}
	if antigo.Trace != nil {
		t.Fatalf("job antigo deveria ter Trace nil, veio %v", antigo.Trace)
	}
}
