<template>
  <div class="min-h-screen bg-zinc-900 text-zinc-100 flex items-start justify-center pt-12">
    <div class="max-w-md w-full mx-4 space-y-6">
      <div class="text-center space-y-2">
        <h1 class="text-3xl font-bold">RadioSync</h1>
        <p class="text-zinc-400">Rádio web sincronizada em tempo real</p>
      </div>

      <div v-if="savedStations.length > 0" class="bg-zinc-800 rounded-lg p-4 space-y-2">
        <h2 class="text-xs font-medium text-zinc-400 uppercase tracking-wide">Salas Recentes</h2>
        <button v-for="s in savedStations" :key="s.id" @click="enterSaved(s)" class="w-full flex items-center gap-3 px-3 py-2 rounded-lg hover:bg-zinc-700 transition-colors text-left">
          <div class="w-8 h-8 rounded-full bg-emerald-600/20 flex items-center justify-center text-xs font-bold text-emerald-500">
            {{ s.role === 'dj' ? 'DJ' : 'L' }}
          </div>
          <div class="flex-1 min-w-0">
            <span class="text-sm font-medium">{{ s.id }}</span>
            <span class="text-xs text-zinc-500 ml-2">{{ s.role === 'dj' ? '(DJ)' : '(Ouvinte)' }}</span>
          </div>
          <button @click.stop="remove(s.id)" class="text-zinc-600 hover:text-zinc-400">
            <X class="w-3.5 h-3.5" />
          </button>
        </button>
      </div>

      <div class="bg-zinc-800 rounded-lg p-6 space-y-4">
        <button @click="createRoom" :disabled="loading" class="w-full py-3 bg-emerald-600 hover:bg-emerald-500 disabled:bg-zinc-600 rounded-lg font-medium transition-colors">
          {{ loading ? 'Criando...' : 'Criar Estação (DJ)' }}
        </button>

        <div class="relative">
          <div class="absolute inset-0 flex items-center">
            <div class="w-full border-t border-zinc-700"></div>
          </div>
          <div class="relative flex justify-center text-xs">
            <span class="bg-zinc-800 px-2 text-zinc-400">ou</span>
          </div>
        </div>

        <div class="flex gap-2">
          <input v-model="joinId" placeholder="Código da estação" @keyup.enter="joinRoom" class="flex-1 bg-zinc-700 rounded-lg px-4 py-2.5 text-sm outline-none focus:ring-2 focus:ring-emerald-500 uppercase" />
          <button @click="joinRoom" :disabled="!joinId" class="px-4 py-2.5 bg-zinc-700 hover:bg-zinc-600 disabled:opacity-50 rounded-lg text-sm font-medium transition-colors">
            Entrar
          </button>
        </div>
      </div>

      <p v-if="error" class="text-red-400 text-sm text-center">{{ error }}</p>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { X } from 'lucide-vue-next'
import { createStation, getStation } from '../services/api'
import { useStationStore } from '../stores/station'
import { getSavedStations, saveStation, removeStation } from '../services/storage'

const router = useRouter()
const store = useStationStore()
const joinId = ref('')
const loading = ref(false)
const error = ref('')
const savedStations = ref(getSavedStations())

async function createRoom() {
  loading.value = true
  error.value = ''
  try {
    const data = await createStation()
    store.setStation(data.stationId, data.djToken, 'dj')
    saveStation(data.stationId, 'dj', data.djToken)
    router.push(`/dj/${data.stationId}`)
  } catch (e) {
    error.value = 'Erro ao criar estação'
  } finally {
    loading.value = false
  }
}

async function joinRoom() {
  const code = joinId.value.trim().toUpperCase()
  if (!code) return
  error.value = ''
  try {
    await getStation(code)
    store.setStation(code, null, 'listener')
    saveStation(code, 'listener', null)
    router.push(`/radio/${code}`)
  } catch (e) {
    error.value = 'Estação não encontrada'
  }
}

function enterSaved(s) {
  store.setStation(s.id, s.token, s.role)
  const route = s.role === 'dj' ? `/dj/${s.id}` : `/radio/${s.id}`
  router.push(route)
}

function remove(id) {
  removeStation(id)
  savedStations.value = getSavedStations()
}
</script>
