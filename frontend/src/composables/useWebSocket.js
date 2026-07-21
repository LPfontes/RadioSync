import { ref, onUnmounted } from 'vue'
import { useStationStore } from '../stores/station'

export function useWebSocket() {
  const store = useStationStore()
  const ws = ref(null)
  const connected = ref(false)
  let reconnectTimer = null

  function connect() {
    if (!store.stationId) return

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = import.meta.env.VITE_WS_URL || `${protocol}//localhost:8080`
    let url = `${host}/ws/stations/${store.stationId}`

    if (store.role === 'dj' && store.djToken) {
      url += `?role=dj&token=${store.djToken}`
    }

    const socket = new WebSocket(url)

    socket.onopen = () => {
      connected.value = true
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
    }
  }

  function send(data) {
    if (ws.value && ws.value.readyState === WebSocket.OPEN) {
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
