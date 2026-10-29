<template>
  <div class="app-page">
    <header class="app-header">
      <div class="header-inner">
        <div class="header-brand">
          <Disc class="icon-lg text-emerald" />
          <span class="brand-name">RadioSync — DJ</span>
          <span class="badge-code">{{ store.stationId }}</span>
        </div>
        <div class="header-actions">
          <button
            @click="switchToListener"
            class="btn-text amber-hover"
            title="Voltar a ser ouvinte liberando a vaga de DJ"
          >
            Entregar cargo de DJ
          </button>
          <button @click="leave" class="btn-text">Sair</button>
        </div>
      </div>
    </header>

    <div class="dj-main">
      <div class="station-code-card">
        <div class="station-actions-grid">
          <!-- Coluna 1: Código da Estação -->
          <div class="station-code-block station-action-block">
            <p class="station-code-label">Código da estação</p>
            <div class="station-code-row">
              <span class="station-code-value">{{ store.stationId }}</span>
              <button @click="copyCode" class="btn-text copy-btn" title="Copiar código">
                <Copy class="icon-md" />
              </button>
            </div>
            <span class="station-code-sub">Compartilhe com seus ouvintes</span>
          </div>

          <!-- Coluna 2: Upload de Músicas -->
          <div class="station-action-block">
            <h3 class="subsection-title">Upload de Músicas</h3>
            <div class="upload-section">
              <input
                type="file"
                ref="fileInputRef"
                accept=".mp3,.mp4,.webm,.wav,.ogg,.flac,.aac,.m4a"
                @change="selectFile"
                class="file-input"
              />
              <div v-if="selectedFile" class="file-meta-box">
                <span class="selected-filename truncate">{{ selectedFile.name }}</span>
                <div class="two-col-grid">
                  <select v-model="uploadCategory" class="select">
                    <option value="">-- Categoria (Opcional) --</option>
                    <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
                  </select>
                  <select v-model="uploadTheme" class="select">
                    <option value="">-- Tema (Opcional) --</option>
                    <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
                  </select>
                </div>
                <div class="flex-end">
                  <button @click="handleUpload" :disabled="uploading" class="btn btn-primary">
                    {{ uploading ? 'Enviando...' : 'Fazer Upload' }}
                  </button>
                </div>
              </div>
              <p v-if="uploadError" class="error-text">{{ uploadError }}</p>
            </div>
          </div>

          <!-- Download do YouTube -->
          <div class="station-action-block yt-section">
            <h3 class="subsection-title-icon">
              <Youtube class="icon-md text-red" />
              Baixar do YouTube (yt-dlp)
            </h3>
            <div class="yt-form">
              <div class="yt-input-row">
                <input
                  v-model="youtubeUrl"
                  placeholder="URL do vídeo (ex: https://youtu.be/...)"
                  @keyup.enter="handleYouTubeDownload"
                  class="input yt-input"
                />
                <button
                  @click="handleYouTubeDownload"
                  :disabled="downloadingYT || !youtubeUrl.trim()"
                  class="btn btn-danger"
                >
                  {{ downloadingYT ? 'Baixando...' : 'Baixar' }}
                </button>
              </div>
              <div v-if="youtubeUrl.trim()" class="file-meta-box two-col-grid">
                <select v-model="ytCategory" class="select">
                  <option value="">-- Categoria (Opcional) --</option>
                  <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
                </select>
                <select v-model="ytTheme" class="select">
                  <option value="">-- Tema (Opcional) --</option>
                  <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
                </select>
              </div>
            </div>

            <!-- Indicador e Botão de Cookies -->
            <div class="cookie-bar">
              <span class="cookie-status" :class="cookieStatus ? 'active' : 'inactive'">
                <span class="cookie-dot" :class="cookieStatus ? 'active' : 'inactive'"></span>
                {{ cookieStatus ? 'Cookies do YouTube ativados' : 'Sem cookies do YouTube' }}
              </span>
              <button @click="showCookieModal = !showCookieModal" class="cookie-toggle-btn">
                <Key class="icon-xs text-amber" />
                {{ showCookieModal ? 'Fechar' : 'Configurar cookies.txt' }}
              </button>
            </div>

            <!-- Form/Card de Importação de Cookies -->
            <div v-if="showCookieModal" class="cookie-card">
              <div class="cookie-card-header">
                <h4 class="cookie-card-title">
                  <Key class="icon-sm text-amber" />
                  Importar cookies.txt do YouTube
                </h4>
              </div>
              <p class="cookie-card-desc">
                Selecione o arquivo <code class="code-amber">cookies.txt</code> exportado do seu navegador ou cole o conteúdo abaixo:
              </p>

              <div class="cookie-inputs">
                <input
                  type="file"
                  ref="cookieFileRef"
                  accept=".txt"
                  @change="onCookieFileSelected"
                  class="file-input"
                />
                <textarea
                  v-model="cookieContent"
                  rows="3"
                  placeholder="Cole o conteúdo do cookies.txt aqui..."
                  class="textarea"
                ></textarea>
              </div>

              <div class="cookie-card-footer">
                <span
                  v-if="cookieMsg"
                  class="cookie-msg"
                  :class="cookieMsg.includes('sucesso') ? 'text-emerald' : 'text-red'"
                >
                  {{ cookieMsg }}
                </span>
                <div v-else></div>
                <button
                  @click="handleSaveCookies"
                  :disabled="savingCookies || !cookieContent.trim()"
                  class="btn btn-amber btn-sm"
                >
                  {{ savingCookies ? 'Salvando...' : 'Salvar Cookies' }}
                </button>
              </div>
            </div>

            <p v-if="ytError" class="error-text">{{ ytError }}</p>
          </div>
        </div>
      </div>

      <div class="dj-three-col">
        <!-- Coluna 1: Repositório / Biblioteca Global / Sugestões -->
        <div class="card left-panel">
          <!-- Navegação de Abas -->
          <div class="tabs-bar dj-tabs">
            <button
              @click="activeTab = 'repo'"
              class="tab-btn"
              :class="{ active: activeTab === 'repo' }"
            >
              <Folder class="icon-sm" />
              Repositório Local
            </button>
            <button
              @click="switchTab('global')"
              class="tab-btn"
              :class="{ active: activeTab === 'global' }"
            >
              <Library class="icon-sm" />
              Biblioteca Global
            </button>
            <button
              @click="activeTab = 'suggestions'"
              class="tab-btn"
              :class="{ 'active-amber': activeTab === 'suggestions' }"
            >
              <MessageSquare class="icon-sm" />
              Sugestões
              <span
                v-if="store.suggestions && store.suggestions.length > 0"
                class="suggestions-count-badge"
              >
                {{ store.suggestions.length }}
              </span>
            </button>
          </div>

          <!-- Conteúdo Aba Repositório Local -->
          <div v-if="activeTab === 'repo'" class="tab-content">
            <!-- Lista e Filtros de Músicas do Repositório Local -->
            <div class="repo-list-section">
              <div class="repo-list-header">
                <p class="subsection-title-icon">
                  <Filter class="icon-xs text-emerald" />
                  Músicas desta Estação
                </p>
              </div>

              <!-- Barra de Pesquisa e Filtros -->
              <div class="filter-controls">
                <div class="search-wrapper">
                  <Search class="icon-sm search-icon" />
                  <input
                    v-model="repoSearchQuery"
                    placeholder="Buscar no repositório local..."
                    class="search-input"
                  />
                </div>
                <div class="two-col-grid">
                  <select v-model="repoCategoryFilter" class="select select-sm">
                    <option value="">Todas Categorias</option>
                    <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
                  </select>
                  <select v-model="repoThemeFilter" class="select select-sm">
                    <option value="">Todos Temas</option>
                    <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
                  </select>
                </div>
              </div>

              <div v-if="filteredRepoTracks.length === 0" class="empty-state">
                Nenhuma música encontrada no repositório local
              </div>
              <div v-else class="tracks-grid">
                <div
                  v-for="track in filteredRepoTracks"
                  :key="track.id"
                  class="track-cell"
                >
                  <div class="track-cell-cover" @click="addToPlaylist(track.id)" title="Clique para adicionar à playlist">
                    <img v-if="track.coverUrl" :src="track.coverUrl" class="track-cell-img" alt="Capa" />
                    <div v-else class="track-cell-placeholder">
                      <Music class="icon-lg text-emerald" />
                    </div>
                    <div class="track-cell-overlay">
                      <Plus class="icon-md" />
                    </div>
                  </div>
                  <div class="track-cell-details">
                    <div class="track-cell-header">
                      <span class="track-cell-title truncate" :title="track.title">{{ track.title }}</span>
                      <button
                        @click.stop="openEditModal(track)"
                        class="edit-icon-btn"
                        title="Editar Título, Categoria, Tema e Capa"
                      >
                        <Edit2 class="icon-xs" />
                      </button>
                    </div>
                    <div class="track-cell-tags">
                      <span v-if="track.category" class="tag-category">{{ track.category }}</span>
                      <span v-if="track.theme" class="tag-theme">{{ track.theme }}</span>
                    </div>
                    <div class="track-cell-footer">
                      <span v-if="track.duration" class="track-duration">{{ formatTime(track.duration) }}</span>
                      <button @click="addToPlaylist(track.id)" class="track-cell-add-btn" title="Adicionar à playlist">
                        <Plus class="icon-xs" /> Fila
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Conteúdo Aba Biblioteca Global -->
          <div v-else-if="activeTab === 'global'" class="tab-content">
            <div class="global-search-row">
              <div class="search-wrapper">
                <Search class="icon-sm search-icon" />
                <input
                  v-model="searchQuery"
                  placeholder="Buscar na biblioteca..."
                  class="search-input"
                />
              </div>
              <button
                @click="fetchGlobalLibrary"
                :disabled="loadingGlobal"
                class="btn-icon refresh-btn"
                title="Atualizar biblioteca"
              >
                <RefreshCw class="icon-sm" :class="{ 'animate-spin': loadingGlobal }" />
              </button>
            </div>

            <!-- Filtros por Categoria e Tema para Biblioteca Global -->
            <div class="two-col-grid">
              <select v-model="globalCategoryFilter" class="select select-sm">
                <option value="">Todas Categorias</option>
                <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
              </select>
              <select v-model="globalThemeFilter" class="select select-sm">
                <option value="">Todos Temas</option>
                <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
              </select>
            </div>

            <div v-if="loadingGlobal" class="empty-state">
              Carregando biblioteca...
            </div>
            <div v-else-if="filteredGlobalTracks.length === 0" class="empty-state">
              Nenhuma música encontrada na biblioteca
            </div>
            <div v-else class="tracks-grid">
              <div
                v-for="track in filteredGlobalTracks"
                :key="track.id"
                class="track-cell"
              >
                <div class="track-cell-cover" @click="addGlobalTrack(track)" title="Clique para adicionar à playlist">
                  <img v-if="track.coverUrl" :src="track.coverUrl" class="track-cell-img" alt="Capa" />
                  <div v-else class="track-cell-placeholder">
                    <Music class="icon-lg text-emerald" />
                  </div>
                  <div class="track-cell-overlay">
                    <Plus class="icon-md" />
                  </div>
                </div>
                <div class="track-cell-details">
                  <div class="track-cell-header">
                    <span class="track-cell-title truncate" :title="track.title">{{ track.title }}</span>
                    <button
                      @click.stop="openEditModal(track)"
                      class="edit-icon-btn"
                      title="Editar Título, Categoria, Tema e Capa"
                    >
                      <Edit2 class="icon-xs" />
                    </button>
                  </div>
                  <div class="track-cell-tags">
                    <span v-if="track.category" class="tag-category">{{ track.category }}</span>
                    <span v-if="track.theme" class="tag-theme">{{ track.theme }}</span>
                  </div>
                  <div class="track-cell-footer">
                    <span v-if="track.duration" class="track-duration">{{ formatTime(track.duration) }}</span>
                    <button @click="addGlobalTrack(track)" class="track-cell-add-btn" title="Adicionar à playlist">
                      <Plus class="icon-xs" /> Fila
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Conteúdo Aba Sugestões de Ouvintes -->
          <div v-else-if="activeTab === 'suggestions'" class="tab-content">
            <div class="suggestions-top-bar">
              <p class="suggestions-heading">
                <MessageSquare class="icon-md" />
                Sugestões Enviadas por Ouvintes ({{ store.suggestions ? store.suggestions.length : 0 }})
              </p>
              <button
                v-if="store.suggestions && store.suggestions.length > 0"
                @click="clearSuggestions"
                class="clear-sug-btn"
              >
                Limpar todas
              </button>
            </div>

            <div v-if="!store.suggestions || store.suggestions.length === 0" class="empty-state">
              Nenhuma sugestão enviada pelos ouvintes no momento.
            </div>
            <div v-else class="suggestions-scroll-list">
              <div
                v-for="sug in store.suggestions"
                :key="sug.id"
                class="suggestion-card"
              >
                <div class="suggestion-card-body">
                  <h4 class="suggestion-title truncate">{{ sug.title }}</h4>
                  <p class="suggestion-by">
                    Sugerido por: <strong class="text-emerald">{{ sug.suggestedBy }}</strong>
                  </p>
                  <p v-if="sug.url" class="suggestion-url truncate">
                    URL: {{ sug.url }}
                  </p>
                </div>

                <div class="suggestion-actions">
                  <button
                    @click="rejectSuggestion(sug.id)"
                    class="btn btn-danger-outline btn-sm"
                  >
                    <X class="icon-xs" />
                    Rejeitar
                  </button>
                  <button
                    @click="approveSuggestion(sug)"
                    class="btn btn-primary btn-sm"
                  >
                    <Check class="icon-xs" />
                    Aprovar & Tocar
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Coluna 2: Mixer de Sons de Fundo e Efeitos -->
        <AudioMixer
          :is-d-j="true"
          :available-tracks="allAvailableTracks"
          @update-mixer="handleUpdateMixer"
        />

        <!-- Coluna 3: Playlist Fila de Reprodução -->
        <div class="card playlist-panel">
          <h3 class="playlist-panel-title">Playlist Fila de Reprodução</h3>
          <div v-if="store.playlist.length === 0" class="empty-state">
            Playlist vazia — adicione músicas do repositório ou da biblioteca global
          </div>
          <div
            v-for="(track, i) in store.playlist"
            :key="track.id"
            draggable="true"
            @dragstart="onDragStart($event, i)"
            @dragover.prevent="onDragOver($event, i)"
            @dragleave="onDragLeave($event, i)"
            @drop="onDrop($event, i)"
            @dragend="onDragEnd"
            @click="playPlaylistTrack(track.id)"
            class="dj-playlist-item"
            :class="{
              'is-playing': i === 0 && store.state.isPlaying,
              'is-dragging': draggedIndex === i,
              'is-drag-over': dragOverIndex === i && draggedIndex !== i
            }"
          >
            <div class="drag-handle" title="Segure e arraste para reordenar">
              <GripVertical class="icon-xs text-muted" />
            </div>

            <button
              @click.stop="removeFromPlaylist(track.id)"
              class="remove-playlist-btn"
              title="Remover da playlist"
            >
              <X class="icon-sm" />
            </button>

            <div class="playlist-num-col">
              <Play v-if="i === 0 && store.state.isPlaying" class="icon-sm text-emerald animate-pulse" />
              <span v-else class="playlist-num">{{ i + 1 }}</span>
            </div>

            <div class="playlist-thumb-box">
              <img v-if="track.coverUrl" :src="track.coverUrl" class="playlist-thumb-img" alt="Capa" />
              <div v-else class="playlist-thumb-placeholder">
                <Music class="icon-xs text-muted" />
              </div>
            </div>

            <div class="playlist-item-info">
              <div class="playlist-item-title truncate" :class="{ first: i === 0 }">
                {{ track.title }}
              </div>
              <div v-if="track.category || track.theme" class="playlist-item-tags">
                <span v-if="track.category" class="tag-category">{{ track.category }}</span>
                <span v-if="track.theme" class="tag-theme">{{ track.theme }}</span>
              </div>
            </div>

            <span v-if="track.duration" class="track-duration">{{ formatTime(track.duration) }}</span>

            <button
              @click.stop="playPlaylistTrack(track.id)"
              class="play-now-btn"
              title="Tocar esta música agora"
            >
              <Play class="icon-sm" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Modal de Edição de Metadados (Título, Categoria e Tema) -->
    <div v-if="showEditModal" class="modal-overlay">
      <div class="modal-box">
        <div class="modal-header">
          <h3 class="modal-title">
            <Edit2 class="icon-md text-emerald" />
            Editar Mídia
          </h3>
          <button @click="showEditModal = false" class="btn-icon">
            <X class="icon-md" />
          </button>
        </div>

        <div class="modal-form">
          <div>
            <label class="field-label">Título da Música</label>
            <input v-model="editTitle" type="text" class="input" />
          </div>

          <div>
            <label class="field-label">Imagem da Capa</label>
            <div class="cover-input-container">
              <div class="cover-input-row">
                <input
                  v-model="editCoverUrl"
                  placeholder="URL da imagem (http://... ou selecione um arquivo)"
                  type="text"
                  class="input"
                />
                <button
                  v-if="editCoverUrl"
                  @click="editCoverUrl = ''"
                  class="btn btn-secondary btn-sm"
                  type="button"
                >
                  Limpar
                </button>
              </div>
              <div class="cover-upload-row">
                <label class="file-pick-label">
                  <ImageIcon class="icon-xs" /> Selecionar imagem do computador
                  <input
                    ref="coverFileInputRef"
                    type="file"
                    accept="image/*"
                    class="sr-only"
                    @change="onCoverFileSelected"
                  />
                </label>
              </div>
              <div v-if="editCoverUrl" class="cover-preview-wrapper">
                <img :src="editCoverUrl" alt="Preview da capa" class="cover-preview-img" />
                <span class="cover-preview-hint">Pré-visualização da capa</span>
              </div>
            </div>
          </div>

          <div>
            <label class="field-label">Categoria</label>
            <div class="two-col-grid">
              <select v-model="editCategory" class="input">
                <option value="">-- Nenhuma Categoria --</option>
                <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
              </select>
              <input v-model="editCategory" placeholder="ou digite customizada..." type="text" class="input" />
            </div>
          </div>

          <div>
            <label class="field-label">Tema</label>
            <div class="two-col-grid">
              <select v-model="editTheme" class="input">
                <option value="">-- Nenhum Tema --</option>
                <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
              </select>
              <input v-model="editTheme" placeholder="ou digite customizado..." type="text" class="input" />
            </div>
          </div>
        </div>

        <div class="modal-footer">
          <button @click="showEditModal = false" class="btn btn-secondary">
            Cancelar
          </button>
          <button @click="handleSaveEdit" :disabled="savingEdit" class="btn btn-primary">
            <Check class="icon-sm" />
            {{ savingEdit ? 'Salvando...' : 'Salvar Alterações' }}
          </button>
        </div>
      </div>
    </div>

    <!-- Barra Fixa do Player na Parte Inferior (Estilo YouTube Music) -->
    <Player @toggle-play="handleTogglePlay" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, X, SkipForward, Copy, Folder, Library, Search, RefreshCw, Youtube, Key, Play, Disc, MessageSquare, Check, Edit2, Tag, Filter, Music, Image as ImageIcon, GripVertical } from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { uploadMusic, getRepository, getGlobalLibrary, downloadFromYouTube, saveYouTubeCookies, getCookiesStatus, getStation, updateTrackMetadata } from '../services/api'
import { useWebSocket } from '../composables/useWebSocket'
import { getSavedStations, saveStation, removeStation } from '../services/storage'
import Player from '../components/Player.vue'
import AudioMixer from '../components/AudioMixer.vue'

const route = useRoute()
const router = useRouter()
const store = useStationStore()
const { connect, send, disconnect } = useWebSocket(null)

const activeTab = ref('repo')
const fileInputRef = ref(null)
const selectedFile = ref(null)
const uploadCategory = ref('')
const uploadTheme = ref('')
const uploading = ref(false)
const uploadError = ref('')

const youtubeUrl = ref('')
const ytCategory = ref('')
const ytTheme = ref('')
const downloadingYT = ref(false)
const ytError = ref('')

const showCookieModal = ref(false)
const cookieContent = ref('')
const cookieFileRef = ref(null)
const savingCookies = ref(false)
const cookieMsg = ref('')
const cookieStatus = ref(false)

const globalTracks = ref([])
const loadingGlobal = ref(false)
const searchQuery = ref('')

const repoSearchQuery = ref('')
const repoCategoryFilter = ref('')
const repoThemeFilter = ref('')

const globalCategoryFilter = ref('')
const globalThemeFilter = ref('')

const showEditModal = ref(false)
const editingTrack = ref(null)
const editTitle = ref('')
const editCategory = ref('')
const editTheme = ref('')
const editCoverUrl = ref('')
const coverFileInputRef = ref(null)
const savingEdit = ref(false)

function onCoverFileSelected(e) {
  const file = e.target.files[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = (evt) => {
    editCoverUrl.value = evt.target.result
  }
  reader.readAsDataURL(file)
}

const categoriesList = [
  'Rock',
  'Nu-Metal',
  'Maidcore',
  'Pop',
  'K-Pop',
  'Jazz',
  'Blues',
  'Sertanejo',
  'MPB',
  'Eletrônica',
  'Gospel',
  'Funk',
  'Hip-Hop',
  'Reggae',
  'Efeitos / Trilhas',
  'Outros',
]
const themesList = ['Terror', 'Combate', 'Investigação', 'Calma', 'Tensão', 'Dramatica']

const filteredRepoTracks = computed(() => {
  return (store.repository || []).filter(t => {
    const matchQuery = !repoSearchQuery.value.trim() || (t.title && t.title.toLowerCase().includes(repoSearchQuery.value.toLowerCase()))
    const matchCat = !repoCategoryFilter.value || t.category === repoCategoryFilter.value
    const matchTheme = !repoThemeFilter.value || t.theme === repoThemeFilter.value
    return matchQuery && matchCat && matchTheme
  })
})

const filteredGlobalTracks = computed(() => {
  return (globalTracks.value || []).filter(t => {
    const q = searchQuery.value.trim().toLowerCase()
    const matchQuery = !q || (t.title && t.title.toLowerCase().includes(q))
    const matchCat = !globalCategoryFilter.value || t.category === globalCategoryFilter.value
    const matchTheme = !globalThemeFilter.value || t.theme === globalThemeFilter.value
    return matchQuery && matchCat && matchTheme
  })
})

onMounted(async () => {
  const id = route.params.stationId
  if (id) {
    store.stationId = id
    store.role = 'dj'
    if (!store.djToken) {
      const saved = getSavedStations().find(s => s.id === id)
      if (saved) store.djToken = saved.token
    }
    saveStation(id, 'dj', store.djToken)
    connect()
    checkCookiesStatus()
    try {
      const data = await getStation(id)
      if (data.state) store.setState(data.state)
      if (data.playlist) store.setPlaylist(data.playlist)
      if (data.repository) store.setRepository(data.repository)
      if (data.suggestions) store.setSuggestions(data.suggestions)
      if (data.mixer) store.setMixer(data.mixer)
    } catch (e) {
      removeStation(id)
      store.reset()
      router.push('/')
    }
  }
})

async function checkCookiesStatus() {
  try {
    const res = await getCookiesStatus(store.stationId)
    cookieStatus.value = res.hasCookies
  } catch {}
}

function onCookieFileSelected(e) {
  const file = e.target.files[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = (evt) => {
    cookieContent.value = evt.target.result
  }
  reader.readAsText(file)
}

async function handleSaveCookies() {
  if (!cookieContent.value.trim()) return
  savingCookies.value = true
  cookieMsg.value = ''
  try {
    const res = await saveYouTubeCookies(store.stationId, cookieContent.value, store.djToken)
    cookieMsg.value = res.message || 'Cookies salvos com sucesso!'
    cookieStatus.value = true
    cookieContent.value = ''
    if (cookieFileRef.value) cookieFileRef.value.value = ''
    setTimeout(() => {
      showCookieModal.value = false
      cookieMsg.value = ''
    }, 1500)
  } catch (e) {
    cookieMsg.value = e?.response?.data || e.message || 'Erro ao salvar cookies'
  } finally {
    savingCookies.value = false
  }
}

async function fetchGlobalLibrary() {
  loadingGlobal.value = true
  try {
    const tracks = await getGlobalLibrary()
    globalTracks.value = tracks
  } catch (e) {
    console.error('Erro ao buscar biblioteca global:', e)
  } finally {
    loadingGlobal.value = false
  }
}

function switchTab(tab) {
  activeTab.value = tab
  if (tab === 'global' && globalTracks.value.length === 0) {
    fetchGlobalLibrary()
  }
}

function selectFile(e) {
  selectedFile.value = e.target.files[0] || null
}

async function handleUpload() {
  if (!selectedFile.value) return
  uploading.value = true
  uploadError.value = ''
  try {
    const track = await uploadMusic(store.stationId, selectedFile.value, store.djToken, uploadCategory.value, uploadTheme.value)
    store.repository.push(track)
    selectedFile.value = null
    uploadCategory.value = ''
    uploadTheme.value = ''
    if (fileInputRef.value) fileInputRef.value.value = ''
    if (globalTracks.value.length > 0) fetchGlobalLibrary()
  } catch (e) {
    const msg = e?.response?.data || e.message || 'Erro desconhecido'
    uploadError.value = typeof msg === 'string' ? msg : 'Erro no upload'
  } finally {
    uploading.value = false
  }
}

async function handleYouTubeDownload() {
  const url = youtubeUrl.value.trim()
  if (!url) return
  downloadingYT.value = true
  ytError.value = ''

  try {
    const track = await downloadFromYouTube(store.stationId, url, store.djToken, ytCategory.value, ytTheme.value)
    store.repository.push(track)
    youtubeUrl.value = ''
    ytCategory.value = ''
    ytTheme.value = ''
    if (globalTracks.value.length > 0) fetchGlobalLibrary()
  } catch (e) {
    const msg = e?.response?.data || e.message || 'Erro no download do YouTube'
    ytError.value = typeof msg === 'string' ? msg : 'Erro ao baixar do YouTube'
  } finally {
    downloadingYT.value = false
  }
}

function openEditModal(track) {
  editingTrack.value = track
  editTitle.value = track.title || ''
  editCategory.value = track.category || ''
  editTheme.value = track.theme || ''
  editCoverUrl.value = track.coverUrl || ''
  showEditModal.value = true
}

async function handleSaveEdit() {
  if (!editingTrack.value) return
  savingEdit.value = true
  try {
    const updated = await updateTrackMetadata(
      store.stationId,
      editingTrack.value.id,
      {
        title: editTitle.value,
        category: editCategory.value,
        theme: editTheme.value,
        coverUrl: editCoverUrl.value
      },
      store.djToken
    )
    const index = store.repository.findIndex(t => t.id === editingTrack.value.id)
    if (index !== -1) {
      store.repository[index] = { ...store.repository[index], ...updated }
    }
    const pIndex = store.playlist.findIndex(t => t.id === editingTrack.value.id)
    if (pIndex !== -1) {
      store.playlist[pIndex] = { ...store.playlist[pIndex], ...updated }
    }
    const gIndex = globalTracks.value.findIndex(t => t.id === editingTrack.value.id)
    if (gIndex !== -1) {
      globalTracks.value[gIndex] = { ...globalTracks.value[gIndex], ...updated }
    }
    showEditModal.value = false
  } catch (e) {
    alert(e?.response?.data || e.message || 'Erro ao salvar alterações')
  } finally {
    savingEdit.value = false
  }
}

function addToPlaylist(trackId) {
  send({ type: 'ADD_TO_PLAYLIST', data: { trackId } })
}

function addGlobalTrack(track) {
  send({ type: 'ADD_TO_PLAYLIST', data: { trackId: track.id, track } })
}

function playPlaylistTrack(trackId) {
  send({ type: 'PLAY_PLAYLIST_TRACK', data: { trackId } })
}

function removeFromPlaylist(trackId) {
  send({ type: 'REMOVE_FROM_PLAYLIST', data: { trackId } })
}

const draggedIndex = ref(null)
const dragOverIndex = ref(null)

function onDragStart(e, index) {
  if (e.target.closest('button')) {
    e.preventDefault()
    return
  }
  draggedIndex.value = index
  e.dataTransfer.effectAllowed = 'move'
  e.dataTransfer.setData('text/plain', index.toString())
}

function onDragOver(e, index) {
  e.preventDefault()
  e.dataTransfer.dropEffect = 'move'
  if (dragOverIndex.value !== index) {
    dragOverIndex.value = index
  }
}

function onDragLeave(e, index) {
  if (dragOverIndex.value === index) {
    dragOverIndex.value = null
  }
}

function onDrop(e, targetIndex) {
  e.preventDefault()
  const from = draggedIndex.value
  const to = targetIndex
  dragOverIndex.value = null
  draggedIndex.value = null

  if (from === null || from === undefined || from === to) return

  // Atualização otimista local imediata
  const updatedPlaylist = [...store.playlist]
  const [movedTrack] = updatedPlaylist.splice(from, 1)
  updatedPlaylist.splice(to, 0, movedTrack)
  store.setPlaylist(updatedPlaylist)

  // Envia ao servidor WebSocket para sincronizar todas as sessões
  send({
    type: 'REORDER_PLAYLIST',
    data: { fromIndex: from, toIndex: to }
  })
}

function onDragEnd() {
  draggedIndex.value = null
  dragOverIndex.value = null
}

function approveSuggestion(sug) {
  send({ type: 'APPROVE_SUGGESTION', data: { suggestionId: sug.id } })
}

function rejectSuggestion(suggestionId) {
  send({ type: 'REJECT_SUGGESTION', data: { suggestionId } })
}

function clearSuggestions() {
  send({ type: 'CLEAR_SUGGESTIONS' })
}

function skipToNext() {
  send({ type: 'NEXT_TRACK' })
}

function handleTogglePlay(action, value) {
  if (action === 'SEEK') {
    send({ type: 'SEEK', data: { position: value } })
  } else if (action === 'NEXT_TRACK') {
    send({ type: 'NEXT_TRACK' })
  } else {
    send({ type: action })
  }
}

function formatTime(seconds) {
  if (!seconds || isNaN(seconds)) return ''
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

function copyCode() {
  navigator.clipboard.writeText(store.stationId)
}

function switchToListener() {
  disconnect()
  store.role = 'listener'
  store.djToken = null
  saveStation(store.stationId, 'listener', null)
}

const allAvailableTracks = computed(() => {
  const map = new Map()
  ;(store.repository || []).forEach(t => {
    if (t && t.id) map.set(t.id, t)
  })
  ;(globalTracks.value || []).forEach(t => {
    if (t && t.id && !map.has(t.id)) map.set(t.id, t)
  })
  return Array.from(map.values())
})

function handleUpdateMixer(channels) {
  send({ type: 'MIXER_UPDATE', data: { channels } })
}

function leave() {
  disconnect()
  removeStation(store.stationId)
  store.reset()
  router.push('/')
}
</script>

<style scoped>
.brand-name {
  font-weight: 700;
  color: var(--emerald-500);
}

.amber-hover:hover {
  color: var(--amber-400);
}

.copy-btn:hover {
  color: var(--emerald-400);
}

.station-actions-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1rem;
  align-items: stretch;
  text-align: left;
}

@media (min-width: 768px) and (max-width: 1099px) {
  .station-actions-grid {
    grid-template-columns: repeat(2, 1fr);
  }
  .station-actions-grid .station-code-block {
    grid-column: span 2;
  }
}

@media (min-width: 1100px) {
  .station-actions-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

.station-code-block {
  display: flex;
  flex-direction:row !important;
  justify-content: center;
  align-items: center;
  text-align: center;
  padding: 0.2rem 0.2rem;
  min-height: 100%;
}

.station-code-sub {
  font-size: 0.6875rem;
  color: var(--zinc-500);
  margin-top: 0.25rem;
}

.station-action-block {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.station-actions-grid .yt-section {
  border-top: none;
  padding-top: 0;
}

.dj-main {
  flex: 1;
  width: 100%;
  margin: 0 auto;
  padding: 1rem 1rem 6rem;
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.dj-three-col {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1rem;
  align-items: start;
}

@media (min-width: 768px) and (max-width: 1199px) {
  .dj-three-col {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1200px) {
  .dj-three-col {
    grid-template-columns: repeat(3, 1fr);
  }
}

.left-panel {
  display: flex;
  flex-direction: column;
}

.dj-tabs {
  margin-bottom: 1rem;
}

.suggestions-count-badge {
  margin-left: 0.25rem;
  background-color: var(--amber-400);
  color: var(--zinc-950);
  font-weight: 700;
  font-size: 0.625rem;
  padding: 0.0625rem 0.375rem;
  border-radius: var(--radius-full);
}

.tab-content {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.subsection-title {
  font-size: 1rem;
  font-weight: 500;
  color: var(--zinc-400);
}

.subsection-title-icon {
  font-size: 1rem;
  font-weight: 500;
  color: var(--zinc-400);
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.upload-section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.file-meta-box {
  background-color: rgba(24, 24, 27, 0.6);
  padding: 0.5rem;
  border-radius: var(--radius-lg);
  border: 1px solid rgba(63, 63, 70, 0.6);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.selected-filename {
  display: block;
  font-size: 0.75rem;
  color: var(--zinc-200);
  font-weight: 500;
}

.two-col-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.5rem;
}

.flex-end {
  display: flex;
  justify-content: flex-end;
}

.error-text {
  font-size: 0.75rem;
  color: var(--red-400);
}

.yt-section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(63, 63, 70, 0.5);
}

.yt-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.yt-input-row {
  display: flex;
  gap: 0.5rem;
}

.yt-input {
  flex: 1;
  padding: 0.375rem 0.75rem;
}

.yt-input:focus {
  border-color: var(--red-500);
  box-shadow: 0 0 0 1px var(--red-500);
}

.cookie-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 0.75rem;
  padding-top: 0.25rem;
}

.cookie-status {
  display: flex;
  align-items: center;
  gap: 0.2rem;
  font-size: 0.8rem;
}

.cookie-status.active {
  color: var(--emerald-400);
}

.cookie-status.inactive {
  color: var(--zinc-500);
}

.cookie-dot {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: var(--radius-full);
}

.cookie-dot.active {
  background-color: var(--emerald-500);
}

.cookie-dot.inactive {
  background-color: var(--zinc-600);
}

.cookie-toggle-btn {
  font-size: 0.6875rem;
  color: var(--zinc-400);
  text-decoration: underline;
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.cookie-toggle-btn:hover {
  color: var(--zinc-200);
}

.cookie-card {
  padding: 0.75rem;
  background-color: rgba(24, 24, 27, 0.9);
  border: 1px solid rgba(63, 63, 70, 0.8);
  border-radius: var(--radius-lg);
  display: flex;
  flex-direction: column;
  gap: 0.625rem;
  margin-top: 0.25rem;
}

.cookie-card-title {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--zinc-200);
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.cookie-card-desc {
  font-size: 0.6875rem;
  color: var(--zinc-400);
  line-height: 1.3;
}

.code-amber {
  color: var(--amber-300);
  font-family: var(--font-mono);
}

.cookie-inputs {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.cookie-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 0.25rem;
}

.cookie-msg {
  font-size: 0.6875rem;
}

.repo-list-section {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid rgba(63, 63, 70, 0.5);
}

.filter-controls {
  display: flex;
  flex-direction: column;
  gap: 0.375rem;
}

.select-sm {
  font-size: 0.8rem;
  padding: 0.5rem 0.5rem;
}

/* Music Grid Layout */
.tracks-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: 0.5rem;
  max-height: 24rem;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.track-cell {
  background-color: rgba(24, 24, 27, 0.7);
  border: 1px solid rgba(63, 63, 70, 0.5);
  border-radius: var(--radius-lg);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  transition: transform 0.15s ease, border-color 0.15s ease, background-color 0.15s ease, box-shadow 0.15s ease;
}

.track-cell:hover {
  transform: translateY(-2px);
  border-color: rgba(16, 185, 129, 0.4);
  background-color: rgba(39, 39, 42, 0.85);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.35);
}

.track-cell-cover {
  position: relative;
  width: 100%;
  aspect-ratio: 1 / 1;
  background-color: rgba(9, 9, 11, 0.8);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  overflow: hidden;
}

.track-cell-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.2s ease;
}

.track-cell:hover .track-cell-img {
  transform: scale(1.05);
}

.track-cell-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, rgba(39, 39, 42, 0.6), rgba(24, 24, 27, 0.9));
}

.track-cell-overlay {
  position: absolute;
  inset: 0;
  background-color: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transition: opacity 0.15s ease;
  color: #ffffff;
}

.track-cell-cover:hover .track-cell-overlay {
  opacity: 1;
}

.track-cell-details {
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  flex: 1;
}

.track-cell-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.25rem;
}

.track-cell-title {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--zinc-200);
}

.track-cell-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
}

.track-cell-footer {
  margin-top: auto;
  padding-top: 0.375rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-top: 1px solid rgba(63, 63, 70, 0.3);
}

.track-cell-add-btn {
  font-size: 0.6875rem;
  font-weight: 500;
  color: var(--emerald-400);
  background-color: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.25);
  border-radius: var(--radius-sm);
  padding: 0.125rem 0.375rem;
  display: inline-flex;
  align-items: center;
  gap: 0.125rem;
  cursor: pointer;
  transition: background-color 0.15s ease, color 0.15s ease;
}

.track-cell-add-btn:hover {
  background-color: var(--emerald-600);
  color: #ffffff;
}

.tracks-scroll-list {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  max-height: 16rem;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.track-row {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.375rem 0.5rem;
  border-radius: var(--radius-md);
  cursor: pointer;
  font-size: 0.75rem;
  transition: background-color 0.15s ease;
}

.track-row:hover {
  background-color: rgba(63, 63, 70, 0.8);
}

.track-row-main {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  flex: 1;
  min-width: 0;
}

.track-plus {
  transition: transform 0.15s ease;
}

.track-row:hover .track-plus {
  transform: scale(1.15);
}

.track-row-title {
  font-weight: 500;
  color: var(--zinc-200);
}

.track-row-meta {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  flex-shrink: 0;
}

.track-duration {
  font-size: 0.625rem;
  color: var(--zinc-500);
  margin-left: 0.25rem;
}

.edit-icon-btn {
  padding: 0.25rem;
  color: var(--zinc-400);
  display: flex;
  align-items: center;
  transition: color 0.15s ease;
}

.edit-icon-btn:hover {
  color: var(--amber-400);
}

.global-search-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.refresh-btn {
  background-color: var(--zinc-700);
}

.helper-hint {
  font-size: 0.75rem;
  color: var(--zinc-500);
}

.suggestions-top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid rgba(63, 63, 70, 0.6);
  padding-bottom: 0.5rem;
}

.suggestions-heading {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--amber-400);
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.clear-sug-btn {
  font-size: 0.6875rem;
  color: var(--zinc-400);
  transition: color 0.15s ease;
}

.clear-sug-btn:hover {
  color: var(--red-400);
}

.suggestions-scroll-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-height: 20rem;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.suggestion-card {
  padding: 0.75rem;
  background-color: rgba(24, 24, 27, 0.8);
  border: 1px solid rgba(63, 63, 70, 0.7);
  border-radius: var(--radius-lg);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.suggestion-card-body {
  min-width: 0;
}

.suggestion-title {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--zinc-200);
}

.suggestion-by {
  font-size: 0.625rem;
  color: var(--zinc-400);
}

.suggestion-url {
  font-size: 0.625rem;
  color: var(--zinc-500);
  font-family: var(--font-mono);
  margin-top: 0.125rem;
}

.suggestion-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem;
  padding-top: 0.25rem;
  border-top: 1px solid var(--zinc-800);
}

/* Playlist Panel */
.playlist-panel-title {
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--zinc-400);
}

.dj-playlist-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem;
  border-bottom: 1px solid rgba(63, 63, 70, 0.5);
  border-radius: var(--radius-lg);
  cursor: grab;
  transition: background-color 0.15s ease, border-color 0.15s ease, transform 0.15s ease, opacity 0.15s ease;
  user-select: none;
}

.dj-playlist-item:active {
  cursor: grabbing;
}

.dj-playlist-item.is-dragging {
  opacity: 0.35;
  background-color: rgba(63, 63, 70, 0.4);
  border: 1px dashed var(--emerald-400);
  transform: scale(0.98);
}

.dj-playlist-item.is-drag-over {
  border-top: 2px solid var(--emerald-400);
  background-color: rgba(16, 185, 129, 0.12);
  transform: translateY(2px);
}

.drag-handle {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--zinc-500);
  cursor: grab;
  padding: 0.125rem;
  border-radius: var(--radius-sm);
  transition: color 0.15s ease;
  flex-shrink: 0;
}

.drag-handle:active {
  cursor: grabbing;
}

.dj-playlist-item:hover .drag-handle {
  color: var(--zinc-300);
}

.dj-playlist-item:last-child {
  border-bottom: 0;
}

.dj-playlist-item:hover {
  background-color: rgba(63, 63, 70, 0.6);
}

.dj-playlist-item.is-playing {
  background-color: rgba(2, 44, 34, 0.4);
  border-color: rgba(6, 95, 70, 0.4);
}

.remove-playlist-btn {
  color: var(--red-400);
  padding: 0.125rem;
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.remove-playlist-btn:hover {
  color: var(--red-300);
}

.playlist-num-col {
  width: 1rem;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.playlist-num {
  font-size: 0.75rem;
  color: var(--zinc-500);
  font-family: var(--font-mono);
}

.dj-playlist-item:hover .playlist-num {
  color: var(--zinc-300);
}

.playlist-item-info {
  flex: 1;
  min-width: 0;
}

.playlist-item-title {
  font-size: 0.875rem;
  color: var(--zinc-200);
}

.playlist-item-title.first {
  color: var(--emerald-400);
  font-weight: 600;
}

.dj-playlist-item:hover .playlist-item-title:not(.first) {
  color: #ffffff;
}

.playlist-item-tags {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  margin-top: 0.125rem;
}

.play-now-btn {
  padding: 0.25rem;
  border-radius: var(--radius-sm);
  color: var(--zinc-400);
  display: flex;
  align-items: center;
  flex-shrink: 0;
  transition: color 0.15s ease, background-color 0.15s ease;
}

.dj-playlist-item:hover .play-now-btn {
  color: var(--emerald-400);
}

.play-now-btn:hover {
  background-color: rgba(82, 82, 91, 0.5);
}

/* Modal Form */
.modal-form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.field-label {
  display: block;
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--zinc-400);
  margin-bottom: 0.25rem;
}

/* Playlist Cover Icon */
.playlist-thumb-box {
  width: 2.25rem;
  height: 2.25rem;
  border-radius: var(--radius-md);
  overflow: hidden;
  background-color: var(--zinc-800);
  border: 1px solid var(--zinc-700);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.playlist-thumb-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.playlist-thumb-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
}

/* Modal Cover Input & Preview */
.cover-input-container {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.cover-input-row {
  display: flex;
  gap: 0.5rem;
}

.cover-upload-row {
  display: flex;
  align-items: center;
}

.file-pick-label {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  font-size: 0.75rem;
  color: var(--zinc-300);
  background-color: rgba(63, 63, 70, 0.6);
  border: 1px dashed var(--zinc-600);
  border-radius: var(--radius-md);
  padding: 0.375rem 0.625rem;
  cursor: pointer;
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.file-pick-label:hover {
  background-color: rgba(63, 63, 70, 0.9);
  border-color: var(--emerald-400);
  color: #ffffff;
}

.cover-preview-wrapper {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem;
  background-color: rgba(24, 24, 27, 0.6);
  border: 1px solid var(--zinc-700);
  border-radius: var(--radius-md);
}

.cover-preview-img {
  width: 3.5rem;
  height: 3.5rem;
  object-fit: cover;
  border-radius: var(--radius-md);
  border: 1px solid var(--zinc-600);
}

.cover-preview-hint {
  font-size: 0.75rem;
  color: var(--zinc-400);
}
</style>
