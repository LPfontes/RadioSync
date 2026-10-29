<template>
  <footer class="ytm-player-bar" role="toolbar" aria-label="Barra do player">
    <audio
      ref="audioRef"
      preload="auto"
      @loadedmetadata="startTimeUpdates"
      @waiting="buffering = true"
      @canplay="buffering = false"
      @timeupdate="onTimeUpdate"
      @ended="onEnded"
    >
      <source :src="store.state.currentSong" type="audio/ogg; codecs=opus" />
    </audio>

    <!-- Barra de Progresso no Topo da Barra (Estilo YouTube Music) -->
    <div
      class="ytm-progress-container"
      :class="{ 'is-interactive': isDJ && store.state.currentSong }"
      @click="seek"
      @mousemove="onProgressHover"
      @mouseleave="hoverTime = null"
      title="Controle deslizante de busca"
    >
      <span
        v-if="hoverTime !== null && duration > 0"
        class="ytm-hover-time"
        :style="{ left: hoverPercent + '%' }"
      >
        {{ formatTime(hoverTime) }}
      </span>
      <div class="ytm-progress-rail">
        <div
          class="ytm-progress-primary"
          :style="{ width: progressPercent + '%' }"
        ></div>
      </div>
      <div
        v-if="store.state.currentSong"
        class="ytm-progress-knob"
        :style="{ left: progressPercent + '%' }"
      ></div>
    </div>

    <!-- Left Controls: Botões de Reprodução + Tempo -->
    <div class="ytm-left-controls">
      <div class="ytm-left-buttons">
        <button
          v-if="isDJ"
          @click="seekRelative(-10)"
          :disabled="!store.state.currentSong"
          class="ytm-icon-btn"
          title="Voltar 10 segundos"
          aria-label="Voltar 10 segundos"
        >
          <RotateCcw class="icon-md" />
        </button>

        <button
          @click="togglePlay"
          :disabled="!store.state.currentSong || !isDJ"
          class="ytm-play-pause-btn"
          :class="{ 'listener-only': !isDJ }"
          :title="store.state.isPlaying ? 'Pausar' : 'Reproduzir'"
          :aria-label="store.state.isPlaying ? 'Pausar' : 'Reproduzir'"
        >
          <Loader2 v-if="buffering" class="icon-xl animate-spin" />
          <Pause v-else-if="store.state.isPlaying" class="icon-xl" />
          <Play v-else class="icon-xl play-offset" />
        </button>

        <button
          v-if="isDJ"
          @click="seekRelative(10)"
          :disabled="!store.state.currentSong"
          class="ytm-icon-btn"
          title="Avançar 10 segundos"
          aria-label="Avançar 10 segundos"
        >
          <RotateCw class="icon-md" />
        </button>

        <button
          v-if="isDJ"
          @click="nextTrack"
          :disabled="!store.state.currentSong && store.playlist.length === 0"
          class="ytm-icon-btn"
          title="Próxima faixa"
          aria-label="Próxima faixa"
        >
          <SkipForward class="icon-lg" />
        </button>
      </div>

      <span class="ytm-time-info" dir="ltr">
        {{ formatTime(currentTime) }} / {{ formatTime(duration) }}
      </span>
    </div>

    <!-- Middle Controls: Thumbnail + Título e Subtítulo da Faixa -->
    <div class="ytm-middle-controls">
      <div
        class="ytm-thumbnail-wrapper"
        :class="{ 'is-playing': store.state.isPlaying && store.state.currentSong }"
      >
        <img
          v-if="currentTrackObj && currentTrackObj.coverUrl"
          :src="currentTrackObj.coverUrl"
          class="ytm-thumb-img"
          alt="Capa"
        />
        <Disc
          v-else-if="store.state.currentSong"
          class="icon-lg text-emerald"
          :class="{ 'animate-spin-slow': store.state.isPlaying }"
        />
        <Music v-else class="icon-lg text-muted" />
      </div>

      <div class="ytm-content-info">
        <div
          class="ytm-title truncate"
          role="heading"
          aria-level="2"
          :title="store.state.currentSong ? currentTrackTitle : 'Nenhuma música tocando no momento'"
        >
          {{ store.state.currentSong ? currentTrackTitle : 'Nenhuma música tocando no momento' }}
        </div>
        <div class="ytm-byline truncate">
          <template v-if="store.state.currentSong">
            <span>RadioSync</span>
            <span class="ytm-dot">•</span>
            <span>Estação {{ store.stationId }}</span>
            <template v-if="currentTrackMeta">
              <span class="ytm-dot">•</span>
              <span class="text-emerald">{{ currentTrackMeta }}</span>
            </template>
            <template v-if="buffering">
              <span class="ytm-dot">•</span>
              <span class="text-amber">Sincronizando...</span>
            </template>
          </template>
          <template v-else>
            <span>{{ isDJ ? 'Selecione ou adicione uma faixa na playlist para iniciar' : 'Aguardando o DJ iniciar a transmissão' }}</span>
          </template>
        </div>
      </div>
    </div>

    <!-- Right Controls: Volume + Repetir -->
    <div class="ytm-right-controls">
      <div class="ytm-volume-group">
        <input
          type="range"
          min="0"
          max="100"
          v-model.number="volume"
          @input="setVolume"
          class="range-slider ytm-volume-slider"
          title="Volume"
          aria-label="Volume"
        />
        <button
          @click="toggleMute"
          class="ytm-icon-btn"
          :title="muted ? 'Ativar som' : 'Desativar som'"
          :aria-label="muted ? 'Ativar som' : 'Desativar som'"
        >
          <VolumeX v-if="muted || volume === 0" class="icon-lg text-red" />
          <Volume2 v-else-if="volume > 50" class="icon-lg" />
          <Volume1 v-else class="icon-lg" />
        </button>
      </div>

      <button
        @click="toggleRepeat"
        class="ytm-icon-btn"
        :class="{ active: repeat }"
        :title="repeat ? 'Repetição ativada' : 'Repetição desativada'"
        :aria-label="repeat ? 'Repetição ativada' : 'Repetição desativada'"
      >
        <Repeat class="icon-lg" />
      </button>
    </div>
  </footer>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import {
  Play,
  Pause,
  SkipForward,
  RotateCcw,
  RotateCw,
  Volume1,
  Volume2,
  VolumeX,
  Repeat,
  Music,
  Disc,
  Loader2,
} from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { usePlayer } from '../composables/usePlayer'

const emit = defineEmits(['togglePlay'])
const store = useStationStore()
const audioRef = ref(null)
const { currentTime, duration, buffering, startTimeUpdates, syncPlayback } = usePlayer(audioRef)

const VOLUME_KEY = 'radiosync_volume'
const REPEAT_KEY = 'radiosync_repeat'

const volume = ref(parseInt(localStorage.getItem(VOLUME_KEY) || '80'))
const muted = ref(false)
const repeat = ref(localStorage.getItem(REPEAT_KEY) === 'true')
const previousVolume = ref(80)

const hoverTime = ref(null)
const hoverPercent = ref(0)

const isDJ = computed(() => store.role === 'dj')

const currentTrackObj = computed(() => {
  if (store.playlist && store.playlist.length > 0) {
    return store.playlist[0]
  }
  if (store.repository && store.repository.length > 0 && store.state.currentSong) {
    return store.repository.find(t => t.url === store.state.currentSong) || null
  }
  return null
})

const currentTrackTitle = computed(() => {
  if (currentTrackObj.value && currentTrackObj.value.title) {
    return currentTrackObj.value.title
  }
  if (store.state.currentSong) {
    const parts = store.state.currentSong.split('/')
    const filename = parts[parts.length - 1]
    return filename ? filename.replace('.opus', '') : 'Música em reprodução'
  }
  return 'Música em reprodução'
})

const currentTrackMeta = computed(() => {
  if (!currentTrackObj.value) return ''
  const parts = []
  if (currentTrackObj.value.category) parts.push(currentTrackObj.value.category)
  if (currentTrackObj.value.theme) parts.push(currentTrackObj.value.theme)
  return parts.join(' / ')
})

const progressPercent = computed(() => {
  if (!duration.value) return 0
  return Math.min(100, Math.max(0, (currentTime.value / duration.value) * 100))
})

watch(audioRef, (audio) => {
  if (audio) {
    audio.volume = muted.value ? 0 : volume.value / 100
    audio.loop = repeat.value
  }
})

watch(repeat, (val) => {
  localStorage.setItem(REPEAT_KEY, val)
  if (audioRef.value) audioRef.value.loop = val
})

function formatTime(seconds) {
  if (!seconds || isNaN(seconds)) return '0:00'
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

function togglePlay() {
  if (!isDJ.value || !store.state.currentSong) return
  emit('togglePlay', store.state.isPlaying ? 'PAUSE' : 'PLAY')
}

function nextTrack() {
  if (!isDJ.value) return
  emit('togglePlay', 'NEXT_TRACK')
}

function seekRelative(deltaSeconds) {
  if (!isDJ.value || !duration.value) return
  const target = Math.max(0, Math.min(duration.value, currentTime.value + deltaSeconds))
  emit('togglePlay', 'SEEK', target)
}

function setVolume() {
  const val = volume.value
  localStorage.setItem(VOLUME_KEY, val)
  muted.value = false
  if (audioRef.value) audioRef.value.volume = val / 100
}

function toggleMute() {
  muted.value = !muted.value
  if (audioRef.value) {
    if (muted.value) {
      previousVolume.value = volume.value
      audioRef.value.volume = 0
    } else {
      volume.value = previousVolume.value
      audioRef.value.volume = volume.value / 100
    }
  }
}

function toggleRepeat() {
  repeat.value = !repeat.value
}

function seek(e) {
  if (!isDJ.value || !duration.value) return
  const rect = e.currentTarget.getBoundingClientRect()
  const pos = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
  const seekTime = pos * duration.value
  emit('togglePlay', 'SEEK', seekTime)
}

function onProgressHover(e) {
  if (!duration.value) return
  const rect = e.currentTarget.getBoundingClientRect()
  const pos = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
  hoverPercent.value = pos * 100
  hoverTime.value = pos * duration.value
}

function onTimeUpdate() {
  if (!audioRef.value || !store.state.isPlaying) return
  const audio = audioRef.value
  if (store.state.isPlaying && audio.paused && !repeat.value) {
    audio.play().catch(() => {})
  }
}

function onEnded() {
  if (repeat.value) return
  if (isDJ.value) {
    emit('togglePlay', 'NEXT_TRACK')
  }
}

defineExpose({ syncPlayback })
</script>

<style scoped>
.ytm-player-bar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 4.5rem;
  background-color: #121215;
  border-top: 1px solid rgba(63, 63, 70, 0.5);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 1rem;
  z-index: 40;
  user-select: none;
  box-shadow: 0 -8px 24px rgba(0, 0, 0, 0.55);
}

/* Top Edge Progress Bar (YouTube Music Style) */
.ytm-progress-container {
  position: absolute;
  top: -6px;
  left: 0;
  right: 0;
  height: 14px;
  display: flex;
  align-items: center;
  z-index: 45;
}

.ytm-progress-container.is-interactive {
  cursor: pointer;
}

.ytm-progress-rail {
  width: 100%;
  height: 3px;
  background-color: rgba(255, 255, 255, 0.15);
  overflow: hidden;
  transition: height 0.15s ease;
}

.ytm-progress-container:hover .ytm-progress-rail {
  height: 5px;
}

.ytm-progress-primary {
  height: 100%;
  background-color: var(--emerald-500);
  transition: width 0.1s linear;
}

.ytm-progress-knob {
  position: absolute;
  top: 50%;
  width: 12px;
  height: 12px;
  border-radius: var(--radius-full);
  background-color: var(--emerald-400);
  transform: translate(-50%, -50%) scale(0);
  transition: transform 0.15s ease;
  box-shadow: 0 0 6px rgba(16, 185, 129, 0.6);
  pointer-events: none;
}

.ytm-progress-container:hover .ytm-progress-knob {
  transform: translate(-50%, -50%) scale(1);
}

.ytm-hover-time {
  position: absolute;
  bottom: 16px;
  transform: translateX(-50%);
  background-color: var(--zinc-800);
  color: var(--zinc-100);
  font-size: 0.6875rem;
  font-family: var(--font-mono);
  padding: 0.125rem 0.375rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--zinc-700);
  pointer-events: none;
  white-space: nowrap;
}

/* Left Controls */
.ytm-left-controls {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  min-width: 180px;
  width: 280px;
}

.ytm-left-buttons {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.ytm-icon-btn {
  width: 2.25rem;
  height: 2.25rem;
  border-radius: var(--radius-full);
  color: var(--zinc-300);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: background-color 0.15s ease, color 0.15s ease;
}

.ytm-icon-btn:hover:not(:disabled) {
  background-color: rgba(255, 255, 255, 0.1);
  color: #ffffff;
}

.ytm-icon-btn:disabled {
  opacity: 0.35;
}

.ytm-icon-btn.active {
  color: var(--emerald-400);
}

.ytm-play-pause-btn {
  width: 2.75rem;
  height: 2.75rem;
  border-radius: var(--radius-full);
  background-color: #ffffff;
  color: var(--zinc-950);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transition: transform 0.15s ease, background-color 0.15s ease, opacity 0.15s ease;
}

.ytm-play-pause-btn:hover:not(:disabled) {
  transform: scale(1.06);
  background-color: var(--emerald-400);
}

.ytm-play-pause-btn:disabled {
  opacity: 0.4;
}

.ytm-play-pause-btn.listener-only:disabled {
  opacity: 0.85;
  background-color: var(--zinc-800);
  color: var(--emerald-400);
  cursor: default;
}

.play-offset {
  margin-left: 2px;
}

.ytm-time-info {
  font-size: 0.75rem;
  color: var(--zinc-400);
  font-family: var(--font-mono);
  white-space: nowrap;
}

/* Middle Controls */
.ytm-middle-controls {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.875rem;
  flex: 1;
  min-width: 0;
  max-width: 36rem;
}

.ytm-thumbnail-wrapper {
  width: 2.75rem;
  height: 2.75rem;
  border-radius: var(--radius-md);
  background-color: var(--zinc-800);
  border: 1px solid var(--zinc-700);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  overflow: hidden;
}

.ytm-thumb-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: var(--radius-md);
}

.ytm-thumbnail-wrapper.is-playing {
  background-color: rgba(5, 150, 105, 0.15);
  border-color: rgba(16, 185, 129, 0.4);
}

.ytm-content-info {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.ytm-title {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--zinc-100);
  line-height: 1.25;
}

.ytm-byline {
  font-size: 0.75rem;
  color: var(--zinc-400);
  display: flex;
  align-items: center;
  gap: 0.375rem;
  margin-top: 0.125rem;
}

.ytm-dot {
  color: var(--zinc-600);
}

/* Right Controls */
.ytm-right-controls {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem;
  min-width: 140px;
  width: 280px;
}

.ytm-volume-group {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.ytm-volume-slider {
  width: 5.5rem;
}

.ytm-volume-slider::-webkit-slider-thumb {
  background-color: #ffffff;
}

.ytm-volume-slider::-moz-range-thumb {
  background-color: #ffffff;
}

@media (max-width: 768px) {
  .ytm-left-controls,
  .ytm-right-controls {
    width: auto;
    min-width: auto;
  }

  .ytm-time-info,
  .ytm-volume-slider {
    display: none;
  }
}
</style>
