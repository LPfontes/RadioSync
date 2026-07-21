<template>
  <div class="w-full">
    <audio ref="audioRef" preload="auto"
      @loadedmetadata="startTimeUpdates"
      @waiting="buffering = true"
      @canplay="buffering = false"
      @timeupdate="onTimeUpdate"
      @ended="onEnded">
      <source :src="store.state.currentSong" type="audio/ogg; codecs=opus" />
    </audio>

    <div v-if="!store.state.currentSong" class="text-zinc-400 text-sm text-center py-8">
      Nenhuma música tocando no momento
    </div>

    <div v-else class="space-y-3">
      <div class="flex items-center gap-3">
        <button @click="togglePlay" class="p-2 rounded-full bg-emerald-600 hover:bg-emerald-500 transition-colors shrink-0" :disabled="!store.state.currentSong">
          <Play v-if="!store.state.isPlaying" class="w-5 h-5 text-white" />
          <Pause v-else class="w-5 h-5 text-white" />
        </button>

        <div class="flex-1 min-w-0">
          <div class="relative h-1.5 bg-zinc-700 rounded-full overflow-hidden cursor-pointer group" @click="seek">
            <div class="h-full bg-emerald-500 rounded-full transition-all duration-100" :style="{ width: progressPercent + '%' }"></div>
            <div class="absolute top-1/2 -translate-y-1/2 w-3 h-3 bg-white rounded-full shadow opacity-0 group-hover:opacity-100 transition-opacity" :style="{ left: `calc(${progressPercent}% - 6px)` }"></div>
          </div>
          <div class="flex justify-between text-xs text-zinc-400 mt-1">
            <span>{{ formatTime(currentTime) }}</span>
            <span>{{ formatTime(duration) }}</span>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <button @click="toggleMute" class="p-1.5 rounded hover:bg-zinc-700 transition-colors" :title="muted ? 'Ativar som' : 'Silenciar'">
          <VolumeX v-if="muted" class="w-4 h-4 text-zinc-400" />
          <Volume2 v-else-if="volume > 50" class="w-4 h-4 text-zinc-400" />
          <Volume1 v-else-if="volume > 0" class="w-4 h-4 text-zinc-400" />
          <Volume v-else class="w-4 h-4 text-zinc-400" />
        </button>
        <input type="range" min="0" max="100" v-model.number="volume" @input="setVolume"
          class="w-24 h-1.5 appearance-none bg-zinc-700 rounded-full cursor-pointer accent-emerald-500 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:w-3 [&::-webkit-slider-thumb]:h-3 [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-white" />

        <div class="flex-1"></div>

        <button @click="toggleRepeat" class="p-1.5 rounded transition-colors"
          :class="repeat ? 'text-emerald-400 hover:text-emerald-300' : 'text-zinc-500 hover:text-zinc-300'"
          :title="repeat ? 'Repetir ativado' : 'Repetir desativado'">
          <Repeat class="w-4 h-4" />
        </button>
      </div>

      <div v-if="buffering" class="text-xs text-emerald-400 text-center">Carregando...</div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { Play, Pause, Volume, Volume1, Volume2, VolumeX, Repeat } from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { usePlayer } from '../composables/usePlayer'

const emit = defineEmits(['togglePlay'])
const store = useStationStore()
const audioRef = ref(null)
const { currentTime, duration, buffering, startTimeUpdates } = usePlayer(audioRef)

const VOLUME_KEY = 'radiosync_volume'
const REPEAT_KEY = 'radiosync_repeat'

const volume = ref(parseInt(localStorage.getItem(VOLUME_KEY) || '80'))
const muted = ref(false)
const repeat = ref(localStorage.getItem(REPEAT_KEY) === 'true')
const previousVolume = ref(80)

const progressPercent = computed(() => {
  if (!duration.value) return 0
  return (currentTime.value / duration.value) * 100
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
  if (store.role !== 'dj') return
  emit('togglePlay', store.state.isPlaying ? 'PAUSE' : 'PLAY')
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
  if (store.role !== 'dj') return
  const rect = e.currentTarget.getBoundingClientRect()
  const pos = (e.clientX - rect.left) / rect.width
  const seekTime = pos * duration.value
  emit('togglePlay', 'SEEK', seekTime)
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
  if (store.role === 'dj') {
    emit('togglePlay', 'NEXT_TRACK')
  }
}
</script>
