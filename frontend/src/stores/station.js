import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useStationStore = defineStore('station', () => {
  const stationId = ref(null)
  const role = ref(null)
  const djToken = ref(null)
  const playlist = ref([])
  const repository = ref([])
  const suggestions = ref([])
  const mixer = ref([])
  const jobs = ref([]) // Uploads e downloads em processamento (ADR-0002)
  const hasDJOnline = ref(false)
  const clockOffset = ref(0) // Offset de relógio (servidor - cliente) em ms (ADR-0004)
  const state = ref({
    isPlaying: false,
    startedAt: 0,
    seekOffset: 0,
    currentSong: '',
    duration: 0,
  })

  function setStation(id, token, userRole) {
    stationId.value = id
    djToken.value = token
    role.value = userRole
  }

  function setState(newState) {
    state.value = { ...newState }
  }

  function setPlaylist(newPlaylist) {
    playlist.value = newPlaylist
  }

  function setRepository(repo) {
    repository.value = repo || []
  }

  function upsertTrack(track) {
    if (!track?.id) return
    const index = repository.value.findIndex(t => t.id === track.id)
    if (index === -1) repository.value.push(track)
    else repository.value[index] = { ...repository.value[index], ...track }
  }

  // Aplica o andamento de um job (resposta da API ou evento do WebSocket).
  // Pronto: sai da lista e a faixa entra no repositório. Os demais ficam na lista.
  function applyJob(job) {
    if (!job?.jobId) return
    if (job.status === 'ready') {
      upsertTrack(job.track)
      removeJob(job.jobId)
      return
    }
    const index = jobs.value.findIndex(j => j.jobId === job.jobId)
    if (index === -1) jobs.value.push(job)
    else jobs.value[index] = { ...jobs.value[index], ...job }
  }

  function removeJob(jobId) {
    jobs.value = jobs.value.filter(j => j.jobId !== jobId)
  }

  function setSuggestions(newSuggestions) {
    suggestions.value = newSuggestions || []
  }

  function setMixer(newMixer) {
    mixer.value = Array.isArray(newMixer) ? [...newMixer] : []
  }

  function reset() {
    stationId.value = null
    role.value = null
    djToken.value = null
    playlist.value = []
    repository.value = []
    suggestions.value = []
    mixer.value = []
    jobs.value = []
    hasDJOnline.value = false
    clockOffset.value = 0
    state.value = { isPlaying: false, startedAt: 0, seekOffset: 0, currentSong: '', duration: 0 }
  }

  return {
    stationId, role, djToken, hasDJOnline, clockOffset, playlist, repository, suggestions, mixer, jobs, state,
    setStation, setState, setPlaylist, setRepository, upsertTrack, applyJob, removeJob, setSuggestions, setMixer, reset,
  }
})
