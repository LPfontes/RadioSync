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

export async function getStation(stationId, token = null) {
  const config = {}
  if (token) {
    config.headers = { Authorization: `Bearer ${token}` }
  }
  const { data } = await api.get(`/api/v1/stations/${stationId}`, config)
  return data
}

export async function claimDJRole(stationId) {
  const { data } = await api.post(`/api/v1/stations/${stationId}/claim-dj`)
  return data
}

export async function uploadMusic(stationId, file, token, category = '', theme = '') {
  const form = new FormData()
  form.append('file', file)
  if (category) form.append('category', category)
  if (theme) form.append('theme', theme)
  const { data } = await api.post(`/api/v1/stations/${stationId}/upload`, form, {
    headers: { 'Authorization': `Bearer ${token}` },
  })
  return data
}

export async function downloadFromYouTube(stationId, youtubeUrl, token, category = '', theme = '') {
  const { data } = await api.post(`/api/v1/stations/${stationId}/youtube`, { url: youtubeUrl, category, theme }, {
    headers: { 'Authorization': `Bearer ${token}` },
  })
  return data
}

export async function updateTrackMetadata(stationId, trackId, { title, category, theme, coverUrl }, token) {
  const { data } = await api.put(`/api/v1/stations/${stationId}/tracks/${trackId}`, { title, category, theme, coverUrl }, {
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

// ---------------------------------------------------------------- Admin
// O token de admin fica no sessionStorage (some ao fechar a aba) e é enviado
// explicitamente, sem passar pelo header padrão usado pelo token de DJ.
const ADMIN_TOKEN_KEY = 'radiosync_admin_token'

export function getAdminToken() {
  try {
    const raw = sessionStorage.getItem(ADMIN_TOKEN_KEY)
    if (!raw) return null
    const { token, expiresAt } = JSON.parse(raw)
    if (!token || (expiresAt && Date.now() >= expiresAt)) {
      sessionStorage.removeItem(ADMIN_TOKEN_KEY)
      return null
    }
    return token
  } catch {
    return null
  }
}

export function clearAdminToken() {
  try { sessionStorage.removeItem(ADMIN_TOKEN_KEY) } catch { /* ignore */ }
}

function adminConfig() {
  const token = getAdminToken()
  return { headers: { Authorization: token ? `Bearer ${token}` : '' } }
}

export async function adminLogin(password) {
  const { data } = await api.post('/api/v1/admin/login', { password })
  try {
    sessionStorage.setItem(ADMIN_TOKEN_KEY, JSON.stringify({ token: data.token, expiresAt: data.expiresAt }))
  } catch { /* ignore */ }
  return data
}

export async function getAdminStations() {
  const { data } = await api.get('/api/v1/admin/stations', adminConfig())
  return data
}

export async function issueDJTokenAdmin(stationId) {
  const { data } = await api.post(`/api/v1/admin/stations/${stationId}/dj-token`, null, adminConfig())
  return data.djToken
}

export async function deleteStationAdmin(stationId) {
  const { data } = await api.delete(`/api/v1/admin/stations/${stationId}`, adminConfig())
  return data
}

export async function purgeOrphanTracksAdmin() {
  const { data } = await api.post('/api/v1/admin/purge-orphans', null, adminConfig())
  return data
}

export async function removeTrackFromStationAdmin(stationId, trackId) {
  const { data } = await api.delete(`/api/v1/admin/stations/${stationId}/tracks/${trackId}`, adminConfig())
  return data
}

export async function renameTrackAdmin(trackId, title) {
  const { data } = await api.put(`/api/v1/admin/tracks/${trackId}`, { title }, adminConfig())
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
