<template>
  <div class="min-h-screen bg-zinc-900 text-zinc-100 flex flex-col">
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
            <span class="text-xs font-medium">Total de Músicas</span>
            <Music class="w-4 h-4 text-blue-400" />
          </div>
          <p class="text-2xl font-bold font-mono">{{ stats.totalTracks }}</p>
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
              <!-- Botão Copiar Token DJ -->
              <button @click="copyToken(st.djToken)" class="px-3 py-1.5 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-xs font-mono text-zinc-300 flex items-center gap-1.5 transition-colors" title="Copiar Token DJ">
                <Key class="w-3.5 h-3.5 text-emerald-400" />
                <span>Copiar Token DJ</span>
              </button>

              <!-- Entrar como DJ -->
              <button @click="enterAsDJ(st.id, st.djToken)" class="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 rounded-lg text-xs font-medium text-white flex items-center gap-1.5 transition-colors">
                <Disc class="w-3.5 h-3.5" />
                Painel DJ
              </button>

              <!-- Entrar como Ouvinte -->
              <button @click="enterAsListener(st.id)" class="px-3 py-1.5 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-xs font-medium text-zinc-200 flex items-center gap-1.5 transition-colors">
                <Headphones class="w-3.5 h-3.5" />
                Ouvir
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
                <div class="flex items-center gap-2 min-w-0">
                  <Music class="w-3.5 h-3.5 text-zinc-500 shrink-0" />
                  <span class="truncate">{{ track.title }}</span>
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
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Shield, Radio, Music, AlertTriangle, HardDrive, Search, RefreshCw, Trash2, Key, Disc, Headphones, X } from 'lucide-vue-next'
import { getAdminStations, deleteStationAdmin, purgeOrphanTracksAdmin, removeTrackFromStationAdmin } from '../services/api'
import { useStationStore } from '../stores/station'
import { saveStation } from '../services/storage'

const router = useRouter()
const store = useStationStore()

const stations = ref([])
const stats = ref({
  totalStations: 0,
  totalTracks: 0,
  totalMissingFiles: 0,
  persistFileSize: 0,
})

const loading = ref(false)
const purging = ref(false)
const actionMessage = ref('')
const searchQuery = ref('')
const expanded = ref({})

const filteredStations = computed(() => {
  if (!searchQuery.value.trim()) return stations.value
  const q = searchQuery.value.toUpperCase()
  return stations.value.filter(s => s.id.includes(q))
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

async function purgeOrphans() {
  if (!confirm('Deseja remover do stations.json todos os metadados de músicas que não possuem arquivo físico no disco?')) return
  purging.value = true
  actionMessage.value = ''
  try {
    const res = await purgeOrphanTracksAdmin()
    actionMessage.value = res.message || `${res.purgedTracks} metadados órfãos foram purgados com sucesso!`
    await loadData()
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

function enterAsListener(stationId) {
  store.setStation(stationId, null, 'listener')
  saveStation(stationId, 'listener', null)
  router.push(`/radio/${stationId}`)
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
})
</script>
