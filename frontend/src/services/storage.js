const STORAGE_KEY = 'radiosync_stations'

export function getSavedStations() {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]')
  } catch {
    return []
  }
}

export function saveStation(id, role, token) {
  const stations = getSavedStations().filter(s => s.id !== id)
  stations.unshift({ id, role, token, visitedAt: Date.now() })
  localStorage.setItem(STORAGE_KEY, JSON.stringify(stations.slice(0, 10)))
}

export function removeStation(id) {
  const stations = getSavedStations().filter(s => s.id !== id)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(stations))
}
