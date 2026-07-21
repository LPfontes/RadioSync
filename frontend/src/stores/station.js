import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useStationStore = defineStore('station', () => {
  const stationId = ref(null)
  const role = ref(null)
  const djToken = ref(null)
  const playlist = ref([])
  const repository = ref([])
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

  function reset() {
    stationId.value = null
    role.value = null
    djToken.value = null
    playlist.value = []
    repository.value = []
    state.value = { isPlaying: false, startedAt: 0, seekOffset: 0, currentSong: '', duration: 0 }
  }

  return {
    stationId, role, djToken, playlist, repository, state,
    setStation, setState, setPlaylist, setRepository, reset,
  }
})
