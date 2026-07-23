<template>
  <div class="min-h-screen bg-zinc-900 text-zinc-100 flex flex-col pb-24">
    <!-- Header -->
    <header class="border-b border-zinc-800 px-4 py-3 bg-zinc-900/80 backdrop-blur sticky top-0 z-10">
      <div class="max-w-6xl mx-auto flex items-center justify-between">
        <div class="flex items-center gap-3">
          <Shield class="w-6 h-6 text-emerald-500" />
          <span class="font-bold text-lg">RadioSync — Painel de Administração</span>
        </div>
        <div class="flex items-center gap-2">
          <router-link to="/" class="px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 rounded-lg text-xs font-medium transition-colors">
            Voltar ao Início
          </router-link>
        </div>
      </div>
    </header>

    <div class="flex-1 max-w-6xl w-full mx-auto p-4 space-y-6">
      <!-- Notificação / Mensagem de Alerta -->
      <div v-if="actionMessage" class="bg-emerald-600/20 border border-emerald-500/30 text-emerald-300 px-4 py-3 rounded-lg text-sm flex items-center justify-between">
        <span>{{ actionMessage }}</span>
        <button @click="actionMessage = ''" class="text-xs text-zinc-400 hover:text-white">Fechar</button>
      </div>

      <!-- Métricas / Dashboard Stats -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div class="bg-zinc-800 rounded-xl p-4 border border-zinc-700/50">
          <div class="flex items-center justify-between text-zinc-400 mb-2">
            <span class="text-xs font-medium">Estações Ativas</span>
            <Radio class="w-4 h-4 text-emerald-400" />
          </div>
          <p class="text-2xl font-bold font-mono">{{ stats.totalStations }}</p>
        </div>

        <div class="bg-zinc-800 rounded-xl p-4 border border-zinc-700/50">
          <div class="flex items-center justify-between text-zinc-400 mb-2">
            <span class="text-xs font-medium">Total de Músicas no Servidor</span>
            <Music class="w-4 h-4 text-blue-400" />
          </div>
          <p class="text-2xl font-bold font-mono">{{ globalTracks.length || stats.totalTracks }}</p>
        </div>

        <div class="bg-zinc-800 rounded-xl p-4 border" :class="stats.totalMissingFiles > 0 ? 'border-amber-500/50 bg-amber-950/10' : 'border-zinc-700/50'">
          <div class="flex items-center justify-between text-zinc-400 mb-2">
            <span class="text-xs font-medium">Músicas Ausentes no Disco</span>
            <AlertTriangle class="w-4 h-4" :class="stats.totalMissingFiles > 0 ? 'text-amber-400' : 'text-zinc-500'" />
          </div>
          <p class="text-2xl font-bold font-mono" :class="stats.totalMissingFiles > 0 ? 'text-amber-400' : 'text-zinc-100'">
            {{ stats.totalMissingFiles }}
          </p>
        </div>

        <div class="bg-zinc-800 rounded-xl p-4 border border-zinc-700/50">
          <div class="flex items-center justify-between text-zinc-400 mb-2">
            <span class="text-xs font-medium">Arquivo stations.json</span>
            <HardDrive class="w-4 h-4 text-purple-400" />
          </div>
          <p class="text-2xl font-bold font-mono">{{ formatBytes(stats.persistFileSize) }}</p>
        </div>
      </div>

      <!-- Seção: Músicas no Servidor (Biblioteca Global & Renomeação) -->
      <div class="bg-zinc-800 rounded-xl border border-zinc-700/50 overflow-hidden">
        <div class="p-4 flex items-center justify-between border-b border-zinc-700/50 bg-zinc-800/80">
          <div class="flex items-center gap-2">
            <Music class="w-5 h-5 text-emerald-400" />
            <h2 class="font-bold text-base">Músicas do Servidor (Biblioteca)</h2>
            <span class="text-xs bg-zinc-700 text-zinc-300 px-2 py-0.5 rounded-full font-mono">{{ globalTracks.length }} faixas</span>
          </div>
          <button @click="showGlobalLibrary = !showGlobalLibrary" class="text-xs text-emerald-400 hover:underline">
            {{ showGlobalLibrary ? 'Recolher' : 'Expandir' }}
          </button>
        </div>

        <div v-if="showGlobalLibrary" class="p-4 space-y-3">
          <div class="relative max-w-md">
            <Search class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-zinc-400" />
            <input v-model="globalSearchQuery" placeholder="Buscar música no servidor pelo nome ou ID..." class="w-full bg-zinc-700 rounded-lg pl-9 pr-3 py-1.5 text-xs outline-none focus:ring-1 focus:ring-emerald-500" />
          </div>

          <div v-if="loadingGlobal" class="text-center py-6 text-zinc-500 text-xs">
            Carregando biblioteca do servidor...
          </div>
          <div v-else-if="filteredGlobalTracks.length === 0" class="text-center py-6 text-zinc-500 text-xs bg-zinc-900/40 rounded-lg">
            Nenhuma música encontrada no servidor.
          </div>
          <div v-else class="space-y-1.5 max-h-64 overflow-y-auto pr-1">
            <div v-for="track in filteredGlobalTracks" :key="track.id" class="flex items-center justify-between py-2 px-3 rounded-lg bg-zinc-900/60 hover:bg-zinc-700/40 text-xs transition-colors">
              <!-- Inline Form / View Title -->
              <div v-if="editingTrackId === track.id" class="flex items-center gap-2 flex-1 mr-2">
                <input v-model="editingTrackTitle" @keyup.enter="saveTrackTitle(track.id)" @keyup.esc="cancelEditTrack" class="flex-1 bg-zinc-800 border border-emerald-500 rounded px-2.5 py-1 text-xs text-white outline-none" placeholder="Digite o nome da música..." autofocus />
                <button @click="saveTrackTitle(track.id)" :disabled="savingTrack" class="p-1 bg-emerald-600 hover:bg-emerald-500 text-white rounded" title="Salvar">
                  <Check class="w-3.5 h-3.5" />
                </button>
                <button @click="cancelEditTrack" class="p-1 bg-zinc-700 hover:bg-zinc-600 text-zinc-300 rounded" title="Cancelar">
                  <X class="w-3.5 h-3.5" />
                </button>
              </div>

              <div v-else class="flex items-center gap-3 min-w-0 flex-1">
                <Music class="w-4 h-4 text-emerald-400 shrink-0" />
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span class="font-medium text-zinc-100 truncate">{{ track.title }}</span>
                    <button @click="startEditTrack(track)" class="text-zinc-400 hover:text-emerald-400 transition-colors p-0.5" title="Renomear música">
                      <Edit2 class="w-3.5 h-3.5" />
                    </button>
                  </div>
                  <div class="text-[10px] text-zinc-500 font-mono flex items-center gap-2">
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
      <div class="bg-zinc-800 rounded-xl p-4 flex flex-col sm:flex-row items-center justify-between gap-4 border border-zinc-700/50">
        <div class="flex items-center gap-3 w-full sm:w-auto">
          <div class="relative flex-1 sm:w-64">
            <Search class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-zinc-400" />
            <input v-model="searchQuery" placeholder="Filtrar código da estação..." class="w-full bg-zinc-700 rounded-lg pl-9 pr-3 py-2 text-xs outline-none focus:ring-1 focus:ring-emerald-500 uppercase font-mono" />
          </div>
        </div>

        <div class="flex items-center gap-2 w-full sm:w-auto justify-end">
          <button @click="purgeOrphans" :disabled="purging" class="px-4 py-2 bg-amber-600 hover:bg-amber-500 disabled:opacity-50 rounded-lg text-xs font-medium transition-colors flex items-center gap-2">
            <Trash2 class="w-3.5 h-3.5" />
            {{ purging ? 'Limpando...' : 'Purgar Músicas Inexistentes' }}
          </button>

          <button @click="loadData" :disabled="loading" class="p-2 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-zinc-300 transition-colors" title="Atualizar Dados">
            <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
          </button>
        </div>
      </div>

      <!-- Lista de Estações -->
      <div v-if="loading && stations.length === 0" class="text-center py-12 text-zinc-500 text-sm">
        Carregando informações das estações...
      </div>

      <div v-else-if="filteredStations.length === 0" class="text-center py-12 text-zinc-500 text-sm bg-zinc-800 rounded-xl">
        Nenhuma estação encontrada.
      </div>

      <div v-else class="space-y-4">
        <div v-for="st in filteredStations" :key="st.id" class="bg-zinc-800 rounded-xl border border-zinc-700/50 overflow-hidden">
          <!-- Cabeçalho do Card da Estação -->
          <div class="p-4 flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-zinc-700/50">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-lg bg-emerald-600/20 flex items-center justify-center font-mono font-bold text-emerald-400 text-base">
                {{ st.id }}
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-bold text-base font-mono tracking-wider">{{ st.id }}</h3>
                  <span v-if="st.state && st.state.isPlaying" class="px-2 py-0.5 bg-emerald-500/20 text-emerald-400 text-[10px] font-medium rounded-full uppercase">Tocando</span>
                  <span v-else class="px-2 py-0.5 bg-zinc-700 text-zinc-400 text-[10px] font-medium rounded-full uppercase">Pausado</span>
                </div>
                <div class="flex items-center gap-3 text-xs text-zinc-400 mt-0.5">
                  <span>{{ st.trackCount }} música(s)</span>
                  <span>•</span>
                  <span>{{ st.activeClients }} conexões ativas</span>
                  <span v-if="st.missingTrackCount > 0" class="text-amber-400 font-medium">• {{ st.missingTrackCount }} sem arquivo</span>
                </div>
              </div>
            </div>

            <!-- Token e Ações Rápidas -->
            <div class="flex items-center gap-2 flex-wrap">
              <!-- Copiar Token DJ -->
              <button @click="copyToken(st.djToken)" class="px-3 py-1.5 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-xs font-mono text-zinc-300 flex items-center gap-1.5 transition-colors" title="Copiar Token DJ">
                <Key class="w-3.5 h-3.5 text-emerald-400" />
                <span>Copiar Token</span>
              </button>

              <!-- Entrar como DJ -->
              <button @click="enterAsDJ(st.id, st.djToken)" class="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 rounded-lg text-xs font-medium text-white flex items-center gap-1.5 transition-colors">
                <Disc class="w-3.5 h-3.5" />
                Painel DJ
              </button>

              <!-- Ouvir no Painel (Ouvinte Embutido) -->
              <button @click="listenInAdmin(st.id)" class="px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 rounded-lg text-xs font-medium text-white flex items-center gap-1.5 transition-colors" :class="{ 'ring-2 ring-indigo-400': activeListenerStationId === st.id }">
                <Headphones class="w-3.5 h-3.5" />
                {{ activeListenerStationId === st.id ? 'Ouvindo...' : 'Ouvir no Painel' }}
              </button>

              <!-- Página de Ouvinte Completa -->
              <button @click="openFullListenerPage(st.id)" class="px-3 py-1.5 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-xs font-medium text-zinc-200 flex items-center gap-1.5 transition-colors" title="Abrir Página de Ouvinte em Modo Completo">
                <ExternalLink class="w-3.5 h-3.5 text-zinc-400" />
                Página Ouvinte
              </button>

              <!-- Deletar Estação -->
              <button @click="confirmDeleteStation(st.id)" class="p-1.5 bg-red-600/20 hover:bg-red-600/40 text-red-400 rounded-lg transition-colors" title="Excluir Estação">
                <Trash2 class="w-4 h-4" />
              </button>
            </div>
          </div>

          <!-- Músicas do Repositório da Estação (Expansível) -->
          <div class="p-4 bg-zinc-800/50">
            <div class="flex items-center justify-between mb-2">
              <span class="text-xs font-medium text-zinc-400 uppercase tracking-wide">Repositório da Estação</span>
              <button @click="toggleExpand(st.id)" class="text-xs text-emerald-400 hover:underline">
                {{ expanded[st.id] ? 'Recolher' : `Ver faixas (${st.repository.length})` }}
              </button>
            </div>

            <div v-if="expanded[st.id]" class="mt-2 space-y-1 max-h-48 overflow-y-auto pr-1">
              <div v-if="st.repository.length === 0" class="text-xs text-zinc-500 py-2 text-center">
                Repositório vazio.
              </div>
              <div v-for="track in st.repository" :key="track.id" class="flex items-center justify-between py-1.5 px-2 rounded bg-zinc-900/40 hover:bg-zinc-700/50 text-xs">
                <!-- Inline edit track title in station repo -->
                <div v-if="editingTrackId === track.id" class="flex items-center gap-2 flex-1 mr-2">
                  <input v-model="editingTrackTitle" @keyup.enter="saveTrackTitle(track.id)" @keyup.esc="cancelEditTrack" class="flex-1 bg-zinc-800 border border-emerald-500 rounded px-2 py-0.5 text-xs text-white outline-none" autofocus />
                  <button @click="saveTrackTitle(track.id)" :disabled="savingTrack" class="p-1 bg-emerald-600 hover:bg-emerald-500 text-white rounded">
                    <Check class="w-3 h-3" />
                  </button>
                  <button @click="cancelEditTrack" class="p-1 bg-zinc-700 hover:bg-zinc-600 text-zinc-300 rounded">
                    <X class="w-3 h-3" />
                  </button>
                </div>

                <div v-else class="flex items-center gap-2 min-w-0">
                  <Music class="w-3.5 h-3.5 text-zinc-500 shrink-0" />
                  <span class="truncate">{{ track.title }}</span>
                  <button @click="startEditTrack(track)" class="text-zinc-500 hover:text-emerald-400 p-0.5" title="Renomear música">
                    <Edit2 class="w-3 h-3" />
                  </button>
                  <span v-if="track.duration" class="text-[10px] text-zinc-500 font-mono">({{ formatTime(track.duration) }})</span>
                </div>

                <div class="flex items-center gap-2">
                  <button @click="removeTrack(st.id, track.id)" class="text-red-400 hover:text-red-300 p-1" title="Remover da estação">
                    <X class="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Live Listener Player Drawer (Fixed at Bottom when Active) -->
    <div v-if="activeListenerStationId" class="fixed bottom-0 left-0 right-0 bg-zinc-950 border-t border-indigo-500/50 shadow-2xl p-3 z-50 backdrop-blur-md">
      <div class="max-w-6xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-3">
        <div class="flex items-center gap-3 w-full sm:w-auto">
          <div class="w-10 h-10 rounded-full bg-indigo-600/30 border border-indigo-500/50 flex items-center justify-center text-indigo-400 shrink-0">
            <Headphones class="w-5 h-5 animate-pulse" />
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="text-xs font-mono bg-indigo-900/60 text-indigo-300 px-2 py-0.5 rounded font-bold">ESTAÇÃO {{ activeListenerStationId }}</span>
              <span v-if="listenerState && listenerState.isPlaying" class="text-[10px] bg-emerald-500/20 text-emerald-400 px-2 py-0.5 rounded-full font-medium uppercase">AO VIVO</span>
              <span v-else class="text-[10px] bg-zinc-800 text-zinc-400 px-2 py-0.5 rounded-full font-medium uppercase">PAUSADO</span>
            </div>
            <p class="text-xs text-zinc-200 truncate max-w-xs sm:max-w-md mt-0.5">
              {{ currentListenerSongTitle }}
            </p>
          </div>
        </div>

        <div class="flex items-center gap-4 w-full sm:w-auto justify-end">
          <audio ref="audioRef" preload="auto"></audio>

          <!-- Volume Controls -->
          <div class="flex items-center gap-2">
            <button @click="toggleMute" class="text-zinc-400 hover:text-white p-1">
              <VolumeX v-if="muted || volume === 0" class="w-4 h-4 text-red-400" />
              <Volume2 v-else class="w-4 h-4 text-indigo-400" />
            </button>
            <input type="range" min="0" max="100" v-model.number="volume" @input="updateVolume" class="w-20 h-1.5 bg-zinc-700 rounded-full appearance-none cursor-pointer accent-indigo-500" />
          </div>

          <!-- Open Page -->
          <button @click="openFullListenerPage(activeListenerStationId)" class="px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 rounded-lg text-xs font-medium text-zinc-300 flex items-center gap-1">
            <ExternalLink class="w-3.5 h-3.5" />
            Ampliar
          </button>

          <!-- Close Drawer -->
          <button @click="stopListener" class="p-1.5 bg-zinc-800 hover:bg-red-600/30 text-zinc-400 hover:text-red-400 rounded-lg transition-colors" title="Parar Reprodução">
            <X class="w-4 h-4" />
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

// Editing track title
const editingTrackId = ref(null)
const editingTrackTitle = ref('')
const savingTrack = ref(false)

// Live listener inside Admin
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
    
    // Update locally in globalTracks
    const gt = globalTracks.value.find(t => t.id === trackId || t.filename === trackId)
    if (gt) gt.title = editingTrackTitle.value.trim()

    // Update locally in stations repositories & playlists
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
  router.push(`/dj/${stationId}`)
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
