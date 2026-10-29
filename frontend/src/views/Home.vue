<template>
  <div class="home-page">
    <div class="home-container">
      <div class="hero-header">
        <div class="hero-icon-box">
          <Disc class="icon-2xl animate-spin-slow" />
        </div>
        <h1 class="hero-title">
          <span>RadioSync</span>
        </h1>
        <p class="hero-subtitle">Rádio web sincronizada em tempo real</p>
      </div>

      <div v-if="savedStations.length > 0" class="card recent-card">
        <h2 class="section-heading">Salas Recentes</h2>
        <div
          v-for="s in savedStations"
          :key="s.id"
          @click="enterSaved(s)"
          class="recent-item"
        >
          <div class="recent-badge">
            {{ s.role === 'dj' ? 'DJ' : 'L' }}
          </div>
          <div class="recent-info">
            <span class="recent-id">{{ s.id }}</span>
            <span class="recent-role">{{ s.role === 'dj' ? '(DJ)' : '(Ouvinte)' }}</span>
          </div>
          <button @click.stop="remove(s.id)" class="recent-remove-btn">
            <X class="icon-sm" />
          </button>
        </div>
      </div>

      <div class="card-lg actions-card">
        <button @click="createRoom" :disabled="loading" class="btn btn-primary btn-lg btn-block">
          {{ loading ? 'Criando...' : 'Criar Estação (DJ)' }}
        </button>

        <div class="divider">
          <div class="divider-line"></div>
          <span class="divider-text">ou</span>
        </div>

        <div class="join-row">
          <input
            v-model="joinId"
            placeholder="Código da estação"
            @keyup.enter="joinRoom"
            class="input join-input"
          />
          <button @click="joinRoom" :disabled="!joinId" class="btn btn-secondary join-btn">
            Entrar
          </button>
        </div>
      </div>

      <div class="admin-link-wrapper">
        <router-link to="/admin" class="admin-link">
          <Shield class="icon-sm" />
          Painel de Administração
        </router-link>
      </div>

      <p v-if="error" class="error-msg">{{ error }}</p>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { X, Shield, Disc } from 'lucide-vue-next'
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
    router.push(`/radio/${data.stationId}`)
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
  router.push(`/radio/${s.id}`)
}

function remove(id) {
  removeStation(id)
  savedStations.value = getSavedStations()
}
</script>

<style scoped>
.home-page {
  min-height: 100vh;
  background-color: var(--zinc-900);
  color: var(--zinc-100);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 3rem;
}

.home-container {
  max-width: 28rem;
  width: 100%;
  margin: 0 1rem;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.hero-header {
  text-align: center;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
}

.hero-icon-box {
  width: 4rem;
  height: 4rem;
  border-radius: var(--radius-2xl);
  background-color: rgba(5, 150, 105, 0.2);
  border: 1px solid rgba(16, 185, 129, 0.3);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--emerald-400);
  box-shadow: 0 10px 15px -3px rgba(2, 44, 34, 0.5);
  margin-bottom: 0.25rem;
}

.hero-title {
  font-size: 1.875rem;
  line-height: 2.25rem;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
}

.hero-subtitle {
  color: var(--zinc-400);
  font-size: 0.875rem;
}

.recent-card {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.section-heading {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--zinc-400);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.recent-item {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 0.75rem;
  border-radius: var(--radius-lg);
  text-align: left;
  cursor: pointer;
  transition: background-color 0.15s ease;
}

.recent-item:hover {
  background-color: var(--zinc-700);
}

.recent-badge {
  width: 2rem;
  height: 2rem;
  border-radius: var(--radius-full);
  background-color: rgba(5, 150, 105, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--emerald-500);
}

.recent-info {
  flex: 1;
  min-width: 0;
}

.recent-id {
  font-size: 0.875rem;
  font-weight: 500;
}

.recent-role {
  font-size: 0.75rem;
  color: var(--zinc-500);
  margin-left: 0.5rem;
}

.recent-remove-btn {
  color: var(--zinc-600);
  padding: 0.25rem;
  border-radius: var(--radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: color 0.15s ease, background-color 0.15s ease;
}

.recent-remove-btn:hover {
  color: var(--zinc-400);
  background-color: rgba(82, 82, 91, 0.5);
}

.actions-card {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.divider {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
}

.divider-line {
  position: absolute;
  inset: 0;
  top: 50%;
  border-top: 1px solid var(--zinc-700);
}

.divider-text {
  position: relative;
  background-color: var(--zinc-800);
  padding: 0 0.5rem;
  font-size: 0.75rem;
  color: var(--zinc-400);
}

.join-row {
  display: flex;
  gap: 0.5rem;
}

.join-input {
  flex: 1;
  padding: 0.625rem 1rem;
  font-size: 0.875rem;
  text-transform: uppercase;
}

.join-btn {
  padding: 0.625rem 1rem;
  font-size: 0.875rem;
}

.admin-link-wrapper {
  text-align: center;
  padding-top: 0.5rem;
}

.admin-link {
  font-size: 0.75rem;
  color: var(--zinc-400);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  transition: color 0.15s ease;
}

.admin-link:hover {
  color: var(--emerald-400);
}

.error-msg {
  color: var(--red-400);
  font-size: 0.875rem;
  text-align: center;
}
</style>
