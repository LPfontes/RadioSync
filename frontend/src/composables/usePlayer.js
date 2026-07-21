import { ref, watch, onUnmounted } from 'vue'
import { useStationStore } from '../stores/station'

const SYNC_THRESHOLD = 1.5

export function usePlayer(audioRef) {
  const store = useStationStore()
  const currentTime = ref(0)
  const duration = ref(0)
  const buffering = ref(false)
  let rafId = null

  function syncPlayback() {
    const audio = audioRef.value
    if (!audio || !store.state.currentSong) return

    if (store.state.isPlaying) {
      const elapsed = (Date.now() - store.state.startedAt) / 1000
      const expectedPosition = elapsed + store.state.seekOffset

      if (audio.paused) {
        audio.play().catch(() => {})
      }

      const diff = Math.abs(audio.currentTime - expectedPosition)
      if (diff > SYNC_THRESHOLD && expectedPosition < audio.duration) {
        audio.currentTime = expectedPosition
      }
    } else {
      if (!audio.paused) {
        audio.pause()
      }
      if (store.state.seekOffset > 0) {
        audio.currentTime = store.state.seekOffset
      }
    }
  }

  function startTimeUpdates() {
    const audio = audioRef.value
    if (!audio) return
    function update() {
      currentTime.value = audio.currentTime
      duration.value = audio.duration || store.state.duration
      rafId = requestAnimationFrame(update)
    }
    update()
  }

  watch(() => store.state.isPlaying, syncPlayback)
  watch(() => store.state.seekOffset, syncPlayback)
  watch(() => store.state.startedAt, syncPlayback)

  watch(() => store.state.currentSong, (url) => {
    const audio = audioRef.value
    if (audio && url) {
      audio.src = url
      audio.load()
      syncPlayback()
    }
  })

  watch(audioRef, (audio) => {
    if (audio && store.state.currentSong && !audio.src) {
      audio.src = store.state.currentSong
      audio.load()
    }
  })

  onUnmounted(() => {
    if (rafId) cancelAnimationFrame(rafId)
  })

  return { currentTime, duration, buffering, syncPlayback, startTimeUpdates }
}
