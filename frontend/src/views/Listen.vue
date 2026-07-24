<template>
  <div class="min-h-screen bg-zinc-900 text-zinc-100 flex flex-col">
    <header class="border-b border-zinc-800 px-4 py-3">
      <div class="max-w-2xl mx-auto flex items-center justify-between">
        <div class="flex items-center gap-2.5">
          <Disc class="w-5 h-5 text-emerald-500" />
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
        <Player ref="playerRef" @toggle-play="handleTogglePlay" />
      </div>

      <!-- Seção Sugerir Músicas para Ouvintes -->
      <div class="bg-zinc-800 rounded-lg p-4 space-y-4">
        <div class="flex items-center justify-between border-b border-zinc-700 pb-3">
          <h3 class="text-sm font-semibold text-zinc-200 flex items-center gap-2">
            <Music class="w-4 h-4 text-emerald-400" />
            Sugerir Música para o DJ
          </h3>
          <span class="text-xs text-zinc-400 flex items-center gap-1">
            <User class="w-3.5 h-3.5" />
            Apelido:
          </span>
        </div>

        <div class="flex gap-2">
          <input
            v-model="listenerName"
            @change="saveNickname"
            placeholder="Seu nome / apelido"
            class="bg-zinc-700 text-xs rounded-lg px-3 py-2 outline-none focus:ring-1 focus:ring-emerald-500 w-full"
          />
        </div>

        <!-- Abas de Sugestão -->
        <div class="flex border-b border-zinc-700 gap-2 pb-2">
          <button
            @click="suggestMode = 'library'"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors"
            :class="suggestMode === 'library' ? 'bg-emerald-600 text-white' : 'text-zinc-400 hover:bg-zinc-700'"
          >
            <Library class="w-3.5 h-3.5" />
            Biblioteca do Servidor
          </button>
          <button
            @click="suggestMode = 'custom'"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors"
            :class="suggestMode === 'custom' ? 'bg-emerald-600 text-white' : 'text-zinc-400 hover:bg-zinc-700'"
          >
            <Youtube class="w-3.5 h-3.5 text-red-400" />
            Link YouTube / Título
          </button>
        </div>

        <!-- Aba Biblioteca -->
        <div v-if="suggestMode === 'library'" class="space-y-3">
          <div class="relative">
            <Search class="w-3.5 h-3.5 absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-500" />
            <input
              v-model="searchQuery"
              placeholder="Buscar música no catálogo..."
              class="w-full bg-zinc-700 rounded-lg pl-8 pr-3 py-1.5 text-xs outline-none focus:ring-1 focus:ring-emerald-500"
            />
          </div>

          <div v-if="loadingLibrary" class="text-zinc-500 text-xs text-center py-4">
            Carregando músicas...
          </div>
          <div v-else-if="filteredLibraryTracks.length === 0" class="text-zinc-500 text-xs text-center py-4">
            Nenhuma música encontrada
          </div>
          <div v-else class="space-y-1 max-h-48 overflow-y-auto pr-1">
            <div
              v-for="track in filteredLibraryTracks"
              :key="track.id"
              class="flex items-center justify-between p-2 rounded hover:bg-zinc-700/70 text-xs group transition-colors"
            >
              <span class="truncate flex-1 text-zinc-200">{{ track.title }}</span>
              <button
                @click="suggestLibraryTrack(track)"
                class="px-2 py-1 bg-emerald-600/80 hover:bg-emerald-600 text-white text-[11px] rounded flex items-center gap-1 transition-colors shrink-0 ml-2"
              >
                <Plus class="w-3 h-3" />
                Sugerir
              </button>
            </div>
          </div>
        </div>

        <!-- Aba Personalizada / YouTube -->
        <div v-else class="space-y-3">
          <input
            v-model="customInput"
            @keyup.enter="suggestCustomTrack"
            placeholder="Cole a URL do YouTube ou digite o nome da música..."
            class="w-full bg-zinc-700 rounded-lg px-3 py-2 text-xs outline-none focus:ring-1 focus:ring-emerald-500"
          />
          <div class="flex justify-end">
            <button
              @click="suggestCustomTrack"
              :disabled="!customInput.trim()"
              class="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:bg-zinc-700 disabled:opacity-50 text-white rounded-lg text-xs font-medium flex items-center gap-1.5 transition-colors"
            >
              <Send class="w-3.5 h-3.5" />
              Enviar Sugestão
            </button>
          </div>
        </div>

        <p v-if="feedbackMsg" class="text-xs text-emerald-400 font-medium bg-emerald-950/40 p-2 rounded border border-emerald-800/40">
          {{ feedbackMsg }}
        </p>

        <!-- Lista de Sugestões Pendentes -->
        <div v-if="store.suggestions && store.suggestions.length > 0" class="pt-3 border-t border-zinc-700/60">
          <h4 class="text-xs font-medium text-zinc-400 mb-2 flex items-center gap-1">
            <Clock class="w-3.5 h-3.5 text-amber-400" />
            Sugestões na Fila de Avaliação do DJ ({{ store.suggestions.length }})
          </h4>
          <div class="space-y-1.5 max-h-36 overflow-y-auto pr-1">
            <div
              v-for="sug in store.suggestions"
              :key="sug.id"
              class="flex items-center justify-between p-2 rounded bg-zinc-900/60 text-xs"
            >
              <div class="truncate flex-1 mr-2">
                <span class="text-zinc-200 font-medium block truncate">{{ sug.title }}</span>
                <span class="text-[10px] text-zinc-500">Por: {{ sug.suggestedBy }}</span>
              </div>
              <span class="text-[10px] bg-amber-500/20 text-amber-300 border border-amber-500/30 px-2 py-0.5 rounded shrink-0">
                Pendente
              </span>
            </div>
          </div>
        </div>
      </div>

      <div class="bg-zinc-800 rounded-lg p-4">
        <h3 class="text-sm font-medium text-zinc-400 mb-3">Playlist Atual</h3>
        <div v-if="store.playlist.length === 0" class="text-zinc-500 text-sm text-center py-4">
          Playlist vazia
        </div>
        <div v-for="(track, i) in store.playlist" :key="track.id" class="flex items-center gap-3 py-2 border-b border-zinc-700 last:border-0">
          <span class="text-xs text-zinc-500 w-5">{{ i + 1 }}</span>
          <span class="text-sm" :class="i === 0 && store.state.isPlaying ? 'text-emerald-400 font-semibold' : ''">{{ track.title }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Radio, Copy, Disc, Music, User, Library, Youtube, Search, Plus, Send, Clock } from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { getStation, getGlobalLibrary } from '../services/api'
import { useWebSocket } from '../composables/useWebSocket'
import { saveStation, removeStation } from '../services/storage'
import Player from '../components/Player.vue'

const route = useRoute()
const router = useRouter()
const store = useStationStore()
const { connect, send, disconnect } = useWebSocket(() => {
  send({ type: 'SYNC_REQUEST' })
  if (playerRef.value?.syncPlayback) {
    playerRef.value.syncPlayback()
  }
})
const showPlayer = ref(false)
const playerRef = ref(null)

const listenerName = ref(localStorage.getItem('listener_nickname') || 'Ouvinte')
const suggestMode = ref('library')
const searchQuery = ref('')
const libraryTracks = ref([])
const loadingLibrary = ref(false)
const customInput = ref('')
const feedbackMsg = ref('')

const filteredLibraryTracks = computed(() => {
  if (!searchQuery.value.trim()) return libraryTracks.value
  const q = searchQuery.value.toLowerCase()
  return libraryTracks.value.filter(t => t.title && t.title.toLowerCase().includes(q))
})

function saveNickname() {
  if (listenerName.value.trim()) {
    localStorage.setItem('listener_nickname', listenerName.value.trim())
  }
}

async function fetchLibrary() {
  loadingLibrary.value = true
  try {
    const tracks = await getGlobalLibrary()
    libraryTracks.value = tracks
  } catch (e) {
    console.error('Erro ao buscar biblioteca:', e)
  } finally {
    loadingLibrary.value = false
  }
}

function suggestLibraryTrack(track) {
  saveNickname()
  send({
    type: 'SUGGEST_TRACK',
    data: {
      trackId: track.id,
      title: track.title,
      url: track.url,
      suggestedBy: listenerName.value || 'Ouvinte Anônimo',
    },
  })
  showFeedback(`Sugestão "${track.title}" enviada ao DJ!`)
}

function suggestCustomTrack() {
  const val = customInput.value.trim()
  if (!val) return
  saveNickname()
  send({
    type: 'SUGGEST_TRACK',
    data: {
      title: val,
      url: val.startsWith('http') ? val : '',
      suggestedBy: listenerName.value || 'Ouvinte Anônimo',
    },
  })
  showFeedback(`Sugestão enviada ao DJ!`)
  customInput.value = ''
}

function showFeedback(msg) {
  feedbackMsg.value = msg
  setTimeout(() => {
    feedbackMsg.value = ''
  }, 4000)
}

function enterStation() {
  showPlayer.value = true
  connect()
  fetchLibrary()
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
      if (data.suggestions) store.setSuggestions(data.suggestions)
      if (route.query.autoJoin) {
        enterStation()
      }
    } catch (e) {
      removeStation(id)
      store.reset()
      router.push('/')
    }
  }
})
</script>
