<template>
  <!-- Modo DJ: renderiza o painel completo de DJ -->
  <DJDashboard v-if="store.role === 'dj'" />

  <!-- Modo Ouvinte -->
  <div v-else class="app-page">
    <header class="app-header">
      <div class="header-inner header-inner-sm">
        <div class="header-brand">
          <Disc class="icon-lg text-emerald" />
          <span class="brand-name">RadioSync</span>
          <span class="badge-code">{{ store.stationId }}</span>
          <span v-if="store.hasDJOnline" class="badge badge-live">
            <span class="status-dot animate-pulse"></span>
            DJ Ao Vivo
          </span>
          <span v-else class="badge badge-neutral">
            Sem DJ no momento
          </span>
        </div>
        <div class="header-actions">
          <!-- Botão só aparece se NÃO houver ninguém com o cargo DJ conectado no momento -->
          <button
            v-if="!store.hasDJOnline"
            @click="handleClaimDJ"
            :disabled="claimingDJ"
            class="btn btn-amber"
          >
            <Key class="icon-sm" />
            {{ claimingDJ ? 'Assumindo...' : 'Entrar como DJ' }}
          </button>
          <button @click="leave" class="btn-text">Sair</button>
        </div>
      </div>
    </header>

    <div v-if="!showPlayer" class="enter-gate">
      <div class="enter-gate-box">
        <div class="enter-gate-icon">
          <Radio class="icon-3xl text-emerald" />
        </div>
        <p class="enter-gate-text">Clique para entrar na estação</p>
        <button @click="enterStation" class="btn btn-primary btn-lg">
          Entrar na Estação
        </button>
      </div>
    </div>

    <div v-else class="listener-content">
      <div class="station-code-card">
        <p class="station-code-label">Código da estação</p>
        <div class="station-code-row">
          <span class="station-code-value">{{ store.stationId }}</span>
          <button @click="copyCode" class="btn-text copy-btn">
            <Copy class="icon-md" />
          </button>
        </div>
      </div>

      <!-- Mixer da Estação para Ouvintes -->
      <AudioMixer :is-d-j="false" />

      <!-- Seção Sugerir Músicas para Ouvintes -->
      <div class="card suggest-section">
        <div class="suggest-header">
          <h3 class="suggest-title">
            <Music class="icon-md text-emerald" />
            Sugerir Música para o DJ
          </h3>
          <span class="nickname-label">
            <User class="icon-sm" />
            Apelido:
          </span>
        </div>

        <div class="nickname-row">
          <input
            v-model="listenerName"
            @change="saveNickname"
            placeholder="Seu nome / apelido"
            class="input"
          />
        </div>

        <!-- Abas de Sugestão -->
        <div class="tabs-bar">
          <button
            @click="suggestMode = 'library'"
            class="tab-btn"
            :class="{ active: suggestMode === 'library' }"
          >
            <Library class="icon-sm" />
            Biblioteca do Servidor
          </button>
          <button
            @click="suggestMode = 'custom'"
            class="tab-btn"
            :class="{ active: suggestMode === 'custom' }"
          >
            <Youtube class="icon-sm text-red" />
            Link YouTube / Título
          </button>
        </div>

        <!-- Aba Biblioteca -->
        <div v-if="suggestMode === 'library'" class="suggest-tab-body">
          <div class="search-wrapper">
            <Search class="icon-sm search-icon" />
            <input
              v-model="searchQuery"
              placeholder="Buscar música no catálogo..."
              class="search-input"
            />
          </div>

          <div v-if="loadingLibrary" class="empty-state">
            Carregando músicas...
          </div>
          <div v-else-if="filteredLibraryTracks.length === 0" class="empty-state">
            Nenhuma música encontrada
          </div>
          <div v-else class="library-list">
            <div
              v-for="track in filteredLibraryTracks"
              :key="track.id"
              class="library-item"
            >
              <span class="library-item-title truncate">{{ track.title }}</span>
              <button
                @click="suggestLibraryTrack(track)"
                class="btn btn-primary btn-sm"
              >
                <Plus class="icon-xs" />
                Sugerir
              </button>
            </div>
          </div>
        </div>

        <!-- Aba Personalizada / YouTube -->
        <div v-else class="suggest-tab-body">
          <input
            v-model="customInput"
            @keyup.enter="suggestCustomTrack"
            placeholder="Cole a URL do YouTube ou digite o nome da música..."
            class="input"
          />
          <div class="custom-actions">
            <button
              @click="suggestCustomTrack"
              :disabled="!customInput.trim()"
              class="btn btn-primary btn-md"
            >
              <Send class="icon-sm" />
              Enviar Sugestão
            </button>
          </div>
        </div>

        <p v-if="feedbackMsg" class="feedback-banner">
          {{ feedbackMsg }}
        </p>

        <!-- Lista de Sugestões Pendentes -->
        <div v-if="store.suggestions && store.suggestions.length > 0" class="pending-suggestions">
          <h4 class="pending-title">
            <Clock class="icon-sm text-amber" />
            Sugestões na Fila de Avaliação do DJ ({{ store.suggestions.length }})
          </h4>
          <div class="pending-list">
            <div
              v-for="sug in store.suggestions"
              :key="sug.id"
              class="pending-item"
            >
              <div class="pending-info truncate">
                <span class="pending-track-title truncate">{{ sug.title }}</span>
                <span class="pending-author">Por: {{ sug.suggestedBy }}</span>
              </div>
              <span class="badge badge-amber">Pendente</span>
            </div>
          </div>
        </div>
      </div>

      <div class="card">
        <h3 class="playlist-heading">Playlist Atual</h3>
        <div v-if="store.playlist.length === 0" class="empty-state">
          Playlist vazia
        </div>
        <div
          v-for="(track, i) in store.playlist"
          :key="track.id"
          class="playlist-row"
        >
          <span class="playlist-index">{{ i + 1 }}</span>
          <span
            class="playlist-title"
            :class="{ playing: i === 0 && store.state.isPlaying }"
          >
            {{ track.title }}
          </span>
        </div>
      </div>

      <!-- Barra Fixa do Player na Parte Inferior (Estilo YouTube Music) -->
      <Player ref="playerRef" @toggle-play="handleTogglePlay" />
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Radio, Copy, Disc, Music, User, Library, Youtube, Search, Plus, Send, Clock, Key } from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { getStation, getGlobalLibrary, claimDJRole } from '../services/api'
import { useWebSocket } from '../composables/useWebSocket'
import { getSavedStations, saveStation, removeStation } from '../services/storage'
import Player from '../components/Player.vue'
import AudioMixer from '../components/AudioMixer.vue'
import DJDashboard from './DJDashboard.vue'

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

const claimingDJ = ref(false)

async function handleClaimDJ() {
  if (store.hasDJOnline) {
    alert('Já existe um DJ conectado nesta estação no momento.')
    return
  }
  claimingDJ.value = true

  try {
    const data = await claimDJRole(store.stationId)
    if (data.djToken) {
      disconnect()
      store.setStation(store.stationId, data.djToken, 'dj')
      saveStation(store.stationId, 'dj', data.djToken)
    }
  } catch (e) {
    const msg = e?.response?.data || 'Não foi possível assumir como DJ no momento.'
    alert(typeof msg === 'string' ? msg.trim() : 'Erro ao assumir cargo de DJ')
  } finally {
    claimingDJ.value = false
  }
}

onMounted(async () => {
  const id = route.params.stationId
  if (id) {
    store.stationId = id

    const queryToken = route.query.token
    let candidateToken = queryToken
    if (!candidateToken) {
      const saved = getSavedStations().find(s => s.id === id)
      if (saved && saved.role === 'dj' && saved.token) {
        candidateToken = saved.token
      }
    }

    try {
      const data = await getStation(id, candidateToken)
      store.hasDJOnline = !!data.hasDJOnline

      if (data.role === 'dj' && candidateToken) {
        store.setStation(id, candidateToken, 'dj')
        saveStation(id, 'dj', candidateToken)
        return
      }

      store.role = 'listener'
      saveStation(id, 'listener', null)

      if (data.state) store.setState(data.state)
      if (data.playlist) store.setPlaylist(data.playlist)
      if (data.suggestions) store.setSuggestions(data.suggestions)
      if (data.mixer) store.setMixer(data.mixer)
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

<style scoped>
.brand-name {
  font-weight: 700;
  color: var(--emerald-500);
}

.enter-gate {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
}

.enter-gate-box {
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 1rem;
}

.enter-gate-icon {
  width: 5rem;
  height: 5rem;
  border-radius: var(--radius-full);
  background-color: rgba(5, 150, 105, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
}

.enter-gate-text {
  color: var(--zinc-300);
}

.listener-content {
  flex: 1;
  max-width: 42rem;
  width: 100%;
  margin: 0 auto;
  padding: 1rem 1rem 6rem;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.copy-btn:hover {
  color: var(--emerald-400);
}

.suggest-section {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.suggest-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--zinc-700);
  padding-bottom: 0.75rem;
}

.suggest-title {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--zinc-200);
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.nickname-label {
  font-size: 0.75rem;
  color: var(--zinc-400);
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.nickname-row {
  display: flex;
  gap: 0.5rem;
}

.suggest-tab-body {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.library-list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  max-height: 12rem;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.library-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem;
  border-radius: var(--radius-md);
  font-size: 0.75rem;
  transition: background-color 0.15s ease;
}

.library-item:hover {
  background-color: rgba(63, 63, 70, 0.7);
}

.library-item-title {
  flex: 1;
  color: var(--zinc-200);
  margin-right: 0.5rem;
}

.custom-actions {
  display: flex;
  justify-content: flex-end;
}

.feedback-banner {
  font-size: 0.75rem;
  color: var(--emerald-400);
  font-weight: 500;
  background-color: rgba(2, 44, 34, 0.4);
  padding: 0.5rem;
  border-radius: var(--radius-md);
  border: 1px solid rgba(6, 95, 70, 0.4);
}

.pending-suggestions {
  padding-top: 0.75rem;
  border-top: 1px solid rgba(63, 63, 70, 0.6);
}

.pending-title {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--zinc-400);
  margin-bottom: 0.5rem;
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.pending-list {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  max-height: 9rem;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.pending-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem;
  border-radius: var(--radius-md);
  background-color: rgba(24, 24, 27, 0.6);
  font-size: 0.75rem;
}

.pending-info {
  flex: 1;
  margin-right: 0.5rem;
}

.pending-track-title {
  color: var(--zinc-200);
  font-weight: 500;
  display: block;
}

.pending-author {
  font-size: 0.625rem;
  color: var(--zinc-500);
}

.playlist-heading {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--zinc-400);
  margin-bottom: 0.75rem;
}

.playlist-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0;
  border-bottom: 1px solid var(--zinc-700);
}

.playlist-row:last-child {
  border-bottom: 0;
}

.playlist-index {
  font-size: 0.75rem;
  color: var(--zinc-500);
  width: 1.25rem;
}

.playlist-title {
  font-size: 0.875rem;
}

.playlist-title.playing {
  color: var(--emerald-400);
  font-weight: 600;
}
</style>
