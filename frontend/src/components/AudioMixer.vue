<template>
  <div class="bg-zinc-850/80 backdrop-blur-md rounded-xl border border-zinc-700/60 shadow-xl overflow-hidden transition-all duration-300">
    <!-- Barra Superior do Mixer -->
    <div class="px-4 py-3 bg-gradient-to-r from-zinc-800/90 via-zinc-800/70 to-zinc-850 flex items-center justify-between">
      <div class="flex items-center gap-2.5">
        <div class="w-8 h-8 rounded-lg bg-emerald-500/10 border border-emerald-500/30 flex items-center justify-center text-emerald-400">
          <Sliders class="w-4 h-4" />
        </div>
        <div>
          <div class="flex items-center gap-2">
            <h3 class="text-sm font-bold text-zinc-100 flex items-center gap-1.5">
              Mixer de Efeitos & Ambiente
            </h3>
            <span v-if="activeChannelsCount > 0" class="text-[10px] font-semibold px-2 py-0.5 rounded-full bg-emerald-500/20 text-emerald-300 border border-emerald-500/30 flex items-center gap-1">
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
              {{ activeChannelsCount }} ativo{{ activeChannelsCount > 1 ? 's' : '' }}
            </span>
            <span v-else class="text-[10px] px-2 py-0.5 rounded-full bg-zinc-700/60 text-zinc-400">
              Inativo
            </span>
          </div>
          <p class="text-[11px] text-zinc-400">
            {{ isDJ ? 'Mixe e transmita sons de fundo para toda a estação' : 'Sons de fundo mixados pela rádio' }}
          </p>
        </div>
      </div>

      <div class="flex items-center gap-2">
        <!-- Modo Ouvinte: Toggle de Ajustes Pessoais -->
        <div v-if="!isDJ && store.mixer && store.mixer.length > 0" class="flex items-center gap-1.5 bg-zinc-900/70 px-2.5 py-1 rounded-lg border border-zinc-700/60 text-xs">
          <span class="text-[11px] text-zinc-400">Ajuste Local</span>
          <button
            @click="togglePersonalMix"
            :class="personalMixEnabled ? 'bg-emerald-500 text-white' : 'bg-zinc-700 text-zinc-400'"
            class="w-8 h-4 rounded-full transition-colors relative p-0.5"
            title="Personalizar volumes no seu navegador sem mudar a rádio"
          >
            <div
              :class="personalMixEnabled ? 'translate-x-4' : 'translate-x-0'"
              class="w-3 h-3 rounded-full bg-white transition-transform"
            ></div>
          </button>
        </div>

        <!-- Botão Adicionar Som (DJ) -->
        <button
          v-if="isDJ"
          @click="showAddModal = true"
          class="px-2.5 py-1.5 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-medium flex items-center gap-1.5 shadow-sm transition-colors"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>Adicionar Som</span>
        </button>

        <!-- Botão Minimizar/Expandir -->
        <button
          @click="isExpanded = !isExpanded"
          class="p-1.5 rounded-lg hover:bg-zinc-700/60 text-zinc-400 hover:text-zinc-200 transition-colors text-xs"
          :title="isExpanded ? 'Minimizar Mixer' : 'Expandir Mixer'"
        >
          <span v-if="isExpanded">▲</span>
          <span v-else>▼</span>
        </button>
      </div>
    </div>

    <!-- Conteúdo Expandido do Mixer -->
    <div v-show="isExpanded" class="p-4 space-y-4">
      <!-- Mensagem quando não há canais ativos -->
      <div v-if="!store.mixer || store.mixer.length === 0" class="py-8 px-4 text-center border border-dashed border-zinc-700/70 rounded-xl bg-zinc-900/40">
        <div class="w-12 h-12 mx-auto mb-3 rounded-full bg-zinc-800 flex items-center justify-center text-zinc-500">
          <SlidersHorizontal class="w-6 h-6" />
        </div>
        <p class="text-sm text-zinc-300 font-medium">Nenhum som de fundo no mixer</p>
        <p class="text-xs text-zinc-500 mt-1 max-w-sm mx-auto">
          {{ isDJ 
            ? 'Adicione sons de ambiente como chuva, cafeteria com conversas, lareira crepitante ou músicas da biblioteca para tocar simultaneamente na estação.' 
            : 'O DJ da rádio ainda não ativou sons de fundo ou ambientação nesta estação.' 
          }}
        </p>
        <div v-if="isDJ" class="mt-4 flex flex-wrap justify-center gap-2">
          <button
            v-for="preset in quickPresets"
            :key="preset.key"
            @click="addPresetChannel(preset)"
            class="px-3 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 text-xs text-zinc-200 flex items-center gap-1.5 transition-all hover:scale-105"
          >
            <component :is="getPresetIcon(preset.key)" class="w-3.5 h-3.5 text-emerald-400" />
            <span>+ {{ preset.name }}</span>
          </button>
        </div>
      </div>

      <!-- Grade de Canais de Som -->
      <div v-else class="space-y-3">
        <!-- Barra de Ações Rápidas (DJ) -->
        <div v-if="isDJ" class="flex items-center justify-between pb-2 border-b border-zinc-800 text-xs">
          <span class="text-zinc-400 font-medium flex items-center gap-1.5">
            Canais Sincronizados na Rádio:
          </span>
          <div class="flex items-center gap-2">
            <button
              @click="muteAllChannels"
              class="text-zinc-400 hover:text-amber-400 transition-colors"
              title="Alternar mudo em todos os canais de fundo"
            >
              {{ areAllMuted ? 'Desmutar Todos' : 'Mutar Todos' }}
            </button>
            <span class="text-zinc-600">|</span>
            <button
              @click="clearMixer"
              class="text-zinc-400 hover:text-red-400 transition-colors"
              title="Remover todos os canais de efeitos"
            >
              Limpar Mixer
            </button>
          </div>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <!-- Card de Canal Individual -->
          <div
            v-for="ch in store.mixer"
            :key="ch.id"
            class="bg-zinc-900/80 border rounded-xl p-3.5 transition-all duration-200"
            :class="ch.isPlaying && !ch.muted ? 'border-zinc-700 shadow-lg' : 'border-zinc-800/80 opacity-70'"
          >
            <!-- Topo do Canal: Ícone, Nome, VU animado e Ações -->
            <div class="flex items-center justify-between mb-2.5">
              <div class="flex items-center gap-2.5 min-w-0">
                <div
                  class="w-7 h-7 rounded-lg flex items-center justify-center shrink-0 border"
                  :class="ch.isPlaying && !ch.muted ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-400' : 'bg-zinc-800 border-zinc-700 text-zinc-500'"
                >
                  <component :is="getChannelIcon(ch)" class="w-3.5 h-3.5" />
                </div>
                <div class="min-w-0">
                  <div class="flex items-center gap-1.5">
                    <span class="text-xs font-semibold text-zinc-100 truncate block">
                      {{ ch.name }}
                    </span>
                    <span v-if="ch.type === 'preset'" class="text-[9px] px-1.5 py-0.2 rounded bg-zinc-800 text-zinc-400 border border-zinc-700/60 uppercase tracking-wider font-mono">
                      Ambiente
                    </span>
                    <span v-else class="text-[9px] px-1.5 py-0.2 rounded bg-purple-900/40 text-purple-300 border border-purple-700/40 uppercase tracking-wider font-mono">
                      Faixa
                    </span>
                    <button
                      v-if="isDJ"
                      @click="toggleChannelLoop(ch)"
                      type="button"
                      class="text-[9px] px-1.5 py-0.5 rounded font-mono uppercase tracking-wider transition-all hover:scale-105 active:scale-95 cursor-pointer select-none"
                      :class="ch.loop ? 'bg-zinc-800 text-emerald-400 border border-emerald-500/30 hover:bg-zinc-700' : 'bg-amber-950/40 text-amber-300 border border-amber-700/40 hover:bg-amber-900/50'"
                      :title="ch.loop ? 'Modo: Repetir em Loop (clique para tocar 1 vez)' : 'Modo: Tocar 1 vez (clique para repetir em loop)'"
                    >
                      {{ ch.loop ? 'Loop' : '1x' }}
                    </button>
                    <span
                      v-else
                      class="text-[9px] px-1.5 py-0.5 rounded font-mono uppercase tracking-wider"
                      :class="ch.loop ? 'bg-zinc-800 text-emerald-400 border border-emerald-500/20' : 'bg-amber-950/40 text-amber-300 border border-amber-700/40'"
                    >
                      {{ ch.loop ? 'Loop' : '1x' }}
                    </span>
                  </div>
                  <span class="text-[10px] text-zinc-500 block truncate">
                    {{ ch.type === 'preset' ? getPresetDescription(ch.presetKey) : 'Faixa da biblioteca' }}
                  </span>
                </div>
              </div>

              <!-- VU Meter Visual Animado -->
              <div class="flex items-end gap-0.5 h-4 px-2 py-0.5 bg-zinc-950/60 rounded border border-zinc-800/80 shrink-0">
                <span
                  class="w-1 rounded-sm bg-emerald-500 transition-all duration-150"
                  :style="{ height: getVuHeight(ch, 1) }"
                ></span>
                <span
                  class="w-1 rounded-sm bg-emerald-400 transition-all duration-150"
                  :style="{ height: getVuHeight(ch, 2) }"
                ></span>
                <span
                  class="w-1 rounded-sm bg-amber-400 transition-all duration-150"
                  :style="{ height: getVuHeight(ch, 3) }"
                ></span>
                <span
                  class="w-1 rounded-sm bg-red-400 transition-all duration-150"
                  :style="{ height: getVuHeight(ch, 4) }"
                ></span>
              </div>
            </div>

            <!-- Fader / Slider de Volume -->
            <div class="space-y-1.5">
              <div class="flex items-center justify-between text-[11px]">
                <div class="flex items-center gap-1 text-zinc-400">
                  <button
                    v-if="isDJ"
                    @click="toggleChannelMute(ch)"
                    class="hover:text-zinc-200 transition-colors p-0.5"
                    :title="ch.muted ? 'Desmutar' : 'Silenciar'"
                  >
                    <VolumeX v-if="ch.muted || getEffectiveVolume(ch) === 0" class="w-3.5 h-3.5 text-red-400" />
                    <Volume2 v-else class="w-3.5 h-3.5 text-zinc-400" />
                  </button>
                  <Volume2 v-else class="w-3.5 h-3.5 text-zinc-400" />
                  <span>Volume:</span>
                </div>
                <div class="font-mono text-zinc-200 font-medium">
                  {{ Math.round(getEffectiveVolume(ch) * 100) }}%
                  <span v-if="!isDJ && personalMixEnabled" class="text-[10px] text-emerald-400 ml-1">
                    (local)
                  </span>
                </div>
              </div>

              <!-- Slider do DJ (ou do Ouvinte com Ajuste Local) -->
              <div class="flex items-center gap-2">
                <input
                  type="range"
                  min="0"
                  max="100"
                  :value="Math.round(getEffectiveVolume(ch) * 100)"
                  @input="handleVolumeChange(ch, $event.target.value)"
                  :disabled="!isDJ && !personalMixEnabled"
                  class="flex-1 h-1.5 appearance-none bg-zinc-800 rounded-full cursor-pointer accent-emerald-500 disabled:opacity-50 disabled:cursor-not-allowed [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-emerald-400"
                />

                <!-- Controles do DJ: Loop/1x, Play/Pause e Deletar -->
                <div v-if="isDJ" class="flex items-center gap-1">
                  <button
                    @click="toggleChannelLoop(ch)"
                    class="p-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 transition-colors"
                    :class="ch.loop ? 'text-emerald-400' : 'text-amber-400'"
                    :title="ch.loop ? 'Modo: Repetir infinitamente (clique para tocar 1 vez)' : 'Modo: Tocar 1 vez (clique para repetir infinitamente)'"
                  >
                    <Repeat v-if="ch.loop" class="w-3 h-3" />
                    <Repeat1 v-else class="w-3 h-3" />
                  </button>
                  <button
                    @click="toggleChannelPlay(ch)"
                    class="p-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-zinc-300 hover:text-white transition-colors"
                    :title="ch.isPlaying ? 'Pausar som' : 'Tocar som'"
                  >
                    <Pause v-if="ch.isPlaying" class="w-3 h-3 text-emerald-400" />
                    <Play v-else class="w-3 h-3 text-zinc-400" />
                  </button>
                  <button
                    @click="removeChannel(ch.id)"
                    class="p-1.5 rounded-lg bg-zinc-800 hover:bg-red-950/40 text-zinc-500 hover:text-red-400 transition-colors"
                    title="Remover canal"
                  >
                    <Trash2 class="w-3 h-3" />
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
      <div v-if="showAddModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm animate-in fade-in duration-200">
        <div class="bg-zinc-900 border border-zinc-700 rounded-2xl max-w-lg w-full max-h-[90vh] flex flex-col shadow-2xl overflow-hidden">
          <!-- Cabeçalho do Modal -->
          <div class="px-5 py-4 border-b border-zinc-800 flex items-center justify-between bg-zinc-850">
            <div class="flex items-center gap-2">
              <Plus class="w-4 h-4 text-emerald-400" />
              <h3 class="text-sm font-bold text-zinc-100">Adicionar Som ao Mixer</h3>
            </div>
            <button @click="showAddModal = false" class="text-zinc-400 hover:text-zinc-200 p-1 rounded-lg hover:bg-zinc-800">
              <X class="w-4 h-4" />
            </button>
          </div>

          <!-- Abas do Modal: Presets de Ambiente vs Faixas da Biblioteca -->
          <div class="px-5 pt-3 pb-2 border-b border-zinc-800 flex gap-2">
            <button
              @click="modalTab = 'presets'"
              :class="modalTab === 'presets' ? 'bg-emerald-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'"
              class="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors flex items-center gap-1.5"
            >
              <Sparkles class="w-3.5 h-3.5" />
              Sons de Ambiente (Presets)
            </button>
            <button
              @click="modalTab = 'library'"
              :class="modalTab === 'library' ? 'bg-emerald-600 text-white' : 'text-zinc-400 hover:bg-zinc-800'"
              class="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors flex items-center gap-1.5"
            >
              <Music class="w-3.5 h-3.5" />
              Biblioteca da Rádio
            </button>
          </div>

          <!-- Corpo do Modal -->
          <div class="flex-1 overflow-y-auto p-5 space-y-4">
            <!-- Aba 1: Presets Prontos -->
            <div v-if="modalTab === 'presets'" class="space-y-3">
              <!-- Filtro de Categorias de Sons -->
              <div class="flex gap-1.5 overflow-x-auto pb-1 text-xs border-b border-zinc-800">
                <button
                  v-for="cat in categories"
                  :key="cat"
                  @click="selectedCategory = cat"
                  :class="selectedCategory === cat ? 'bg-zinc-750 text-emerald-400 font-semibold border border-emerald-500/30' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800'"
                  class="px-2.5 py-1 rounded-lg transition-colors shrink-0"
                >
                  {{ cat }}
                </button>
              </div>

              <div class="grid grid-cols-2 gap-2.5">
                <button
                  v-for="preset in filteredPresets"
                  :key="preset.key"
                  @click="addPresetChannel(preset)"
                  :disabled="isPresetActive(preset.key)"
                  :class="isPresetActive(preset.key) ? 'opacity-40 border-zinc-800 cursor-not-allowed bg-zinc-900' : 'hover:border-emerald-500/60 hover:bg-zinc-800/80 border-zinc-800 bg-zinc-850/80'"
                  class="p-3 rounded-xl border text-left transition-all group flex flex-col justify-between"
                >
                  <div>
                    <div class="flex items-center justify-between mb-1.5">
                      <div class="w-7 h-7 rounded-lg bg-zinc-800 group-hover:bg-emerald-500/20 text-emerald-400 flex items-center justify-center transition-colors">
                        <component :is="getPresetIcon(preset.key)" class="w-4 h-4" />
                      </div>
                      <span v-if="isPresetActive(preset.key)" class="text-[10px] text-emerald-400 font-medium">
                        Já ativo
                      </span>
                      <Plus v-else class="w-4 h-4 text-zinc-500 group-hover:text-emerald-400 transition-colors" />
                    </div>
                    <div class="text-xs font-bold text-zinc-200 group-hover:text-emerald-300 transition-colors">
                      {{ preset.name }}
                    </div>
                    <div class="text-[10px] text-zinc-400 mt-0.5 line-clamp-2">
                      {{ preset.description }}
                    </div>
                  </div>
                </button>
              </div>
            </div>

            <!-- Aba 2: Faixas da Biblioteca da Rádio -->
            <div v-else class="space-y-3">
              <div class="relative">
                <Search class="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
                <input
                  v-model="librarySearch"
                  placeholder="Buscar faixa pelo título..."
                  class="w-full bg-zinc-800 border border-zinc-700 rounded-lg pl-8 pr-3 py-1.5 text-xs text-zinc-200 outline-none focus:border-emerald-500"
                />
              </div>

              <div v-if="filteredLibraryTracks.length === 0" class="text-center py-8 text-xs text-zinc-500">
                Nenhuma faixa encontrada no catálogo da rádio.
              </div>

              <div v-else class="space-y-1.5 max-h-60 overflow-y-auto pr-1">
                <div
                  v-for="track in filteredLibraryTracks"
                  :key="track.id"
                  class="p-2.5 rounded-lg bg-zinc-850 border border-zinc-800 hover:border-zinc-700 flex items-center justify-between gap-3 transition-colors"
                >
                  <div class="min-w-0 flex items-center gap-2">
                    <Music class="w-3.5 h-3.5 text-purple-400 shrink-0" />
                    <span class="text-xs text-zinc-200 truncate font-medium">{{ track.title }}</span>
                  </div>
                  <button
                    @click="addTrackChannel(track)"
                    class="px-2.5 py-1 rounded bg-purple-600 hover:bg-purple-500 text-white text-[11px] font-medium transition-colors shrink-0"
                  >
                    Adicionar
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div class="px-5 py-3 border-t border-zinc-800 bg-zinc-850 flex justify-end">
            <button
              @click="showAddModal = false"
              class="px-4 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-zinc-300 text-xs font-medium transition-colors"
            >
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

// Mapeia ícones Lucide para presets
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
  // Simula animação pulsante realista
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

// --- AÇÕES DO DJ ---

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
    loop: preset.loop === true, // Padrão: 1 vez (false)
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
    loop: false, // Padrão: 1 vez (false)
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
    // Ajuste local do ouvinte
    localVolumeMap.value.set(ch.id, vol)
    ambientAudio.setLocalMultiplier(ch.id, vol / (ch.volume || 0.001))
    syncAudio()
  }
}

function togglePersonalMix() {
  personalMixEnabled.value = !personalMixEnabled.value
  ambientAudio.setPersonalMixEnabled(personalMixEnabled.value)
  if (personalMixEnabled.value) {
    // Inicializa os volumes locais com os volumes atuais do DJ
    (store.mixer || []).forEach(ch => {
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
