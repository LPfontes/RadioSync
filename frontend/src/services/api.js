import axios from 'axios'

const api = axios.create({
  baseURL: import.meta.env.VITE_API_URL || 'http://localhost:8080',
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

export default api
