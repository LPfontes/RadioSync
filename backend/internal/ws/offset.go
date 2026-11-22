package ws

// EstimateClockOffset computes the client-server clock offset using NTP algorithm:
// t0: client timestamp when request was sent (ms)
// t1: client timestamp when response was received (ms)
// serverTime: server timestamp when request was processed (ms)
// offset = serverTime - (t0 + t1)/2
// With this offset, estimated server time at client local time t is: t + offset
func EstimateClockOffset(t0, t1, serverTime int64) int64 {
	clientMid := (t0 + t1) / 2
	return serverTime - clientMid
}

// CalcPlaybackPosition calculates the expected position in seconds:
// clientNow: current client local time (Unix ms)
// startedAt: server timestamp when playback started (Unix ms)
// seekOffset: offset in seconds
// clockOffset: estimated serverTime - clientTime (Unix ms)
// isPlaying: whether playback is active
func CalcPlaybackPosition(clientNow, startedAt int64, seekOffset float64, clockOffset int64, isPlaying bool) float64 {
	if !isPlaying {
		return seekOffset
	}
	serverNow := clientNow + clockOffset
	elapsed := float64(serverNow-startedAt) / 1000.0
	if elapsed < 0 {
		elapsed = 0
	}
	return seekOffset + elapsed
}
