package ws

import (
	"math"
	"testing"
)

func TestEstimateClockOffset(t *testing.T) {
	tests := []struct {
		name       string
		t0         int64
		t1         int64
		serverTime int64
		wantOffset int64
	}{
		{
			name:       "perfeitamente sincronizado, RTT 40ms",
			t0:         1000,
			t1:         1040,
			serverTime: 1020,
			wantOffset: 0,
		},
		{
			name:       "cliente 2000ms atrasado em relação ao servidor",
			t0:         1000,
			t1:         1040,
			serverTime: 3020,
			wantOffset: 2000,
		},
		{
			name:       "cliente 1500ms adiantado em relação ao servidor",
			t0:         3000,
			t1:         3040,
			serverTime: 1520,
			wantOffset: -1500,
		},
		{
			name:       "assimetria de rede leve (RTT 60ms)",
			t0:         5000,
			t1:         5060,
			serverTime: 5035,
			wantOffset: 5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EstimateClockOffset(tc.t0, tc.t1, tc.serverTime)
			if got != tc.wantOffset {
				t.Errorf("EstimateClockOffset(%d, %d, %d) = %d; want %d", tc.t0, tc.t1, tc.serverTime, got, tc.wantOffset)
			}
		})
	}
}

func TestCalcPlaybackPosition(t *testing.T) {
	// Servidor iniciou faixa em serverTime = 10000 com seekOffset = 5.0s
	startedAt := int64(10000)
	seekOffset := 5.0

	t.Run("pausado retorna apenas seekOffset", func(t *testing.T) {
		pos := CalcPlaybackPosition(12000, startedAt, seekOffset, 0, false)
		if pos != 5.0 {
			t.Errorf("esperado 5.0, obtido %f", pos)
		}
	})

	t.Run("em reprodução com relógio sincronizado (offset 0)", func(t *testing.T) {
		// clientNow = 15000 -> decorrido = (15000 - 10000)/1000 = 5s -> pos = 5.0 + 5.0 = 10.0s
		clientNow := int64(15000)
		pos := CalcPlaybackPosition(clientNow, startedAt, seekOffset, 0, true)
		if math.Abs(pos-10.0) > 0.001 {
			t.Errorf("esperado 10.0, obtido %f", pos)
		}
	})

	t.Run("em reprodução com relógio do cliente 3s atrasado (clockOffset = +3000)", func(t *testing.T) {
		// Se o relógio do cliente está atrasado em 3s, clientNow é 12000 quando no servidor é 15000.
		// Sem correção: (12000 - 10000)/1000 + 5 = 7.0s (3s de erro!)
		// Com correção: (12000 + 3000 - 10000)/1000 + 5 = 10.0s (exato!)
		clientNow := int64(12000)
		clockOffset := int64(3000)
		pos := CalcPlaybackPosition(clientNow, startedAt, seekOffset, clockOffset, true)
		if math.Abs(pos-10.0) > 0.001 {
			t.Errorf("esperado 10.0, obtido %f", pos)
		}
	})

	t.Run("em reprodução com relógio do cliente 2s adiantado (clockOffset = -2000)", func(t *testing.T) {
		// clientNow é 17000 quando no servidor é 15000.
		// Com correção: (17000 - 2000 - 10000)/1000 + 5 = 10.0s (exato!)
		clientNow := int64(17000)
		clockOffset := int64(-2000)
		pos := CalcPlaybackPosition(clientNow, startedAt, seekOffset, clockOffset, true)
		if math.Abs(pos-10.0) > 0.001 {
			t.Errorf("esperado 10.0, obtido %f", pos)
		}
	})
}
