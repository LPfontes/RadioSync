<template>
  <div class="min-h-screen bg-zinc-900 text-zinc-100 flex flex-col">
    <header class="border-b border-zinc-800 px-4 py-3">
      <div class="max-w-2xl mx-auto flex items-center justify-between">
        <div class="flex items-center gap-3">
          <span class="font-bold text-emerald-500">RadioSync</span>
          <span class="text-xs bg-zinc-800 px-2 py-0.5 rounded font-mono">{{ store.stationId }}</span>
        </div>
        <button @click="leave" class="text-xs text-zinc-400 hover:text-zinc-200 transition-colors">Sair</button>
      </div>
    </header>

    <div v-if="!showPlayer" class="flex-1 flex items-center justify-center p-4">
      <div class="text-center space-y-4">
        <div class="w-20 h-20 mx-auto rounded-full bg-emerald-600/20 flex items-center justify-center">
          <Radio class="w-10 h-10 text-emerald-500" />
        </div>
        <p class="text-zinc-300">Clique para entrar na estação</p>
        <button @click="enterStation" class="px-6 py-3 bg-emerald-600 hover:bg-emerald-500 rounded-lg font-medium transition-colors">
          Entrar na Estação
        </button>
      </div>
    </div>

    <div v-else class="flex-1 max-w-2xl w-full mx-auto p-4 space-y-6">
      <div class="bg-zinc-800 rounded-lg p-4 text-center">
        <p class="text-xs text-zinc-400 mb-1">Código da estação</p>
        <div class="flex items-center justify-center gap-3">
          <span class="text-2xl font-bold tracking-widest text-emerald-400 font-mono">{{ store.stationId }}</span>
          <button @click="copyCode" class="text-xs text-zinc-400 hover:text-emerald-400 transition-colors">
            <Copy class="w-4 h-4" />
          </button>
        </div>
      </div>

      <div class="bg-zinc-800 rounded-lg p-6">
        <Player @toggle-play="handleTogglePlay" />
      </div>

      <div class="bg-zinc-800 rounded-lg p-4">
        <h3 class="text-sm font-medium text-zinc-400 mb-3">Playlist</h3>
        <div v-if="store.playlist.length === 0" class="text-zinc-500 text-sm text-center py-4">
          Playlist vazia
        </div>
        <div v-for="(track, i) in store.playlist" :key="track.id" class="flex items-center gap-3 py-2 border-b border-zinc-700 last:border-0">
          <span class="text-xs text-zinc-500 w-5">{{ i + 1 }}</span>
          <span class="text-sm">{{ track.title }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Radio, Copy } from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { getStation } from '../services/api'
import { useWebSocket } from '../composables/useWebSocket'
import { saveStation, removeStation } from '../services/storage'
import Player from '../components/Player.vue'

const route = useRoute()
const router = useRouter()
const store = useStationStore()
const { connect, send, disconnect } = useWebSocket()
const showPlayer = ref(false)

function enterStation() {
  showPlayer.value = true
  connect()
}

function handleTogglePlay(action, value) {
  if (action === 'SEEK') {
    send({ type: 'SEEK', data: { position: value } })
  } else if (action === 'NEXT_TRACK') {
    send({ type: 'NEXT_TRACK' })
  } else {
    send({ type: action })
  }
}

function copyCode() {
  navigator.clipboard.writeText(store.stationId)
}

function leave() {
  disconnect()
  removeStation(store.stationId)
  store.reset()
  router.push('/')
}

onMounted(async () => {
  const id = route.params.stationId
  if (id) {
    store.stationId = id
    saveStation(id, 'listener', null)
    try {
      const data = await getStation(id)
      if (data.state) store.setState(data.state)
      if (data.playlist) store.setPlaylist(data.playlist)
    } catch (e) {
      console.error('Erro ao carregar estação:', e)
    }
  }
})
</script>
