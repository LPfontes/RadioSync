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
        <div class="bg-zinc-800 rounded-lg p-4">
          <h3 class="text-sm font-medium text-zinc-400 mb-3">Upload</h3>
          <div class="space-y-3">
            <input type="file" ref="fileInputRef" accept=".mp3,.mp4,.wav,.ogg,.flac,.aac,.m4a" @change="selectFile" class="block w-full text-sm text-zinc-400 file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0 file:text-sm file:font-medium file:bg-zinc-700 file:text-zinc-200 hover:file:bg-zinc-600" />
            <div v-if="selectedFile" class="flex items-center gap-2 text-sm text-zinc-300">
              <span class="flex-1 truncate">{{ selectedFile.name }}</span>
              <button @click="handleUpload" :disabled="uploading" class="px-4 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:bg-zinc-600 disabled:opacity-50 rounded-lg text-xs font-medium transition-colors">
                {{ uploading ? 'Enviando...' : 'Upload' }}
              </button>
            </div>
            <p v-if="uploadError" class="text-xs text-red-400">{{ uploadError }}</p>
          </div>

          <div v-if="store.repository.length > 0" class="mt-4 space-y-1">
            <p class="text-xs text-zinc-500 mb-2">Repositório (clique para adicionar à playlist)</p>
            <div v-for="track in store.repository" :key="track.id" @click="addToPlaylist(track.id)" class="flex items-center gap-2 py-1.5 px-2 rounded hover:bg-zinc-700 cursor-pointer text-sm">
              <Plus class="w-3.5 h-3.5 text-emerald-500" />
              <span>{{ track.title }}</span>
            </div>
          </div>
        </div>

        <div class="bg-zinc-800 rounded-lg p-4">
          <h3 class="text-sm font-medium text-zinc-400 mb-3">Playlist</h3>
          <div v-if="store.playlist.length === 0" class="text-zinc-500 text-sm text-center py-4">
            Playlist vazia — clique em uma música do repositório
          </div>
          <div v-for="(track, i) in store.playlist" :key="track.id" class="flex items-center gap-3 py-2 border-b border-zinc-700 last:border-0">
            <button @click="removeFromPlaylist(track.id)" class="text-red-400 hover:text-red-300">
              <X class="w-3.5 h-3.5" />
            </button>
            <span class="text-xs text-zinc-500 w-4">{{ i + 1 }}</span>
            <span class="text-sm flex-1">{{ track.title }}</span>
            <button @click="skipToNext" v-if="i === 0" class="text-xs text-emerald-500 hover:text-emerald-400">
              <SkipForward class="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, X, SkipForward, Copy } from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { uploadMusic, getRepository } from '../services/api'
import { useWebSocket } from '../composables/useWebSocket'
import { saveStation, removeStation } from '../services/storage'
import Player from '../components/Player.vue'

const route = useRoute()
const router = useRouter()
const store = useStationStore()
const { connect, send, disconnect } = useWebSocket()
const fileInputRef = ref(null)
const selectedFile = ref(null)
const uploading = ref(false)
const uploadError = ref('')

onMounted(async () => {
  const id = route.params.stationId
  if (id) {
    store.stationId = id
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
