import { ref, onUnmounted } from 'vue'
import { useStationStore } from '../stores/station'

export function useWebSocket(onConnected) {
  const store = useStationStore()
  const ws = ref(null)
  const connected = ref(false)
  let reconnectTimer = null
  let syncT0 = 0

  function connect() {
    if (!store.stationId) return

    const host = import.meta.env.VITE_WS_URL || `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}`
    const url = `${host}/ws/stations/${store.stationId}`

    // O token de DJ vai no subprotocolo (Sec-WebSocket-Protocol), não na URL:
    // URLs ficam nos logs de acesso do servidor e do gateway (ADR-0005).
    const protocols = ['radiosync.v1']
    if (store.role === 'dj' && store.djToken) {
      protocols.push(`radiosync.auth.${store.djToken}`)
    }

    const socket = new WebSocket(url, protocols)

    socket.onopen = () => {
      connected.value = true
      if (onConnected) onConnected()
    }

    socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        handleMessage(msg)
      } catch (e) {
        console.error('Erro ao parsear mensagem WS:', e)
      }
    }

    socket.onclose = () => {
      connected.value = false
      reconnectTimer = setTimeout(() => connect(), 3000)
    }

    socket.onerror = () => {
      socket.close()
    }

    ws.value = socket
  }

  function handleMessage(msg) {
    switch (msg.type) {
      case 'STATE_UPDATE':
        store.setState(msg.state)
        break
      case 'PLAYLIST_UPDATED':
        store.setPlaylist(msg.playlist)
        break
      case 'SUGGESTIONS_UPDATED':
        store.setSuggestions(msg.suggestions)
        break
      case 'MIXER_UPDATED':
        store.setMixer(msg.channels)
        break
      case 'DJ_STATUS':
        store.hasDJOnline = !!msg.hasDJOnline
        break
      case 'TRACK_READY':
        store.applyJob({ jobId: msg.jobId, status: 'ready', track: msg.track })
        break
      case 'TRACK_FAILED':
        // Só o DJ acompanha falhas: o ouvinte não tem lista de jobs
        if (store.role === 'dj') {
          store.applyJob({ jobId: msg.jobId, title: msg.title, status: 'failed', error: msg.error })
        }
        break
      case 'SYNC': {
        const t1 = Date.now()
        if (msg.serverTime && syncT0) {
          // Estimativa de offset estilo NTP: offset = serverTime - (t0 + t1)/2 (ADR-0004)
          const clientMid = (syncT0 + t1) / 2
          store.clockOffset = msg.serverTime - clientMid
        }
        store.state.seekOffset = msg.position
        store.state.startedAt = Date.now() + (store.clockOffset || 0)
        if (msg.mixer) {
          store.setMixer(msg.mixer)
        }
        break
      }
    }
  }

  function send(data) {
    if (ws.value && ws.value.readyState === WebSocket.OPEN) {
      if (data && data.type === 'SYNC_REQUEST') {
        syncT0 = Date.now()
      }
      ws.value.send(JSON.stringify(data))
    }
  }

  function disconnect() {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (ws.value) {
      ws.value.onclose = null
      ws.value.close()
      ws.value = null
    }
    connected.value = false
  }

  onUnmounted(() => disconnect())

  return { ws, connected, connect, send, disconnect }
}
