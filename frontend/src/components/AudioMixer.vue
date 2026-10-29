<template>
  <div class="mixer-panel card">
    <!-- Barra Superior do Mixer -->
    <div class="mixer-header">
      <div class="mixer-header-left">
        <div class="mixer-icon-box">
          <Sliders class="icon-md" />
        </div>
        <div>
          <div class="mixer-title-row">
            <h3 class="mixer-title">Mixer de Efeitos & Ambiente</h3>
            <span v-if="activeChannelsCount > 0" class="mixer-badge active">
              <span class="status-dot animate-pulse"></span>
              {{ activeChannelsCount }} ativo{{ activeChannelsCount > 1 ? 's' : '' }}
            </span>
            <span v-else class="mixer-badge inactive">Inativo</span>
          </div>
          <p class="mixer-subtitle">
            {{ isDJ ? 'Mixe e transmita sons de fundo para toda a estação' : 'Sons de fundo mixados pela rádio' }}
          </p>
        </div>
      </div>

      <div class="mixer-header-right">
        <!-- Modo Ouvinte: Toggle de Ajustes Pessoais -->
        <div v-if="!isDJ && store.mixer && store.mixer.length > 0" class="personal-mix-box">
          <span class="personal-mix-label">Ajuste Local</span>
          <button
            @click="togglePersonalMix"
            :class="{ enabled: personalMixEnabled }"
            class="switch-btn"
            title="Personalizar volumes no seu navegador sem mudar a rádio"
          >
            <div class="switch-thumb"></div>
          </button>
        </div>

        <!-- Botão Adicionar Som (DJ) -->
        <button
          v-if="isDJ"
          @click="showAddModal = true"
          class="btn btn-primary"
        >
          <Plus class="icon-sm" />
          <span>Adicionar Som</span>
        </button>

        <!-- Botão Minimizar/Expandir -->
        <button
          @click="isExpanded = !isExpanded"
          class="btn-icon"
          :title="isExpanded ? 'Minimizar Mixer' : 'Expandir Mixer'"
        >
          <span v-if="isExpanded">▲</span>
          <span v-else>▼</span>
        </button>
      </div>
    </div>

    <!-- Conteúdo Expandido do Mixer -->
    <div v-show="isExpanded" class="mixer-body">
      <!-- Mensagem quando não há canais ativos -->
      <div v-if="!store.mixer || store.mixer.length === 0" class="mixer-empty">
        <div class="mixer-empty-icon">
          <SlidersHorizontal class="icon-xl" />
        </div>
        <p class="mixer-empty-title">Nenhum som de fundo no mixer</p>
        <p class="mixer-empty-desc">
          {{ isDJ
            ? 'Adicione sons de ambiente como chuva, cafeteria com conversas, lareira crepitante ou músicas da biblioteca para tocar simultaneamente na estação.'
            : 'O DJ da rádio ainda não ativou sons de fundo ou ambientação nesta estação.'
          }}
        </p>
        <div v-if="isDJ" class="quick-presets-row">
          <button
            v-for="preset in quickPresets"
            :key="preset.key"
            @click="addPresetChannel(preset)"
            class="quick-preset-btn"
          >
            <component :is="getPresetIcon(preset.key)" class="icon-sm text-emerald" />
            <span>+ {{ preset.name }}</span>
          </button>
        </div>
      </div>

      <!-- Grade de Canais de Som -->
      <div v-else class="mixer-channels-section">
        <!-- Barra de Ações Rápidas (DJ) -->
        <div v-if="isDJ" class="mixer-toolbar">
          <span class="mixer-toolbar-label">Canais Sincronizados na Rádio:</span>
          <div class="mixer-toolbar-actions">
            <button
              @click="muteAllChannels"
              class="toolbar-link amber-hover"
              title="Alternar mudo em todos os canais de fundo"
            >
              {{ areAllMuted ? 'Desmutar Todos' : 'Mutar Todos' }}
            </button>
            <span class="toolbar-sep">|</span>
            <button
              @click="clearMixer"
              class="toolbar-link red-hover"
              title="Remover todos os canais de efeitos"
            >
              Limpar Mixer
            </button>
          </div>
        </div>

        <div class="channels-grid">
          <!-- Card de Canal Individual -->
          <div
            v-for="ch in store.mixer"
            :key="ch.id"
            class="channel-card"
            :class="ch.isPlaying && !ch.muted ? 'active' : 'muted'"
          >
            <!-- Topo do Canal: Ícone, Nome, VU animado e Ações -->
            <div class="channel-top">
              <div class="channel-info-row">
                <div
                  class="channel-icon"
                  :class="ch.isPlaying && !ch.muted ? 'active' : 'inactive'"
                >
                  <component :is="getChannelIcon(ch)" class="icon-sm" />
                </div>
                <div class="channel-meta">
                  <div class="channel-name-row">
                    <span class="channel-name truncate">{{ ch.name }}</span>
                    <span v-if="ch.type === 'preset'" class="channel-tag preset">Ambiente</span>
                    <span v-else class="channel-tag track">Faixa</span>
                    <button
                      v-if="isDJ"
                      @click="toggleChannelLoop(ch)"
                      type="button"
                      class="loop-pill clickable"
                      :class="ch.loop ? 'loop-on' : 'loop-once'"
                      :title="ch.loop ? 'Modo: Repetir em Loop (clique para tocar 1 vez)' : 'Modo: Tocar 1 vez (clique para repetir em loop)'"
                    >
                      {{ ch.loop ? 'Loop' : '1x' }}
                    </button>
                    <span
                      v-else
                      class="loop-pill"
                      :class="ch.loop ? 'loop-on' : 'loop-once'"
                    >
                      {{ ch.loop ? 'Loop' : '1x' }}
                    </span>
                  </div>
                  <span class="channel-desc truncate">
                    {{ ch.type === 'preset' ? getPresetDescription(ch.presetKey) : 'Faixa da biblioteca' }}
                  </span>
                </div>
              </div>

              <!-- VU Meter Visual Animado -->
              <div class="vu-meter">
                <span class="vu-bar vu-1" :style="{ height: getVuHeight(ch, 1) }"></span>
                <span class="vu-bar vu-2" :style="{ height: getVuHeight(ch, 2) }"></span>
                <span class="vu-bar vu-3" :style="{ height: getVuHeight(ch, 3) }"></span>
                <span class="vu-bar vu-4" :style="{ height: getVuHeight(ch, 4) }"></span>
              </div>
            </div>

            <!-- Fader / Slider de Volume -->
            <div class="channel-fader-area">
              <div class="fader-label-row">
                <div class="fader-mute-group">
                  <button
                    v-if="isDJ"
                    @click="toggleChannelMute(ch)"
                    class="mute-icon-btn"
                    :title="ch.muted ? 'Desmutar' : 'Silenciar'"
                  >
                    <VolumeX v-if="ch.muted || getEffectiveVolume(ch) === 0" class="icon-sm text-red" />
                    <Volume2 v-else class="icon-sm" />
                  </button>
                  <Volume2 v-else class="icon-sm" />
                  <span>Volume:</span>
                </div>
                <div class="fader-value">
                  {{ Math.round(getEffectiveVolume(ch) * 100) }}%
                  <span v-if="!isDJ && personalMixEnabled" class="local-indicator">(local)</span>
                </div>
              </div>

              <!-- Slider do DJ (ou do Ouvinte com Ajuste Local) -->
              <div class="fader-controls-row">
                <input
                  type="range"
                  min="0"
                  max="100"
                  :value="Math.round(getEffectiveVolume(ch) * 100)"
                  @input="handleVolumeChange(ch, $event.target.value)"
                  :disabled="!isDJ && !personalMixEnabled"
                  class="range-slider channel-slider"
                />

                <!-- Controles do DJ: Loop/1x, Play/Pause e Deletar -->
                <div v-if="isDJ" class="channel-actions">
                  <button
                    @click="toggleChannelLoop(ch)"
                    class="channel-action-btn"
                    :class="ch.loop ? 'text-emerald' : 'text-amber'"
                    :title="ch.loop ? 'Modo: Repetir infinitamente (clique para tocar 1 vez)' : 'Modo: Tocar 1 vez (clique para repetir infinitamente)'"
                  >
                    <Repeat v-if="ch.loop" class="icon-xs" />
                    <Repeat1 v-else class="icon-xs" />
                  </button>
                  <button
                    @click="toggleChannelPlay(ch)"
                    class="channel-action-btn"
                    :title="ch.isPlaying ? 'Pausar som' : 'Tocar som'"
                  >
                    <Pause v-if="ch.isPlaying" class="icon-xs text-emerald" />
                    <Play v-else class="icon-xs" />
                  </button>
                  <button
                    @click="removeChannel(ch.id)"
                    class="channel-action-btn delete"
                    title="Remover canal"
                  >
                    <Trash2 class="icon-xs" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Modal para Adicionar Som / Camada ao Mixer (Apenas DJ) -->
    <Teleport to="body">
      <div v-if="showAddModal" class="modal-overlay">
        <div class="mixer-modal">
          <!-- Cabeçalho do Modal -->
          <div class="mixer-modal-header">
            <div class="modal-title">
              <Plus class="icon-md text-emerald" />
              <h3>Adicionar Som ao Mixer</h3>
            </div>
            <button @click="showAddModal = false" class="btn-icon">
              <X class="icon-md" />
            </button>
          </div>

          <!-- Abas do Modal: Presets de Ambiente vs Faixas da Biblioteca -->
          <div class="mixer-modal-tabs">
            <button
              @click="modalTab = 'presets'"
              :class="{ active: modalTab === 'presets' }"
              class="tab-btn"
            >
              <Sparkles class="icon-sm" />
              Sons de Ambiente (Presets)
            </button>
            <button
              @click="modalTab = 'library'"
              :class="{ active: modalTab === 'library' }"
              class="tab-btn"
            >
              <Music class="icon-sm" />
              Biblioteca da Rádio
            </button>
          </div>

          <!-- Corpo do Modal -->
          <div class="mixer-modal-body">
            <!-- Aba 1: Presets Prontos -->
            <div v-if="modalTab === 'presets'" class="presets-tab-content">
              <!-- Filtro de Categorias de Sons -->
              <div class="category-pills">
                <button
                  v-for="cat in categories"
                  :key="cat"
                  @click="selectedCategory = cat"
                  :class="{ selected: selectedCategory === cat }"
                  class="category-pill"
                >
                  {{ cat }}
                </button>
              </div>

              <div class="presets-grid">
                <button
                  v-for="preset in filteredPresets"
                  :key="preset.key"
                  @click="addPresetChannel(preset)"
                  :disabled="isPresetActive(preset.key)"
                  :class="{ 'is-active': isPresetActive(preset.key) }"
                  class="preset-card"
                >
                  <div>
                    <div class="preset-card-top">
                      <div class="preset-card-icon">
                        <component :is="getPresetIcon(preset.key)" class="icon-md" />
                      </div>
                      <span v-if="isPresetActive(preset.key)" class="preset-active-label">
                        Já ativo
                      </span>
                      <Plus v-else class="icon-md preset-plus-icon" />
                    </div>
                    <div class="preset-card-name">{{ preset.name }}</div>
                    <div class="preset-card-desc">{{ preset.description }}</div>
                  </div>
                </button>
              </div>
            </div>

            <!-- Aba 2: Faixas da Biblioteca da Rádio -->
            <div v-else class="library-tab-content">
              <div class="search-wrapper">
                <Search class="icon-sm search-icon" />
                <input
                  v-model="librarySearch"
                  placeholder="Buscar faixa pelo título..."
                  class="search-input"
                />
              </div>

              <div v-if="filteredLibraryTracks.length === 0" class="empty-state">
                Nenhuma faixa encontrada no catálogo da rádio.
              </div>

              <div v-else class="modal-tracks-list">
                <div
                  v-for="track in filteredLibraryTracks"
                  :key="track.id"
                  class="modal-track-item"
                >
                  <div class="modal-track-info">
                    <Music class="icon-sm icon-purple" />
                    <span class="modal-track-title truncate">{{ track.title }}</span>
                  </div>
                  <button
                    @click="addTrackChannel(track)"
                    class="btn btn-purple btn-sm"
                  >
                    Adicionar
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div class="mixer-modal-footer">
            <button @click="showAddModal = false" class="btn btn-secondary">
              Fechar
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import {
  Sliders,
  SlidersHorizontal,
  Plus,
  Volume2,
  VolumeX,
  Play,
  Pause,
  Trash2,
  Sparkles,
  Music,
  X,
  Search,
  CloudRain,
  CloudLightning,
  Coffee,
  Flame,
  Waves,
  Wind,
  Trees,
  Disc,
  Droplets,
  Moon,
  Megaphone,
  Bell,
  Radio,
  Repeat,
  Repeat1,
} from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { AMBIENT_PRESETS } from '../services/ambientPresets'
import { ambientAudio } from '../services/ambientAudio'

const props = defineProps({
  isDJ: {
    type: Boolean,
    default: false,
  },
  availableTracks: {
    type: Array,
    default: () => [],
  },
})

const emit = defineEmits(['updateMixer'])

const store = useStationStore()
const isDJ = computed(() => Boolean(props.isDJ || store.role === 'dj'))
const isExpanded = ref(true)
const showAddModal = ref(false)
const modalTab = ref('presets')
const selectedCategory = ref('Todas')
const categories = ['Todas', 'Chuva', 'Natureza', 'Ambiente', 'Efeitos DJ']
const librarySearch = ref('')
const personalMixEnabled = ref(false)
const localVolumeMap = ref(new Map())
const vuTick = ref(0)
let vuInterval = null

const quickPresets = computed(() => AMBIENT_PRESETS.slice(0, 4))

const filteredPresets = computed(() => {
  if (selectedCategory.value === 'Todas') return AMBIENT_PRESETS
  return AMBIENT_PRESETS.filter(p => p.category === selectedCategory.value)
})

const activeChannelsCount = computed(() => {
  if (!store.mixer) return 0
  return store.mixer.filter(ch => ch.isPlaying && !ch.muted).length
})

const areAllMuted = computed(() => {
  if (!store.mixer || store.mixer.length === 0) return false
  return store.mixer.every(ch => ch.muted)
})

const filteredLibraryTracks = computed(() => {
  const query = librarySearch.value.toLowerCase().trim()
  const list = props.availableTracks || []
  if (!query) return list
  return list.filter(t => t.title && t.title.toLowerCase().includes(query))
})

const iconMap = {
  rain: CloudRain,
  thunder: CloudLightning,
  thunder_fade: CloudLightning,
  thunderstorm_long: CloudRain,
  creek: Droplets,
  waves: Waves,
  wind: Wind,
  forest: Trees,
  crickets: Moon,
  fireplace: Flame,
  cafe: Coffee,
  vinyl: Disc,
  horn: Megaphone,
  alarm: Bell,
  fire_breath: Flame,
  alert: Radio,
}

function getPresetIcon(presetKey) {
  return iconMap[presetKey] || Music
}

function getChannelIcon(ch) {
  if (ch.type === 'track') return Music
  return iconMap[ch.presetKey] || Music
}

function getPresetDescription(presetKey) {
  const p = AMBIENT_PRESETS.find(item => item.key === presetKey)
  return p ? p.description : 'Efeito sonoro'
}

function isPresetActive(presetKey) {
  if (!store.mixer) return false
  return store.mixer.some(ch => ch.presetKey === presetKey)
}

function getEffectiveVolume(ch) {
  if (!props.isDJ && personalMixEnabled.value && localVolumeMap.value.has(ch.id)) {
    return localVolumeMap.value.get(ch.id)
  }
  return typeof ch.volume === 'number' ? ch.volume : 0.4
}

function getVuHeight(ch, barIndex) {
  if (!ch.isPlaying || ch.muted) return '2px'
  const vol = getEffectiveVolume(ch)
  const tick = vuTick.value + barIndex * 1.5
  const wave = (Math.sin(tick) + 1) / 2
  const minH = 3
  const maxH = 14 * vol
  const h = Math.max(2, Math.round(minH + wave * maxH))
  return `${h}px`
}

function syncAudio() {
  ambientAudio.setPersonalMixEnabled(personalMixEnabled.value)
  ambientAudio.sync(store.mixer || [], true, 1.0, false)
}

watch(() => store.mixer, () => {
  syncAudio()
}, { deep: true })

onMounted(() => {
  syncAudio()
  vuInterval = setInterval(() => {
    vuTick.value += 0.35
  }, 120)
  ambientAudio.onChannelEnded = (channelId) => {
    if (props.isDJ) {
      const current = (store.mixer || []).map(item => {
        if (item.id === channelId) {
          return { ...item, isPlaying: false }
        }
        return item
      })
      emitMixerUpdate(current)
    }
  }
})

onUnmounted(() => {
  if (vuInterval) clearInterval(vuInterval)
})

function addPresetChannel(preset) {
  if (!isDJ.value) return
  const current = store.mixer ? [...store.mixer] : []
  if (current.some(ch => ch.presetKey === preset.key)) {
    showAddModal.value = false
    return
  }

  const newChannel = {
    id: `preset_${preset.key}_${Date.now()}`,
    name: preset.name,
    type: 'preset',
    presetKey: preset.key,
    trackUrl: preset.audioUrl || `/sounds/${preset.key}.mp3`,
    volume: preset.defaultVolume || 0.4,
    isPlaying: true,
    muted: false,
    loop: preset.loop === true,
  }

  current.push(newChannel)
  emitMixerUpdate(current)
  showAddModal.value = false
}

function addTrackChannel(track) {
  if (!isDJ.value) return
  const current = store.mixer ? [...store.mixer] : []
  const newChannel = {
    id: `track_${track.id}_${Date.now()}`,
    name: track.title,
    type: 'track',
    presetKey: '',
    trackUrl: track.url,
    volume: 0.35,
    isPlaying: true,
    muted: false,
    loop: false,
  }

  current.push(newChannel)
  emitMixerUpdate(current)
  showAddModal.value = false
}

function toggleChannelLoop(ch) {
  if (!isDJ.value) return
  const current = (store.mixer || []).map(item => {
    if (item.id === ch.id) {
      return { ...item, loop: !item.loop }
    }
    return item
  })
  emitMixerUpdate(current)
}

function removeChannel(channelId) {
  if (!isDJ.value) return
  const current = (store.mixer || []).filter(ch => ch.id !== channelId)
  emitMixerUpdate(current)
}

function toggleChannelPlay(ch) {
  if (!isDJ.value) return
  const current = (store.mixer || []).map(item => {
    if (item.id === ch.id) {
      return { ...item, isPlaying: !item.isPlaying }
    }
    return item
  })
  emitMixerUpdate(current)
}

function toggleChannelMute(ch) {
  if (!isDJ.value) return
  const current = (store.mixer || []).map(item => {
    if (item.id === ch.id) {
      return { ...item, muted: !item.muted }
    }
    return item
  })
  emitMixerUpdate(current)
}

function muteAllChannels() {
  if (!isDJ.value) return
  const targetMuted = !areAllMuted.value
  const current = (store.mixer || []).map(item => ({
    ...item,
    muted: targetMuted,
  }))
  emitMixerUpdate(current)
}

function clearMixer() {
  if (!isDJ.value) return
  emitMixerUpdate([])
}

function handleVolumeChange(ch, newVolumeInt) {
  const vol = Math.max(0, Math.min(100, parseInt(newVolumeInt, 10))) / 100
  if (isDJ.value) {
    const current = (store.mixer || []).map(item => {
      if (item.id === ch.id) {
        return { ...item, volume: vol }
      }
      return item
    })
    emitMixerUpdate(current)
  } else if (personalMixEnabled.value) {
    localVolumeMap.value.set(ch.id, vol)
    ambientAudio.setLocalMultiplier(ch.id, vol / (ch.volume || 0.001))
    syncAudio()
  }
}

function togglePersonalMix() {
  personalMixEnabled.value = !personalMixEnabled.value
  ambientAudio.setPersonalMixEnabled(personalMixEnabled.value)
  if (personalMixEnabled.value) {
    ;(store.mixer || []).forEach(ch => {
      if (!localVolumeMap.value.has(ch.id)) {
        localVolumeMap.value.set(ch.id, ch.volume || 0.4)
      }
    })
  }
  syncAudio()
}

function emitMixerUpdate(channels) {
  store.setMixer(channels)
  syncAudio()
  emit('updateMixer', channels)
}
</script>

<style scoped>
.mixer-panel {

  overflow: hidden;
  transition: all 0.3s ease;
}

.mixer-header {
  padding: 0.75rem 1rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.mixer-header-left {
  display: flex;
  align-items: center;
  gap: 0.625rem;
}

.mixer-icon-box {
  width: 2rem;
  height: 2rem;
  border-radius: var(--radius-lg);
  background-color: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.3);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--emerald-400);
}

.mixer-title-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.mixer-title {
  font-size: 0.875rem;
  font-weight: 700;
  color: var(--zinc-100);
}

.mixer-badge {
  font-size: 0.625rem;
  padding: 0.125rem 0.5rem;
  border-radius: var(--radius-full);
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
}

.mixer-badge.active {
  font-weight: 600;
  background-color: rgba(16, 185, 129, 0.2);
  color: var(--emerald-300);
  border: 1px solid rgba(16, 185, 129, 0.3);
}

.mixer-badge.inactive {
  background-color: rgba(63, 63, 70, 0.6);
  color: var(--zinc-400);
}

.mixer-subtitle {
  font-size: 0.6875rem;
  color: var(--zinc-400);
}

.mixer-header-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.personal-mix-box {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  background-color: rgba(24, 24, 27, 0.7);
  padding: 0.25rem 0.625rem;
  border-radius: var(--radius-lg);
  border: 1px solid rgba(63, 63, 70, 0.6);
}

.personal-mix-label {
  font-size: 0.6875rem;
  color: var(--zinc-400);
}

.switch-btn {
  width: 2rem;
  height: 1rem;
  border-radius: var(--radius-full);
  background-color: var(--zinc-700);
  position: relative;
  padding: 0.125rem;
  transition: background-color 0.2s ease;
}

.switch-btn.enabled {
  background-color: var(--emerald-500);
}

.switch-thumb {
  width: 0.75rem;
  height: 0.75rem;
  border-radius: var(--radius-full);
  background-color: #ffffff;
  transition: transform 0.2s ease;
  transform: translateX(0);
}

.switch-btn.enabled .switch-thumb {
  transform: translateX(1rem);
}

.mixer-empty {
  padding: 1rem 1rem;
  text-align: center;
}

.mixer-empty-icon {
  width: 3rem;
  height: 3rem;
  margin: 0 auto 0.75rem;
  border-radius: var(--radius-full);
  background-color: var(--zinc-800);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--zinc-500);
}

.mixer-empty-title {
  font-size: 0.875rem;
  color: var(--zinc-300);
  font-weight: 500;
}

.mixer-empty-desc {
  font-size: 0.75rem;
  color: var(--zinc-500);
  margin: 0.25rem auto 0;
  max-width: 24rem;
}

.quick-presets-row {
  margin-top: 1rem;
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.5rem;
}

.quick-preset-btn {
  padding: 0.375rem 0.75rem;
  border-radius: var(--radius-lg);
  background-color: var(--zinc-800);
  border: 1px solid var(--zinc-700);
  font-size: 0.75rem;
  color: var(--zinc-200);
  display: flex;
  align-items: center;
  gap: 0.375rem;
  transition: background-color 0.15s ease, transform 0.15s ease;
}

.quick-preset-btn:hover {
  background-color: var(--zinc-700);
  transform: scale(1.04);
}

.mixer-channels-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.mixer-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 0.5rem;
  border-bottom: 1px solid var(--zinc-800);
  font-size: 0.75rem;
}

.mixer-toolbar-label {
  color: var(--zinc-400);
  font-weight: 500;
}

.mixer-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.toolbar-link {
  color: var(--zinc-400);
  transition: color 0.15s ease;
}

.toolbar-link.amber-hover:hover {
  color: var(--amber-400);
}

.toolbar-link.red-hover:hover {
  color: var(--red-400);
}

.toolbar-sep {
  color: var(--zinc-600);
}

.channels-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 0.75rem;
}

.channel-card {
  background-color: rgba(24, 24, 27, 0.85);
  border: 1px solid var(--zinc-800);
  border-radius: var(--radius-xl);
  padding: 0.875rem;
  transition: all 0.2s ease;
}

.channel-card.active {
  border-color: var(--zinc-700);
  box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.25);
}

.channel-card.muted {
  opacity: 0.7;
}

.channel-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.625rem;
  gap: 0.5rem;
}

.channel-info-row {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  min-width: 0;
}

.channel-icon {
  width: 1.75rem;
  height: 1.75rem;
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  border: 1px solid var(--zinc-700);
}

.channel-icon.active {
  background-color: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.3);
  color: var(--emerald-400);
}

.channel-icon.inactive {
  background-color: var(--zinc-800);
  color: var(--zinc-500);
}

.channel-meta {
  min-width: 0;
}

.channel-name-row {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  flex-wrap: wrap;
}

.channel-name {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--zinc-100);
}

.channel-tag {
  font-size: 0.5625rem;
  padding: 0.0625rem 0.375rem;
  border-radius: var(--radius-sm);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  font-family: var(--font-mono);
}

.channel-tag.preset {
  background-color: var(--zinc-800);
  color: var(--zinc-400);
  border: 1px solid rgba(63, 63, 70, 0.6);
}

.channel-tag.track {
  background-color: rgba(88, 28, 135, 0.4);
  color: var(--purple-300);
  border: 1px solid rgba(126, 34, 206, 0.4);
}

.loop-pill {
  font-size: 0.5625rem;
  padding: 0.125rem 0.375rem;
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.loop-pill.clickable {
  cursor: pointer;
  user-select: none;
  transition: transform 0.15s ease, background-color 0.15s ease;
}

.loop-pill.clickable:hover {
  transform: scale(1.05);
}

.loop-pill.loop-on {
  background-color: var(--zinc-800);
  color: var(--emerald-400);
  border: 1px solid rgba(16, 185, 129, 0.3);
}

.loop-pill.loop-once {
  background-color: rgba(69, 26, 3, 0.4);
  color: var(--amber-300);
  border: 1px solid rgba(180, 83, 9, 0.4);
}

.channel-desc {
  font-size: 0.625rem;
  color: var(--zinc-500);
  display: block;
}

/* VU Meter */
.vu-meter {
  display: flex;
  align-items: flex-end;
  gap: 0.125rem;
  height: 1rem;
  padding: 0.125rem 0.5rem;
  background-color: rgba(9, 9, 11, 0.6);
  border-radius: var(--radius-sm);
  border: 1px solid rgba(39, 39, 42, 0.8);
  flex-shrink: 0;
}

.vu-bar {
  width: 0.25rem;
  border-radius: 1px;
  transition: height 0.15s ease;
}

.vu-1 { background-color: var(--emerald-500); }
.vu-2 { background-color: var(--emerald-400); }
.vu-3 { background-color: var(--amber-400); }
.vu-4 { background-color: var(--red-400); }

/* Fader */
.channel-fader-area {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.fader-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 0.6875rem;
}

.fader-mute-group {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  color: var(--zinc-400);
}

.mute-icon-btn {
  padding: 0.125rem;
  color: var(--zinc-400);
  display: flex;
  align-items: center;
}

.mute-icon-btn:hover {
  color: var(--zinc-200);
}

.fader-value {
  font-family: var(--font-mono);
  color: var(--zinc-200);
  font-weight: 500;
}

.local-indicator {
  font-size: 0.625rem;
  color: var(--emerald-400);
  margin-left: 0.25rem;
}

.fader-controls-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.channel-slider {
  flex: 1;
  background-color: var(--zinc-800);
}

.channel-actions {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.channel-action-btn {
  padding: 0.375rem;
  border-radius: var(--radius-lg);
  background-color: var(--zinc-800);
  color: var(--zinc-400);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background-color 0.15s ease, color 0.15s ease;
}

.channel-action-btn:hover {
  background-color: var(--zinc-700);
  color: #ffffff;
}

.channel-action-btn.delete:hover {
  background-color: rgba(69, 10, 10, 0.5);
  color: var(--red-400);
}

/* Mixer Modal */
.mixer-modal {
  background-color: var(--zinc-900);
  border: 1px solid var(--zinc-700);
  border-radius: var(--radius-2xl);
  max-width: 32rem;
  width: 100%;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.7);
  overflow: hidden;
}

.mixer-modal-header {
  padding: 1rem 1.25rem;
  border-bottom: 1px solid var(--zinc-800);
  background-color: var(--zinc-850);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.mixer-modal-tabs {
  padding: 0.75rem 1.25rem 0.5rem;
  border-bottom: 1px solid var(--zinc-800);
  display: flex;
  gap: 0.5rem;
}

.mixer-modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 1.25rem;
}

.presets-tab-content,
.library-tab-content {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.category-pills {
  display: flex;
  gap: 0.375rem;
  overflow-x: auto;
  padding-bottom: 0.25rem;
  border-bottom: 1px solid var(--zinc-800);
}

.category-pill {
  padding: 0.25rem 0.625rem;
  border-radius: var(--radius-lg);
  font-size: 0.75rem;
  color: var(--zinc-400);
  flex-shrink: 0;
  border: 1px solid transparent;
  transition: all 0.15s ease;
}

.category-pill:hover {
  color: var(--zinc-200);
  background-color: var(--zinc-800);
}

.category-pill.selected {
  background-color: var(--zinc-750);
  color: var(--emerald-400);
  font-weight: 600;
  border-color: rgba(16, 185, 129, 0.3);
}

.presets-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.625rem;
}

.preset-card {
  padding: 0.75rem;
  border-radius: var(--radius-xl);
  border: 1px solid var(--zinc-800);
  background-color: rgba(31, 31, 35, 0.8);
  text-align: left;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
}

.preset-card:hover:not(:disabled) {
  border-color: rgba(16, 185, 129, 0.6);
  background-color: rgba(39, 39, 42, 0.8);
}

.preset-card.is-active {
  opacity: 0.4;
  background-color: var(--zinc-900);
}

.preset-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.375rem;
}

.preset-card-icon {
  width: 1.75rem;
  height: 1.75rem;
  border-radius: var(--radius-lg);
  background-color: var(--zinc-800);
  color: var(--emerald-400);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background-color 0.15s ease;
}

.preset-card:hover:not(:disabled) .preset-card-icon {
  background-color: rgba(16, 185, 129, 0.2);
}

.preset-active-label {
  font-size: 0.625rem;
  color: var(--emerald-400);
  font-weight: 500;
}

.preset-plus-icon {
  color: var(--zinc-500);
  transition: color 0.15s ease;
}

.preset-card:hover:not(:disabled) .preset-plus-icon {
  color: var(--emerald-400);
}

.preset-card-name {
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--zinc-200);
  transition: color 0.15s ease;
}

.preset-card:hover:not(:disabled) .preset-card-name {
  color: var(--emerald-300);
}

.preset-card-desc {
  font-size: 0.625rem;
  color: var(--zinc-400);
  margin-top: 0.125rem;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.modal-tracks-list {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
  max-height: 15rem;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.modal-track-item {
  padding: 0.625rem;
  border-radius: var(--radius-lg);
  background-color: var(--zinc-850);
  border: 1px solid var(--zinc-800);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  transition: border-color 0.15s ease;
}

.modal-track-item:hover {
  border-color: var(--zinc-700);
}

.modal-track-info {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.icon-purple {
  color: var(--purple-400);
}

.modal-track-title {
  font-size: 0.75rem;
  color: var(--zinc-200);
  font-weight: 500;
}

.mixer-modal-footer {
  padding: 0.75rem 1.25rem;
  border-top: 1px solid var(--zinc-800);
  background-color: var(--zinc-850);
  display: flex;
  justify-content: flex-end;
}
</style>
