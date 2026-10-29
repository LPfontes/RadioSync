<template>
  <div class="app-page admin-page">
    <!-- Header -->
    <header class="app-header app-header-sticky">
      <div class="header-inner header-inner-lg">
        <div class="header-brand">
          <Shield class="icon-xl text-emerald" />
          <span class="admin-brand-title">RadioSync — Painel de Administração</span>
        </div>
        <div class="header-actions">
          <router-link to="/" class="btn btn-secondary">
            Voltar ao Início
          </router-link>
        </div>
      </div>
    </header>

    <div class="admin-main">
      <!-- Notificação / Mensagem de Alerta -->
      <div v-if="actionMessage" class="alert-banner">
        <span>{{ actionMessage }}</span>
        <button @click="actionMessage = ''" class="btn-text">Fechar</button>
      </div>

      <!-- Métricas / Dashboard Stats -->
      <div class="stats-grid">
        <div class="card-bordered">
          <div class="stat-top">
            <span class="stat-label">Estações Ativas</span>
            <Radio class="icon-md text-emerald" />
          </div>
          <p class="stat-value">{{ stats.totalStations }}</p>
        </div>

        <div class="card-bordered">
          <div class="stat-top">
            <span class="stat-label">Total de Músicas no Servidor</span>
            <Music class="icon-md icon-blue" />
          </div>
          <p class="stat-value">{{ globalTracks.length || stats.totalTracks }}</p>
        </div>

        <div
          class="card-bordered"
          :class="{ 'stat-warning': stats.totalMissingFiles > 0 }"
        >
          <div class="stat-top">
            <span class="stat-label">Músicas Ausentes no Disco</span>
            <AlertTriangle
              class="icon-md"
              :class="stats.totalMissingFiles > 0 ? 'text-amber' : 'text-muted'"
            />
          </div>
          <p
            class="stat-value"
            :class="{ 'text-amber': stats.totalMissingFiles > 0 }"
          >
            {{ stats.totalMissingFiles }}
          </p>
        </div>

        <div class="card-bordered">
          <div class="stat-top">
            <span class="stat-label">Arquivo stations.json</span>
            <HardDrive class="icon-md icon-purple" />
          </div>
          <p class="stat-value">{{ formatBytes(stats.persistFileSize) }}</p>
        </div>
      </div>

      <!-- Seção: Músicas no Servidor (Biblioteca Global & Renomeação) -->
      <div class="server-library-card">
        <div class="server-library-header">
          <div class="server-library-title-group">
            <Music class="icon-lg text-emerald" />
            <h2 class="server-library-title">Músicas do Servidor (Biblioteca)</h2>
            <span class="count-pill">{{ globalTracks.length }} faixas</span>
          </div>
          <button @click="showGlobalLibrary = !showGlobalLibrary" class="toggle-link">
            {{ showGlobalLibrary ? 'Recolher' : 'Expandir' }}
          </button>
        </div>

        <div v-if="showGlobalLibrary" class="server-library-body">
          <div class="search-wrapper max-w-md">
            <Search class="icon-md search-icon" />
            <input
              v-model="globalSearchQuery"
              placeholder="Buscar música no servidor pelo nome ou ID..."
              class="search-input"
            />
          </div>

          <div v-if="loadingGlobal" class="empty-state">
            Carregando biblioteca do servidor...
          </div>
          <div v-else-if="filteredGlobalTracks.length === 0" class="empty-state empty-box">
            Nenhuma música encontrada no servidor.
          </div>
          <div v-else class="global-tracks-scroll">
            <div
              v-for="track in filteredGlobalTracks"
              :key="track.id"
              class="global-track-row"
            >
              <!-- Inline Form / View Title -->
              <div v-if="editingTrackId === track.id" class="inline-edit-row">
                <input
                  v-model="editingTrackTitle"
                  @keyup.enter="saveTrackTitle(track.id)"
                  @keyup.esc="cancelEditTrack"
                  class="inline-edit-input"
                  placeholder="Digite o nome da música..."
                  autofocus
                />
                <button
                  @click="saveTrackTitle(track.id)"
                  :disabled="savingTrack"
                  class="inline-btn save"
                  title="Salvar"
                >
                  <Check class="icon-sm" />
                </button>
                <button
                  @click="cancelEditTrack"
                  class="inline-btn cancel"
                  title="Cancelar"
                >
                  <X class="icon-sm" />
                </button>
              </div>

              <div v-else class="global-track-info">
                <Music class="icon-md text-emerald" />
                <div class="global-track-text">
                  <div class="global-track-title-row">
                    <span class="global-track-name truncate">{{ track.title }}</span>
                    <button
                      @click="startEditTrack(track)"
                      class="rename-btn"
                      title="Renomear música"
                    >
                      <Edit2 class="icon-sm" />
                    </button>
                  </div>
                  <div class="global-track-sub">
                    <span>Arquivo: {{ track.filename }}</span>
                    <span v-if="track.duration">• {{ formatTime(track.duration) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Barra de Ações Principais -->
      <div class="card-bordered actions-bar">
        <div class="actions-bar-search">
          <div class="search-wrapper">
            <Search class="icon-md search-icon" />
            <input
              v-model="searchQuery"
              placeholder="Filtrar código da estação..."
              class="search-input filter-station-input"
            />
          </div>
        </div>

        <div class="actions-bar-btns">
          <button
            @click="purgeOrphans"
            :disabled="purging"
            class="btn btn-amber btn-md"
          >
            <Trash2 class="icon-sm" />
            {{ purging ? 'Limpando...' : 'Purgar Músicas Inexistentes' }}
          </button>

          <button
            @click="loadData"
            :disabled="loading"
            class="btn btn-secondary icon-only-btn"
            title="Atualizar Dados"
          >
            <RefreshCw class="icon-md" :class="{ 'animate-spin': loading }" />
          </button>
        </div>
      </div>

      <!-- Lista de Estações -->
      <div v-if="loading && stations.length === 0" class="empty-state">
        Carregando informações das estações...
      </div>

      <div v-else-if="filteredStations.length === 0" class="card-bordered empty-state">
        Nenhuma estação encontrada.
      </div>

      <div v-else class="stations-list">
        <div
          v-for="st in filteredStations"
          :key="st.id"
          class="station-admin-card"
        >
          <!-- Cabeçalho do Card da Estação -->
          <div class="station-admin-top">
            <div class="station-admin-id-group">
              <div class="station-avatar">{{ st.id }}</div>
              <div>
                <div class="station-title-row">
                  <h3 class="station-code-heading">{{ st.id }}</h3>
                  <span v-if="st.state && st.state.isPlaying" class="status-pill playing">
                    Tocando
                  </span>
                  <span v-else class="status-pill paused">Pausado</span>
                </div>
                <div class="station-meta-row">
                  <span>{{ st.trackCount }} música(s)</span>
                  <span>•</span>
                  <span>{{ st.activeClients }} conexões ativas</span>
                  <span v-if="st.missingTrackCount > 0" class="text-amber font-medium">
                    • {{ st.missingTrackCount }} sem arquivo
                  </span>
                </div>
              </div>
            </div>

            <!-- Token e Ações Rápidas -->
            <div class="station-admin-actions">
              <!-- Copiar Token DJ -->
              <button
                @click="copyToken(st.djToken)"
                class="btn btn-secondary font-mono"
                title="Copiar Token DJ"
              >
                <Key class="icon-sm text-emerald" />
                <span>Copiar Token</span>
              </button>

              <!-- Entrar como DJ -->
              <button @click="enterAsDJ(st.id, st.djToken)" class="btn btn-primary">
                <Disc class="icon-sm" />
                Painel DJ
              </button>

              <!-- Ouvir no Painel (Ouvinte Embutido) -->
              <button
                @click="listenInAdmin(st.id)"
                class="btn btn-indigo"
                :class="{ 'ring-indigo': activeListenerStationId === st.id }"
              >
                <Headphones class="icon-sm" />
                {{ activeListenerStationId === st.id ? 'Ouvindo...' : 'Ouvir no Painel' }}
              </button>

              <!-- Página de Ouvinte Completa -->
              <button
                @click="openFullListenerPage(st.id)"
                class="btn btn-secondary"
                title="Abrir Página de Ouvinte em Modo Completo"
              >
                <ExternalLink class="icon-sm" />
                Página Ouvinte
              </button>

              <!-- Deletar Estação -->
              <button
                @click="confirmDeleteStation(st.id)"
                class="btn btn-danger-soft icon-only-sm"
                title="Excluir Estação"
              >
                <Trash2 class="icon-md" />
              </button>
            </div>
          </div>

          <!-- Músicas do Repositório da Estação (Expansível) -->
          <div class="station-repo-drawer">
            <div class="station-repo-header">
              <span class="station-repo-label">Repositório da Estação</span>
              <button @click="toggleExpand(st.id)" class="toggle-link">
                {{ expanded[st.id] ? 'Recolher' : `Ver faixas (${st.repository.length})` }}
              </button>
            </div>

            <div v-if="expanded[st.id]" class="station-repo-list">
              <div v-if="st.repository.length === 0" class="empty-state">
                Repositório vazio.
              </div>
              <div
                v-for="track in st.repository"
                :key="track.id"
                class="station-repo-item"
              >
                <!-- Inline edit track title in station repo -->
                <div v-if="editingTrackId === track.id" class="inline-edit-row">
                  <input
                    v-model="editingTrackTitle"
                    @keyup.enter="saveTrackTitle(track.id)"
                    @keyup.esc="cancelEditTrack"
                    class="inline-edit-input"
                    autofocus
                  />
                  <button
                    @click="saveTrackTitle(track.id)"
                    :disabled="savingTrack"
                    class="inline-btn save"
                  >
                    <Check class="icon-xs" />
                  </button>
                  <button @click="cancelEditTrack" class="inline-btn cancel">
                    <X class="icon-xs" />
                  </button>
                </div>

                <div v-else class="station-repo-item-left">
                  <Music class="icon-sm text-muted" />
                  <span class="truncate">{{ track.title }}</span>
                  <button
                    @click="startEditTrack(track)"
                    class="rename-btn"
                    title="Renomear música"
                  >
                    <Edit2 class="icon-xs" />
                  </button>
                  <span v-if="track.duration" class="repo-item-duration">
                    ({{ formatTime(track.duration) }})
                  </span>
                </div>

                <div>
                  <button
                    @click="removeTrack(st.id, track.id)"
                    class="remove-track-btn"
                    title="Remover da estação"
                  >
                    <X class="icon-sm" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Live Listener Player Drawer (Fixed at Bottom when Active) -->
    <div v-if="activeListenerStationId" class="live-drawer">
      <div class="live-drawer-inner">
        <div class="live-drawer-left">
          <div class="live-avatar">
            <Headphones class="icon-lg animate-pulse" />
          </div>
          <div class="live-info">
            <div class="live-badges-row">
              <span class="live-station-tag">ESTAÇÃO {{ activeListenerStationId }}</span>
              <span v-if="listenerState && listenerState.isPlaying" class="status-pill playing">
                AO VIVO
              </span>
              <span v-else class="status-pill paused">PAUSADO</span>
            </div>
            <p class="live-song-title truncate">
              {{ currentListenerSongTitle }}
            </p>
          </div>
        </div>

        <div class="live-drawer-right">
          <audio ref="audioRef" preload="auto"></audio>

          <!-- Volume Controls -->
          <div class="live-volume">
            <button @click="toggleMute" class="btn-icon">
              <VolumeX v-if="muted || volume === 0" class="icon-md text-red" />
              <Volume2 v-else class="icon-md icon-indigo" />
            </button>
            <input
              type="range"
              min="0"
              max="100"
              v-model.number="volume"
              @input="updateVolume"
              class="range-slider live-slider"
            />
          </div>

          <!-- Open Page -->
          <button
            @click="openFullListenerPage(activeListenerStationId)"
            class="btn btn-secondary"
          >
            <ExternalLink class="icon-sm" />
            Ampliar
          </button>

          <!-- Close Drawer -->
          <button
            @click="stopListener"
            class="btn btn-secondary close-drawer-btn"
            title="Parar Reprodução"
          >
            <X class="icon-md" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { Shield, Radio, Music, AlertTriangle, HardDrive, Search, RefreshCw, Trash2, Key, Disc, Headphones, X, Edit2, Check, ExternalLink, Volume2, VolumeX } from 'lucide-vue-next'
import { getAdminStations, deleteStationAdmin, purgeOrphanTracksAdmin, removeTrackFromStationAdmin, getGlobalLibrary, renameTrackAdmin } from '../services/api'
import { useStationStore } from '../stores/station'
import { saveStation } from '../services/storage'

const router = useRouter()
const store = useStationStore()

const stations = ref([])
const globalTracks = ref([])
const stats = ref({
  totalStations: 0,
  totalTracks: 0,
  totalMissingFiles: 0,
  persistFileSize: 0,
})

const loading = ref(false)
const loadingGlobal = ref(false)
const purging = ref(false)
const actionMessage = ref('')
const searchQuery = ref('')
const globalSearchQuery = ref('')
const showGlobalLibrary = ref(true)
const expanded = ref({})

const editingTrackId = ref(null)
const editingTrackTitle = ref('')
const savingTrack = ref(false)

const activeListenerStationId = ref(null)
const listenerState = ref(null)
const listenerPlaylist = ref([])
const audioRef = ref(null)
const volume = ref(80)
const muted = ref(false)
let wsSocket = null

const filteredStations = computed(() => {
  if (!searchQuery.value.trim()) return stations.value
  const q = searchQuery.value.toUpperCase()
  return stations.value.filter(s => s.id.includes(q))
})

const filteredGlobalTracks = computed(() => {
  if (!globalSearchQuery.value.trim()) return globalTracks.value
  const q = globalSearchQuery.value.toLowerCase()
  return globalTracks.value.filter(t => t.title.toLowerCase().includes(q) || t.id.toLowerCase().includes(q) || t.filename.toLowerCase().includes(q))
})

const currentListenerSongTitle = computed(() => {
  if (listenerPlaylist.value && listenerPlaylist.value.length > 0) {
    return listenerPlaylist.value[0].title
  }
  if (listenerState.value && listenerState.value.currentSong) {
    const parts = listenerState.value.currentSong.split('/')
    const fn = parts[parts.length - 1]
    return fn ? fn.replace('.opus', '') : 'Música ao Vivo'
  }
  return 'Nenhuma música em reprodução'
})

async function loadData() {
  loading.value = true
  try {
    const data = await getAdminStations()
    stations.value = data.stations || []
    stats.value = {
      totalStations: data.totalStations || 0,
      totalTracks: data.totalTracks || 0,
      totalMissingFiles: data.totalMissingFiles || 0,
      persistFileSize: data.persistFileSize || 0,
    }
  } catch (e) {
    actionMessage.value = 'Erro ao carregar dados de administração.'
  } finally {
    loading.value = false
  }
}

async function loadGlobalTracks() {
  loadingGlobal.value = true
  try {
    const tracks = await getGlobalLibrary()
    globalTracks.value = tracks || []
  } catch (e) {
    console.error('Erro ao carregar biblioteca global:', e)
  } finally {
    loadingGlobal.value = false
  }
}

function startEditTrack(track) {
  editingTrackId.value = track.id
  editingTrackTitle.value = track.title
}

function cancelEditTrack() {
  editingTrackId.value = null
  editingTrackTitle.value = ''
}

async function saveTrackTitle(trackId) {
  if (!editingTrackTitle.value.trim()) return
  savingTrack.value = true
  try {
    const res = await renameTrackAdmin(trackId, editingTrackTitle.value.trim())
    actionMessage.value = res.message || 'Nome da música atualizado com sucesso!'

    const gt = globalTracks.value.find(t => t.id === trackId || t.filename === trackId)
    if (gt) gt.title = editingTrackTitle.value.trim()

    stations.value.forEach(s => {
      s.repository.forEach(t => {
        if (t.id === trackId || t.filename === trackId) t.title = editingTrackTitle.value.trim()
      })
      s.playlist.forEach(t => {
        if (t.id === trackId || t.filename === trackId) t.title = editingTrackTitle.value.trim()
      })
    })

    cancelEditTrack()
  } catch (e) {
    actionMessage.value = 'Erro ao atualizar nome da música.'
  } finally {
    savingTrack.value = false
  }
}

async function purgeOrphans() {
  if (!confirm('Deseja remover do stations.json todos os metadados de músicas que não possuem arquivo físico no disco?')) return
  purging.value = true
  actionMessage.value = ''
  try {
    const res = await purgeOrphanTracksAdmin()
    actionMessage.value = res.message || `${res.purgedTracks} metadados órfãos foram purgados com sucesso!`
    await loadData()
    await loadGlobalTracks()
  } catch (e) {
    actionMessage.value = 'Erro ao purgar metadados órfãos.'
  } finally {
    purging.value = false
  }
}

async function confirmDeleteStation(stationId) {
  if (!confirm(`Tem certeza de que deseja excluir permanentemente a estação ${stationId}?`)) return
  try {
    await deleteStationAdmin(stationId)
    actionMessage.value = `Estação ${stationId} foi excluída.`
    if (activeListenerStationId.value === stationId) stopListener()
    await loadData()
  } catch (e) {
    actionMessage.value = `Erro ao excluir estação ${stationId}.`
  }
}

async function removeTrack(stationId, trackId) {
  try {
    await removeTrackFromStationAdmin(stationId, trackId)
    actionMessage.value = `Música removida da estação ${stationId}.`
    await loadData()
  } catch (e) {
    actionMessage.value = 'Erro ao remover música da estação.'
  }
}

function enterAsDJ(stationId, djToken) {
  store.setStation(stationId, djToken, 'dj')
  saveStation(stationId, 'dj', djToken)
  router.push(`/radio/${stationId}`)
}

function openFullListenerPage(stationId) {
  store.setStation(stationId, null, 'listener')
  saveStation(stationId, 'listener', null)
  router.push(`/radio/${stationId}?autoJoin=true`)
}

function listenInAdmin(stationId) {
  if (activeListenerStationId.value === stationId) {
    stopListener()
    return
  }

  stopListener()
  activeListenerStationId.value = stationId
  store.setStation(stationId, null, 'listener')

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = import.meta.env.VITE_WS_URL || `${protocol}//${window.location.host}`
  const wsUrl = `${host}/ws/stations/${stationId}`

  wsSocket = new WebSocket(wsUrl)
  wsSocket.onopen = () => {
    wsSocket.send(JSON.stringify({ type: 'SYNC_REQUEST' }))
  }

  wsSocket.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data)
      if (msg.type === 'STATE_UPDATE' && msg.state) {
        listenerState.value = msg.state
        playAudioCurrentSong(msg.state)
      } else if (msg.type === 'PLAYLIST_UPDATED' && msg.playlist) {
        listenerPlaylist.value = msg.playlist
      } else if (msg.type === 'SYNC') {
        if (audioRef.value && msg.position !== undefined) {
          audioRef.value.currentTime = msg.position
        }
      }
    } catch (err) {
      console.error('Erro WS Admin Listener:', err)
    }
  }
}

function playAudioCurrentSong(state) {
  if (!audioRef.value) return
  if (state.currentSong) {
    const fullUrl = state.currentSong.startsWith('/') ? window.location.origin + state.currentSong : state.currentSong
    if (audioRef.value.src !== fullUrl) {
      audioRef.value.src = fullUrl
    }
    if (state.isPlaying) {
      audioRef.value.play().catch(() => {})
    } else {
      audioRef.value.pause()
    }
  } else {
    audioRef.value.pause()
  }
}

function stopListener() {
  if (wsSocket) {
    wsSocket.close()
    wsSocket = null
  }
  if (audioRef.value) {
    audioRef.value.pause()
  }
  activeListenerStationId.value = null
  listenerState.value = null
  listenerPlaylist.value = []
}

function updateVolume() {
  if (audioRef.value) {
    audioRef.value.volume = volume.value / 100
    muted.value = volume.value === 0
  }
}

function toggleMute() {
  muted.value = !muted.value
  if (audioRef.value) {
    audioRef.value.volume = muted.value ? 0 : volume.value / 100
  }
}

function copyToken(token) {
  navigator.clipboard.writeText(token)
  actionMessage.value = 'Token DJ copiado para a área de transferência!'
}

function toggleExpand(id) {
  expanded.value[id] = !expanded.value[id]
}

function formatTime(seconds) {
  if (!seconds || isNaN(seconds)) return ''
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

function formatBytes(bytes) {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

onMounted(() => {
  loadData()
  loadGlobalTracks()
})

onUnmounted(() => {
  stopListener()
})
</script>

<style scoped>
.admin-page {
  padding-bottom: 6rem;
}

.admin-brand-title {
  font-weight: 700;
  font-size: 1.125rem;
}

.admin-main {
  flex: 1;
  max-width: 72rem;
  width: 100%;
  margin: 0 auto;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.alert-banner {
  background-color: rgba(5, 150, 105, 0.2);
  border: 1px solid rgba(16, 185, 129, 0.3);
  color: var(--emerald-300);
  padding: 0.75rem 1rem;
  border-radius: var(--radius-lg);
  font-size: 0.875rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.stats-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1rem;
}

@media (min-width: 640px) {
  .stats-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1024px) {
  .stats-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

.stat-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: var(--zinc-400);
  margin-bottom: 0.5rem;
}

.stat-label {
  font-size: 0.75rem;
  font-weight: 500;
}

.stat-value {
  font-size: 1.5rem;
  font-weight: 700;
  font-family: var(--font-mono);
}

.stat-warning {
  border-color: rgba(245, 158, 11, 0.5);
  background-color: rgba(69, 26, 3, 0.15);
}

.icon-blue {
  color: var(--blue-400);
}

.icon-purple {
  color: var(--purple-400);
}

.icon-indigo {
  color: var(--indigo-400);
}

/* Server Library Card */
.server-library-card {
  background-color: var(--zinc-800);
  border-radius: var(--radius-xl);
  border: 1px solid rgba(63, 63, 70, 0.5);
  overflow: hidden;
}

.server-library-header {
  padding: 1rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid rgba(63, 63, 70, 0.5);
}

.server-library-title-group {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.server-library-title {
  font-weight: 700;
  font-size: 1rem;
}

.count-pill {
  font-size: 0.75rem;
  background-color: var(--zinc-700);
  color: var(--zinc-300);
  padding: 0.125rem 0.5rem;
  border-radius: var(--radius-full);
  font-family: var(--font-mono);
}

.toggle-link {
  font-size: 0.75rem;
  color: var(--emerald-400);
}

.toggle-link:hover {
  text-decoration: underline;
}

.server-library-body {
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.max-w-md {
  max-width: 28rem;
}

.empty-box {
  background-color: rgba(24, 24, 27, 0.4);
  border-radius: var(--radius-lg);
}

.global-tracks-scroll {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  max-height: 16rem;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.global-track-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.75rem;
  border-radius: var(--radius-lg);
  background-color: rgba(24, 24, 27, 0.6);
  font-size: 0.75rem;
  transition: background-color 0.15s ease;
}

.global-track-row:hover {
  background-color: rgba(63, 63, 70, 0.4);
}

.inline-edit-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex: 1;
  margin-right: 0.5rem;
}

.inline-edit-input {
  flex: 1;
  background-color: var(--zinc-800);
  border: 1px solid var(--emerald-500);
  border-radius: var(--radius-sm);
  padding: 0.25rem 0.625rem;
  font-size: 0.75rem;
  color: #ffffff;
  outline: none;
}

.inline-btn {
  padding: 0.25rem;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
}

.inline-btn.save {
  background-color: var(--emerald-600);
  color: #ffffff;
}

.inline-btn.save:hover {
  background-color: var(--emerald-500);
}

.inline-btn.cancel {
  background-color: var(--zinc-700);
  color: var(--zinc-300);
}

.inline-btn.cancel:hover {
  background-color: var(--zinc-600);
}

.global-track-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  min-width: 0;
  flex: 1;
}

.global-track-text {
  min-width: 0;
  flex: 1;
}

.global-track-title-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.global-track-name {
  font-weight: 500;
  color: var(--zinc-100);
}

.rename-btn {
  color: var(--zinc-400);
  padding: 0.125rem;
  display: flex;
  align-items: center;
  transition: color 0.15s ease;
}

.rename-btn:hover {
  color: var(--emerald-400);
}

.global-track-sub {
  font-size: 0.625rem;
  color: var(--zinc-500);
  font-family: var(--font-mono);
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

/* Actions Bar */
.actions-bar {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

@media (min-width: 640px) {
  .actions-bar {
    flex-direction: row;
  }
}

.actions-bar-search {
  width: 100%;
}

@media (min-width: 640px) {
  .actions-bar-search {
    width: 16rem;
  }
}

.filter-station-input {
  padding-top: 0.5rem;
  padding-bottom: 0.5rem;
  text-transform: uppercase;
  font-family: var(--font-mono);
}

.actions-bar-btns {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem;
  width: 100%;
}

@media (min-width: 640px) {
  .actions-bar-btns {
    width: auto;
  }
}

.icon-only-btn {
  padding: 0.5rem;
}

/* Stations List */
.stations-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.station-admin-card {
  background-color: var(--zinc-800);
  border-radius: var(--radius-xl);
  border: 1px solid rgba(63, 63, 70, 0.5);
  overflow: hidden;
}

.station-admin-top {
  padding: 1rem;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 1rem;
  border-bottom: 1px solid rgba(63, 63, 70, 0.5);
}

@media (min-width: 768px) {
  .station-admin-top {
    flex-direction: row;
    align-items: center;
  }
}

.station-admin-id-group {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.station-avatar {
  width: 2.5rem;
  height: 2.5rem;
  border-radius: var(--radius-lg);
  background-color: rgba(5, 150, 105, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: var(--font-mono);
  font-weight: 700;
  color: var(--emerald-400);
  font-size: 1rem;
}

.station-title-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.station-code-heading {
  font-weight: 700;
  font-size: 1rem;
  font-family: var(--font-mono);
  letter-spacing: 0.05em;
}

.status-pill {
  padding: 0.125rem 0.5rem;
  font-size: 0.625rem;
  font-weight: 500;
  border-radius: var(--radius-full);
  text-transform: uppercase;
}

.status-pill.playing {
  background-color: rgba(16, 185, 129, 0.2);
  color: var(--emerald-400);
}

.status-pill.paused {
  background-color: var(--zinc-700);
  color: var(--zinc-400);
}

.station-meta-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-size: 0.75rem;
  color: var(--zinc-400);
  margin-top: 0.125rem;
  flex-wrap: wrap;
}

.font-medium {
  font-weight: 500;
}

.font-mono {
  font-family: var(--font-mono);
}

.station-admin-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.ring-indigo {
  box-shadow: 0 0 0 2px var(--indigo-400);
}

.icon-only-sm {
  padding: 0.375rem;
}

/* Station Repo Drawer */
.station-repo-drawer {
  padding: 1rem;
  background-color: rgba(39, 39, 42, 0.5);
}

.station-repo-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.station-repo-label {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--zinc-400);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.station-repo-list {
  margin-top: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  max-height: 12rem;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.station-repo-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.375rem 0.5rem;
  border-radius: var(--radius-sm);
  background-color: rgba(24, 24, 27, 0.4);
  font-size: 0.75rem;
  transition: background-color 0.15s ease;
}

.station-repo-item:hover {
  background-color: rgba(63, 63, 70, 0.5);
}

.station-repo-item-left {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.repo-item-duration {
  font-size: 0.625rem;
  color: var(--zinc-500);
  font-family: var(--font-mono);
}

.remove-track-btn {
  color: var(--red-400);
  padding: 0.25rem;
  display: flex;
  align-items: center;
}

.remove-track-btn:hover {
  color: var(--red-300);
}

/* Live Listener Drawer */
.live-drawer {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  background-color: rgba(9, 9, 11, 0.92);
  border-top: 1px solid rgba(99, 102, 241, 0.5);
  box-shadow: 0 -10px 25px rgba(0, 0, 0, 0.6);
  padding: 0.75rem;
  z-index: 50;
  backdrop-filter: blur(12px);
}

.live-drawer-inner {
  max-width: 72rem;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

@media (min-width: 640px) {
  .live-drawer-inner {
    flex-direction: row;
  }
}

.live-drawer-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
}

@media (min-width: 640px) {
  .live-drawer-left {
    width: auto;
  }
}

.live-avatar {
  width: 2.5rem;
  height: 2.5rem;
  border-radius: var(--radius-full);
  background-color: rgba(79, 70, 229, 0.3);
  border: 1px solid rgba(99, 102, 241, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--indigo-400);
  flex-shrink: 0;
}

.live-info {
  min-width: 0;
}

.live-badges-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.live-station-tag {
  font-size: 0.75rem;
  font-family: var(--font-mono);
  background-color: rgba(49, 46, 129, 0.6);
  color: var(--indigo-300);
  padding: 0.125rem 0.5rem;
  border-radius: var(--radius-sm);
  font-weight: 700;
}

.live-song-title {
  font-size: 0.75rem;
  color: var(--zinc-200);
  max-width: 24rem;
  margin-top: 0.125rem;
}

.live-drawer-right {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 1rem;
  width: 100%;
}

@media (min-width: 640px) {
  .live-drawer-right {
    width: auto;
  }
}

.live-volume {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.live-slider {
  width: 5rem;
  accent-color: var(--indigo-500);
}

.live-slider::-webkit-slider-thumb {
  background-color: var(--indigo-400);
}

.live-slider::-moz-range-thumb {
  background-color: var(--indigo-400);
}

.close-drawer-btn:hover {
  background-color: rgba(220, 38, 38, 0.3);
  color: var(--red-400);
}
</style>
