<template>
  <div class="min-h-screen bg-zinc-900 text-zinc-100 flex flex-col">
    <header class="border-b border-zinc-800 px-4 py-3">
      <div class="max-w-4xl mx-auto flex items-center justify-between">
        <div class="flex items-center gap-3">
          <span class="font-bold text-emerald-500">RadioSync — DJ</span>
          <span class="text-xs bg-zinc-800 px-2 py-0.5 rounded font-mono">{{ store.stationId }}</span>
        </div>
        <button @click="leave" class="text-xs text-zinc-400 hover:text-zinc-200 transition-colors">Sair</button>
      </div>
    </header>

    <div class="flex-1 max-w-4xl w-full mx-auto p-4 space-y-6">
      <div class="bg-zinc-800 rounded-lg p-4 text-center">
        <p class="text-xs text-zinc-400 mb-1">Compartilhe o código da estação</p>
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

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <!-- Esquerda: Repositório / Biblioteca Global -->
        <div class="bg-zinc-800 rounded-lg p-4 flex flex-col">
          <!-- Navegação de Abas -->
          <div class="flex border-b border-zinc-700 mb-4 pb-2 gap-2">
            <button @click="activeTab = 'repo'" class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors" :class="activeTab === 'repo' ? 'bg-emerald-600 text-white' : 'text-zinc-400 hover:bg-zinc-700'">
              <Folder class="w-3.5 h-3.5" />
              Repositório Local
            </button>
            <button @click="switchTab('global')" class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors" :class="activeTab === 'global' ? 'bg-emerald-600 text-white' : 'text-zinc-400 hover:bg-zinc-700'">
              <Library class="w-3.5 h-3.5" />
              Biblioteca Global
            </button>
          </div>

          <!-- Conteúdo Aba Repositório Local -->
          <div v-if="activeTab === 'repo'" class="space-y-4">
            <div>
              <h3 class="text-xs font-medium text-zinc-400 mb-2">Upload de Músicas</h3>
              <div class="space-y-3">
                <input type="file" ref="fileInputRef" accept=".mp3,.mp4,.wav,.ogg,.flac,.aac,.m4a" @change="selectFile" class="block w-full text-xs text-zinc-400 file:mr-3 file:py-1.5 file:px-3 file:rounded-lg file:border-0 file:text-xs file:font-medium file:bg-zinc-700 file:text-zinc-200 hover:file:bg-zinc-600" />
                <div v-if="selectedFile" class="flex items-center gap-2 text-sm text-zinc-300">
                  <span class="flex-1 truncate text-xs">{{ selectedFile.name }}</span>
                  <button @click="handleUpload" :disabled="uploading" class="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:bg-zinc-600 disabled:opacity-50 rounded-lg text-xs font-medium transition-colors">
                    {{ uploading ? 'Enviando...' : 'Upload' }}
                  </button>
                </div>
                <p v-if="uploadError" class="text-xs text-red-400">{{ uploadError }}</p>
              </div>
            </div>

            <div class="space-y-1">
              <p class="text-xs text-zinc-500 mb-2">Músicas desta Estação (clique para adicionar à playlist)</p>
              <div v-if="store.repository.length === 0" class="text-zinc-500 text-xs text-center py-4">
                Nenhuma música no repositório local
              </div>
              <div v-for="track in store.repository" :key="track.id" @click="addToPlaylist(track.id)" class="flex items-center gap-2 py-1.5 px-2 rounded hover:bg-zinc-700 cursor-pointer text-xs group">
                <Plus class="w-3.5 h-3.5 text-emerald-500 group-hover:scale-110 transition-transform" />
                <span class="truncate flex-1">{{ track.title }}</span>
                <span v-if="track.duration" class="text-[10px] text-zinc-500">{{ formatTime(track.duration) }}</span>
              </div>
            </div>
          </div>

          <!-- Conteúdo Aba Biblioteca Global -->
          <div v-else class="space-y-3">
            <div class="flex items-center gap-2">
              <div class="relative flex-1">
                <Search class="w-3.5 h-3.5 absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-500" />
                <input v-model="searchQuery" placeholder="Buscar na biblioteca..." class="w-full bg-zinc-700 rounded-lg pl-8 pr-3 py-1.5 text-xs outline-none focus:ring-1 focus:ring-emerald-500" />
              </div>
              <button @click="fetchGlobalLibrary" :disabled="loadingGlobal" class="p-1.5 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-zinc-400 hover:text-zinc-200 transition-colors" title="Atualizar biblioteca">
                <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': loadingGlobal }" />
              </button>
            </div>

            <p class="text-xs text-zinc-500 mb-2">Todas as músicas salvas no servidor (clique para adicionar à playlist)</p>

            <div v-if="loadingGlobal" class="text-zinc-500 text-xs text-center py-6">
              Carregando biblioteca...
            </div>
            <div v-else-if="filteredGlobalTracks.length === 0" class="text-zinc-500 text-xs text-center py-6">
              Nenhuma música encontrada na biblioteca
            </div>
            <div v-else class="space-y-1 max-h-64 overflow-y-auto pr-1">
              <div v-for="track in filteredGlobalTracks" :key="track.id" @click="addGlobalTrack(track)" class="flex items-center gap-2 py-1.5 px-2 rounded hover:bg-zinc-700 cursor-pointer text-xs group">
                <Plus class="w-3.5 h-3.5 text-emerald-500 group-hover:scale-110 transition-transform" />
                <span class="truncate flex-1">{{ track.title }}</span>
                <span v-if="track.duration" class="text-[10px] text-zinc-500">{{ formatTime(track.duration) }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Direita: Playlist -->
        <div class="bg-zinc-800 rounded-lg p-4">
          <h3 class="text-sm font-medium text-zinc-400 mb-3">Playlist Fila de Reprodução</h3>
          <div v-if="store.playlist.length === 0" class="text-zinc-500 text-sm text-center py-4">
            Playlist vazia — adicione músicas do repositório ou da biblioteca global
          </div>
          <div v-for="(track, i) in store.playlist" :key="track.id" class="flex items-center gap-3 py-2 border-b border-zinc-700 last:border-0">
            <button @click="removeFromPlaylist(track.id)" class="text-red-400 hover:text-red-300">
              <X class="w-3.5 h-3.5" />
            </button>
            <span class="text-xs text-zinc-500 w-4">{{ i + 1 }}</span>
            <span class="text-sm flex-1 truncate">{{ track.title }}</span>
            <button @click="skipToNext" v-if="i === 0" class="text-xs text-emerald-500 hover:text-emerald-400" title="Pular música">
              <SkipForward class="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, X, SkipForward, Copy, Folder, Library, Search, RefreshCw } from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { uploadMusic, getRepository, getGlobalLibrary } from '../services/api'
import { useWebSocket } from '../composables/useWebSocket'
import { getSavedStations, saveStation, removeStation } from '../services/storage'
import Player from '../components/Player.vue'

const route = useRoute()
const router = useRouter()
const store = useStationStore()
const { connect, send, disconnect } = useWebSocket(null)

const activeTab = ref('repo')
const fileInputRef = ref(null)
const selectedFile = ref(null)
const uploading = ref(false)
const uploadError = ref('')

const globalTracks = ref([])
const loadingGlobal = ref(false)
const searchQuery = ref('')

const filteredGlobalTracks = computed(() => {
  if (!searchQuery.value.trim()) return globalTracks.value
  const q = searchQuery.value.toLowerCase()
  return globalTracks.value.filter(t => t.title && t.title.toLowerCase().includes(q))
})

onMounted(async () => {
  const id = route.params.stationId
  if (id) {
    store.stationId = id
    store.role = 'dj'
    if (!store.djToken) {
      const saved = getSavedStations().find(s => s.id === id)
      if (saved) store.djToken = saved.token
    }
    saveStation(id, 'dj', store.djToken)
    connect()
    try {
      const repo = await getRepository(id, store.djToken)
      store.setRepository(repo)
    } catch (e) {
      removeStation(id)
      store.reset()
      router.push('/')
    }
  }
})

async function fetchGlobalLibrary() {
  loadingGlobal.value = true
  try {
    const tracks = await getGlobalLibrary()
    globalTracks.value = tracks
  } catch (e) {
    console.error('Erro ao buscar biblioteca global:', e)
  } finally {
    loadingGlobal.value = false
  }
}

function switchTab(tab) {
  activeTab.value = tab
  if (tab === 'global' && globalTracks.value.length === 0) {
    fetchGlobalLibrary()
  }
}

function selectFile(e) {
  selectedFile.value = e.target.files[0] || null
}

async function handleUpload() {
  if (!selectedFile.value) return
  uploading.value = true
  uploadError.value = ''
  try {
    const track = await uploadMusic(store.stationId, selectedFile.value, store.djToken)
    store.repository.push(track)
    selectedFile.value = null
    if (fileInputRef.value) fileInputRef.value.value = ''
    if (globalTracks.value.length > 0) fetchGlobalLibrary()
  } catch (e) {
    const msg = e?.response?.data || e.message || 'Erro desconhecido'
    uploadError.value = typeof msg === 'string' ? msg : 'Erro no upload'
  } finally {
    uploading.value = false
  }
}

function addToPlaylist(trackId) {
  send({ type: 'ADD_TO_PLAYLIST', data: { trackId } })
}

function addGlobalTrack(track) {
  send({ type: 'ADD_TO_PLAYLIST', data: { trackId: track.id, track } })
}

function removeFromPlaylist(trackId) {
  send({ type: 'REMOVE_FROM_PLAYLIST', data: { trackId } })
}

function skipToNext() {
  send({ type: 'NEXT_TRACK' })
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

function formatTime(seconds) {
  if (!seconds || isNaN(seconds)) return ''
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
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
</script>
