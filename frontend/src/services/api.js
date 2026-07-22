import axios from 'axios'

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || '',
})

export function setDJToken(token) {
  if (token) {
    api.defaults.headers.common['Authorization'] = `Bearer ${token}`
  } else {
    delete api.defaults.headers.common['Authorization']
  }
}

export async function createStation() {
  const { data } = await api.post('/api/v1/stations')
  return data
}

export async function getStation(stationId) {
  const { data } = await api.get(`/api/v1/stations/${stationId}`)
  return data
}

export async function uploadMusic(stationId, file, token) {
  const form = new FormData()
  form.append('file', file)
  const { data } = await api.post(`/api/v1/stations/${stationId}/upload`, form, {
    headers: { 'Authorization': `Bearer ${token}` },
  })
  return data
}

export async function downloadFromYouTube(stationId, youtubeUrl, token) {
  const { data } = await api.post(`/api/v1/stations/${stationId}/youtube`, { url: youtubeUrl }, {
    headers: { 'Authorization': `Bearer ${token}` },
  })
  return data
}

export async function getRepository(stationId, token) {
  const { data } = await api.get(`/api/v1/stations/${stationId}/repository`, {
    headers: { 'Authorization': `Bearer ${token}` },
  })
  return data
}

export async function listMusicFiles(stationId) {
  const { data } = await api.get(`/api/v1/stations/${stationId}/musicas`)
  return data
}

export async function getGlobalLibrary() {
  const { data } = await api.get('/api/v1/library')
  return data
}

export async function getAdminStations() {
  const { data } = await api.get('/api/v1/admin/stations')
  return data
}

export async function deleteStationAdmin(stationId) {
  const { data } = await api.delete(`/api/v1/admin/stations/${stationId}`)
  return data
}

export async function purgeOrphanTracksAdmin() {
  const { data } = await api.post('/api/v1/admin/purge-orphans')
  return data
}

export async function removeTrackFromStationAdmin(stationId, trackId) {
  const { data } = await api.delete(`/api/v1/admin/stations/${stationId}/tracks/${trackId}`)
  return data
}

export async function saveYouTubeCookies(stationId, cookiesContent, token) {
  const { data } = await api.post(`/api/v1/stations/${stationId}/cookies`, { content: cookiesContent }, {
    headers: { 'Authorization': `Bearer ${token}` },
  })
  return data
}

export async function getCookiesStatus(stationId) {
  const { data } = await api.get(`/api/v1/stations/${stationId}/cookies/status`)
  return data
}

export default api
