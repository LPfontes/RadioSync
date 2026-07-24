<template>
  <div class="min-h-screen bg-zinc-900 text-zinc-100 flex flex-col">
    <header class="border-b border-zinc-800 px-4 py-3">
      <div class="max-w-4xl mx-auto flex items-center justify-between">
        <div class="flex items-center gap-2.5">
          <Disc class="w-5 h-5 text-emerald-500" />
          <span class="font-bold text-emerald-500">RadioSync — DJ</span>
          <span class="text-xs bg-zinc-800 px-2 py-0.5 rounded font-mono">{{ store.stationId }}</span>
        </div>
        <button @click="leave" class="text-xs text-zinc-400 hover:text-zinc-200 transition-colors">Sair</button>
      </div>
    </header>

    <div class="flex-1 max-w-4xl w-full mx-auto p-4 space-y-6">
      <div class="bg-zinc-800 rounded-lg p-4 text-center">
        <p class="text-xs text-zinc-400 mb-1">Compartilhe o código da estação</p>
        <div class="flex items-center justify-center gap-3">
          <span class="text-2xl font-bold tracking-widest text-emerald-400 font-mono">{{ store.stationId }}</span>
          <button @click="copyCode" class="text-xs text-zinc-400 hover:text-emerald-400 transition-colors">
            <Copy class="w-4 h-4" />
          </button>
        </div>
      </div>

      <div class="bg-zinc-800 rounded-lg p-6">
        <Player @toggle-play="handleTogglePlay" />
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <!-- Esquerda: Repositório / Biblioteca Global / Sugestões -->
        <div class="bg-zinc-800 rounded-lg p-4 flex flex-col">
          <!-- Navegação de Abas -->
          <div class="flex border-b border-zinc-700 mb-4 pb-2 gap-2 overflow-x-auto">
            <button @click="activeTab = 'repo'" class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors shrink-0" :class="activeTab === 'repo' ? 'bg-emerald-600 text-white' : 'text-zinc-400 hover:bg-zinc-700'">
              <Folder class="w-3.5 h-3.5" />
              Repositório Local
            </button>
            <button @click="switchTab('global')" class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors shrink-0" :class="activeTab === 'global' ? 'bg-emerald-600 text-white' : 'text-zinc-400 hover:bg-zinc-700'">
              <Library class="w-3.5 h-3.5" />
              Biblioteca Global
            </button>
            <button @click="activeTab = 'suggestions'" class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-colors shrink-0 relative" :class="activeTab === 'suggestions' ? 'bg-amber-600 text-white' : 'text-zinc-400 hover:bg-zinc-700'">
              <MessageSquare class="w-3.5 h-3.5" />
              Sugestões
              <span v-if="store.suggestions && store.suggestions.length > 0" class="ml-1 bg-amber-400 text-zinc-950 font-bold text-[10px] px-1.5 py-0.2 rounded-full">
                {{ store.suggestions.length }}
              </span>
            </button>
          </div>

          <!-- Conteúdo Aba Repositório Local -->
          <div v-if="activeTab === 'repo'" class="space-y-4">
            <div>
              <h3 class="text-xs font-medium text-zinc-400 mb-2">Upload de Músicas</h3>
              <div class="space-y-3">
                <input type="file" ref="fileInputRef" accept=".mp3,.mp4,.wav,.ogg,.flac,.aac,.m4a" @change="selectFile" class="block w-full text-xs text-zinc-400 file:mr-3 file:py-1.5 file:px-3 file:rounded-lg file:border-0 file:text-xs file:font-medium file:bg-zinc-700 file:text-zinc-200 hover:file:bg-zinc-600" />
                <div v-if="selectedFile" class="space-y-2 pt-1 bg-zinc-900/60 p-2.5 rounded-lg border border-zinc-700/60">
                  <span class="block truncate text-xs text-zinc-200 font-medium">{{ selectedFile.name }}</span>
                  <div class="grid grid-cols-2 gap-2">
                    <select v-model="uploadCategory" class="bg-zinc-800 text-zinc-300 text-xs rounded p-1.5 border border-zinc-700 outline-none focus:ring-1 focus:ring-emerald-500">
                      <option value="">-- Categoria (Opcional) --</option>
                      <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
                    </select>
                    <select v-model="uploadTheme" class="bg-zinc-800 text-zinc-300 text-xs rounded p-1.5 border border-zinc-700 outline-none focus:ring-1 focus:ring-emerald-500">
                      <option value="">-- Tema (Opcional) --</option>
                      <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
                    </select>
                  </div>
                  <div class="flex justify-end">
                    <button @click="handleUpload" :disabled="uploading" class="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:bg-zinc-600 disabled:opacity-50 rounded-lg text-xs font-medium transition-colors">
                      {{ uploading ? 'Enviando...' : 'Fazer Upload' }}
                    </button>
                  </div>
                </div>
                <p v-if="uploadError" class="text-xs text-red-400">{{ uploadError }}</p>
              </div>
            </div>

            <!-- Download do YouTube -->
            <div class="space-y-2 pt-2 border-t border-zinc-700/50">
              <h3 class="text-xs font-medium text-zinc-400 flex items-center gap-1.5">
                <Youtube class="w-4 h-4 text-red-500" />
                Baixar do YouTube (yt-dlp)
              </h3>
              <div class="space-y-2">
                <div class="flex gap-2">
                  <input v-model="youtubeUrl" placeholder="URL do vídeo (ex: https://youtu.be/...)" @keyup.enter="handleYouTubeDownload" class="flex-1 bg-zinc-700 rounded-lg px-3 py-1.5 text-xs outline-none focus:ring-1 focus:ring-red-500" />
                  <button @click="handleYouTubeDownload" :disabled="downloadingYT || !youtubeUrl.trim()" class="px-3 py-1.5 bg-red-600 hover:bg-red-500 disabled:bg-zinc-600 disabled:opacity-50 rounded-lg text-xs font-medium transition-colors whitespace-nowrap">
                    {{ downloadingYT ? 'Baixando...' : 'Baixar' }}
                  </button>
                </div>
                <div v-if="youtubeUrl.trim()" class="grid grid-cols-2 gap-2 bg-zinc-900/60 p-2 rounded-lg border border-zinc-700/60">
                  <select v-model="ytCategory" class="bg-zinc-800 text-zinc-300 text-xs rounded p-1.5 border border-zinc-700 outline-none focus:ring-1 focus:ring-red-500">
                    <option value="">-- Categoria (Opcional) --</option>
                    <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
                  </select>
                  <select v-model="ytTheme" class="bg-zinc-800 text-zinc-300 text-xs rounded p-1.5 border border-zinc-700 outline-none focus:ring-1 focus:ring-red-500">
                    <option value="">-- Tema (Opcional) --</option>
                    <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
                  </select>
                </div>
              </div>

              <!-- Indicador e Botão de Cookies -->
              <div class="flex items-center justify-between text-xs pt-1">
                <span class="flex items-center gap-1 text-[11px]" :class="cookieStatus ? 'text-emerald-400' : 'text-zinc-500'">
                  <span class="w-2 h-2 rounded-full" :class="cookieStatus ? 'bg-emerald-500' : 'bg-zinc-600'"></span>
                  {{ cookieStatus ? 'Cookies do YouTube ativados' : 'Sem cookies do YouTube' }}
                </span>
                <button @click="showCookieModal = !showCookieModal" class="text-[11px] text-zinc-400 hover:text-zinc-200 underline flex items-center gap-1">
                  <Key class="w-3 h-3 text-amber-400" />
                  {{ showCookieModal ? 'Fechar' : 'Configurar cookies.txt' }}
                </button>
              </div>

              <!-- Form/Card de Importação de Cookies -->
              <div v-if="showCookieModal" class="p-3 bg-zinc-900/90 border border-zinc-700/80 rounded-lg space-y-2.5 mt-2">
                <div class="flex items-center justify-between">
                  <h4 class="text-xs font-semibold text-zinc-200 flex items-center gap-1.5">
                    <Key class="w-3.5 h-3.5 text-amber-400" />
                    Importar cookies.txt do YouTube
                  </h4>
                </div>
                <p class="text-[11px] text-zinc-400 leading-tight">
                  Selecione o arquivo <code class="text-amber-300">cookies.txt</code> exportado do seu navegador ou cole o conteúdo abaixo:
                </p>

                <div class="flex flex-col gap-2">
                  <input type="file" ref="cookieFileRef" accept=".txt" @change="onCookieFileSelected" class="text-xs text-zinc-400 file:mr-2 file:py-1 file:px-2 file:rounded file:border-0 file:text-xs file:bg-zinc-700 file:text-zinc-300 hover:file:bg-zinc-600 cursor-pointer" />
                  <textarea v-model="cookieContent" rows="3" placeholder="Cole o conteúdo do cookies.txt aqui..." class="w-full bg-zinc-800 rounded p-2 text-[10px] font-mono text-zinc-300 outline-none focus:ring-1 focus:ring-amber-500 resize-none"></textarea>
                </div>

                <div class="flex items-center justify-between pt-1">
                  <span v-if="cookieMsg" class="text-[11px]" :class="cookieMsg.includes('sucesso') ? 'text-emerald-400' : 'text-red-400'">{{ cookieMsg }}</span>
                  <div v-else></div>
                  <button @click="handleSaveCookies" :disabled="savingCookies || !cookieContent.trim()" class="px-3 py-1 bg-amber-600 hover:bg-amber-500 disabled:bg-zinc-700 disabled:opacity-50 text-white rounded text-xs font-medium transition-colors">
                    {{ savingCookies ? 'Salvando...' : 'Salvar Cookies' }}
                  </button>
                </div>
              </div>

              <p v-if="ytError" class="text-xs text-red-400">{{ ytError }}</p>
            </div>

            <!-- Lista e Filtros de Músicas do Repositório Local -->
            <div class="space-y-2 pt-2 border-t border-zinc-700/50">
              <div class="flex items-center justify-between">
                <p class="text-xs text-zinc-400 font-medium flex items-center gap-1">
                  <Filter class="w-3 h-3 text-emerald-400" />
                  Músicas desta Estação
                </p>
              </div>

              <!-- Barra de Pesquisa e Filtros -->
              <div class="space-y-1.5">
                <div class="relative">
                  <Search class="w-3.5 h-3.5 absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-500" />
                  <input v-model="repoSearchQuery" placeholder="Buscar no repositório local..." class="w-full bg-zinc-700/80 rounded-lg pl-8 pr-3 py-1 text-xs outline-none focus:ring-1 focus:ring-emerald-500" />
                </div>
                <div class="grid grid-cols-2 gap-1.5 text-xs">
                  <select v-model="repoCategoryFilter" class="bg-zinc-700/60 text-zinc-300 text-[11px] rounded px-2 py-1 outline-none border border-zinc-700">
                    <option value="">Todas Categorias</option>
                    <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
                  </select>
                  <select v-model="repoThemeFilter" class="bg-zinc-700/60 text-zinc-300 text-[11px] rounded px-2 py-1 outline-none border border-zinc-700">
                    <option value="">Todos Temas</option>
                    <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
                  </select>
                </div>
              </div>

              <div v-if="filteredRepoTracks.length === 0" class="text-zinc-500 text-xs text-center py-4">
                Nenhuma música encontrada no repositório local
              </div>
              <div v-else class="space-y-1 max-h-64 overflow-y-auto pr-1">
                <div v-for="track in filteredRepoTracks" :key="track.id" class="flex items-center gap-1.5 py-1.5 px-2 rounded hover:bg-zinc-700/80 cursor-pointer text-xs group">
                  <div @click="addToPlaylist(track.id)" class="flex items-center gap-1.5 flex-1 min-w-0">
                    <Plus class="w-3.5 h-3.5 text-emerald-500 group-hover:scale-110 transition-transform shrink-0" />
                    <span class="truncate font-medium text-zinc-200">{{ track.title }}</span>
                  </div>
                  <div class="flex items-center gap-1 shrink-0">
                    <span v-if="track.category" class="bg-emerald-950 text-emerald-400 border border-emerald-800/60 px-1.5 py-0.2 rounded text-[10px]">
                      {{ track.category }}
                    </span>
                    <span v-if="track.theme" class="bg-sky-950 text-sky-400 border border-sky-800/60 px-1.5 py-0.2 rounded text-[10px]">
                      {{ track.theme }}
                    </span>
                    <span v-if="track.duration" class="text-[10px] text-zinc-500 ml-1">{{ formatTime(track.duration) }}</span>
                    <button @click.stop="openEditModal(track)" class="p-1 text-zinc-400 hover:text-amber-400 transition-colors" title="Editar Título, Categoria e Tema">
                      <Edit2 class="w-3 h-3" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Conteúdo Aba Biblioteca Global -->
          <div v-else-if="activeTab === 'global'" class="space-y-3">
            <div class="flex items-center gap-2">
              <div class="relative flex-1">
                <Search class="w-3.5 h-3.5 absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-500" />
                <input v-model="searchQuery" placeholder="Buscar na biblioteca..." class="w-full bg-zinc-700 rounded-lg pl-8 pr-3 py-1.5 text-xs outline-none focus:ring-1 focus:ring-emerald-500" />
              </div>
              <button @click="fetchGlobalLibrary" :disabled="loadingGlobal" class="p-1.5 bg-zinc-700 hover:bg-zinc-600 rounded-lg text-zinc-400 hover:text-zinc-200 transition-colors" title="Atualizar biblioteca">
                <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': loadingGlobal }" />
              </button>
            </div>

            <!-- Filtros por Categoria e Tema para Biblioteca Global -->
            <div class="grid grid-cols-2 gap-1.5 text-xs">
              <select v-model="globalCategoryFilter" class="bg-zinc-700/60 text-zinc-300 text-[11px] rounded px-2 py-1 outline-none border border-zinc-700">
                <option value="">Todas Categorias</option>
                <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
              </select>
              <select v-model="globalThemeFilter" class="bg-zinc-700/60 text-zinc-300 text-[11px] rounded px-2 py-1 outline-none border border-zinc-700">
                <option value="">Todos Temas</option>
                <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
              </select>
            </div>

            <p class="text-xs text-zinc-500 mb-2">Todas as músicas salvas no servidor (clique para adicionar à playlist)</p>

            <div v-if="loadingGlobal" class="text-zinc-500 text-xs text-center py-6">
              Carregando biblioteca...
            </div>
            <div v-else-if="filteredGlobalTracks.length === 0" class="text-zinc-500 text-xs text-center py-6">
              Nenhuma música encontrada na biblioteca
            </div>
            <div v-else class="space-y-1 max-h-64 overflow-y-auto pr-1">
              <div v-for="track in filteredGlobalTracks" :key="track.id" class="flex items-center gap-1.5 py-1.5 px-2 rounded hover:bg-zinc-700/80 cursor-pointer text-xs group">
                <div @click="addGlobalTrack(track)" class="flex items-center gap-1.5 flex-1 min-w-0">
                  <Plus class="w-3.5 h-3.5 text-emerald-500 group-hover:scale-110 transition-transform shrink-0" />
                  <span class="truncate font-medium text-zinc-200">{{ track.title }}</span>
                </div>
                <div class="flex items-center gap-1 shrink-0">
                  <span v-if="track.category" class="bg-emerald-950 text-emerald-400 border border-emerald-800/60 px-1.5 py-0.2 rounded text-[10px]">
                    {{ track.category }}
                  </span>
                  <span v-if="track.theme" class="bg-sky-950 text-sky-400 border border-sky-800/60 px-1.5 py-0.2 rounded text-[10px]">
                    {{ track.theme }}
                  </span>
                  <span v-if="track.duration" class="text-[10px] text-zinc-500 ml-1">{{ formatTime(track.duration) }}</span>
                  <button @click.stop="openEditModal(track)" class="p-1 text-zinc-400 hover:text-amber-400 transition-colors" title="Editar Título, Categoria e Tema">
                    <Edit2 class="w-3 h-3" />
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- Conteúdo Aba Sugestões de Ouvintes -->
          <div v-else-if="activeTab === 'suggestions'" class="space-y-3">
            <div class="flex items-center justify-between border-b border-zinc-700/60 pb-2">
              <p class="text-xs font-medium text-amber-400 flex items-center gap-1.5">
                <MessageSquare class="w-4 h-4" />
                Sugestões Enviadas por Ouvintes ({{ store.suggestions ? store.suggestions.length : 0 }})
              </p>
              <button
                v-if="store.suggestions && store.suggestions.length > 0"
                @click="clearSuggestions"
                class="text-[11px] text-zinc-400 hover:text-red-400 transition-colors"
              >
                Limpar todas
              </button>
            </div>

            <div v-if="!store.suggestions || store.suggestions.length === 0" class="text-zinc-500 text-xs text-center py-8">
              Nenhuma sugestão enviada pelos ouvintes no momento.
            </div>
            <div v-else class="space-y-2 max-h-80 overflow-y-auto pr-1">
              <div
                v-for="sug in store.suggestions"
                :key="sug.id"
                class="p-3 bg-zinc-900/80 border border-zinc-700/70 rounded-lg space-y-2"
              >
                <div class="flex items-start justify-between gap-2">
                  <div class="flex-1 min-w-0">
                    <h4 class="text-xs font-semibold text-zinc-200 truncate">{{ sug.title }}</h4>
                    <p class="text-[10px] text-zinc-400">
                      Sugerido por: <strong class="text-emerald-400">{{ sug.suggestedBy }}</strong>
                    </p>
                    <p v-if="sug.url" class="text-[10px] text-zinc-500 truncate font-mono mt-0.5">
                      URL: {{ sug.url }}
                    </p>
                  </div>
                </div>

                <div class="flex items-center justify-end gap-2 pt-1 border-t border-zinc-800">
                  <button
                    @click="rejectSuggestion(sug.id)"
                    class="px-2.5 py-1 bg-red-950/60 hover:bg-red-900 border border-red-800/60 text-red-300 rounded text-xs font-medium flex items-center gap-1 transition-colors"
                  >
                    <X class="w-3 h-3" />
                    Rejeitar
                  </button>
                  <button
                    @click="approveSuggestion(sug)"
                    class="px-2.5 py-1 bg-emerald-600 hover:bg-emerald-500 text-white rounded text-xs font-medium flex items-center gap-1 transition-colors"
                  >
                    <Check class="w-3 h-3" />
                    Aprovar & Tocar
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Direita: Playlist -->
        <div class="bg-zinc-800 rounded-lg p-4">
          <h3 class="text-sm font-medium text-zinc-400 mb-3">Playlist Fila de Reprodução</h3>
          <div v-if="store.playlist.length === 0" class="text-zinc-500 text-sm text-center py-4">
            Playlist vazia — adicione músicas do repositório ou da biblioteca global
          </div>
          <div v-for="(track, i) in store.playlist" :key="track.id"
            @click="playPlaylistTrack(track.id)"
            class="flex items-center gap-2.5 py-2 px-2 border-b border-zinc-700/50 last:border-0 rounded-lg hover:bg-zinc-700/60 transition-colors cursor-pointer group"
            :class="{ 'bg-emerald-950/40 border-emerald-800/40': i === 0 && store.state.isPlaying }">
            
            <button @click.stop="removeFromPlaylist(track.id)" class="text-red-400 hover:text-red-300 p-0.5 shrink-0" title="Remover da playlist">
              <X class="w-3.5 h-3.5" />
            </button>
            
            <div class="w-4 flex items-center justify-center shrink-0">
              <Play v-if="i === 0 && store.state.isPlaying" class="w-3.5 h-3.5 text-emerald-400 animate-pulse" />
              <span v-else class="text-xs text-zinc-500 group-hover:text-zinc-300 font-mono">{{ i + 1 }}</span>
            </div>

            <div class="flex-1 min-w-0">
              <div class="text-sm truncate" :class="i === 0 ? 'text-emerald-400 font-semibold' : 'text-zinc-200 group-hover:text-white'">
                {{ track.title }}
              </div>
              <div v-if="track.category || track.theme" class="flex items-center gap-1 mt-0.5">
                <span v-if="track.category" class="bg-emerald-950/80 text-emerald-400 border border-emerald-800/50 text-[9px] px-1 rounded">
                  {{ track.category }}
                </span>
                <span v-if="track.theme" class="bg-sky-950/80 text-sky-400 border border-sky-800/50 text-[9px] px-1 rounded">
                  {{ track.theme }}
                </span>
              </div>
            </div>

            <span v-if="track.duration" class="text-[10px] text-zinc-500 shrink-0">{{ formatTime(track.duration) }}</span>

            <button @click.stop="playPlaylistTrack(track.id)" class="p-1 rounded text-zinc-400 group-hover:text-emerald-400 hover:bg-zinc-600/50 transition-colors shrink-0" title="Tocar esta música agora">
              <Play class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Modal de Edição de Metadados (Título, Categoria e Tema) -->
    <div v-if="showEditModal" class="fixed inset-0 bg-black/70 flex items-center justify-center p-4 z-50">
      <div class="bg-zinc-800 border border-zinc-700 rounded-xl p-5 max-w-md w-full space-y-4 shadow-xl">
        <div class="flex items-center justify-between border-b border-zinc-700 pb-3">
          <h3 class="text-sm font-bold text-zinc-100 flex items-center gap-2">
            <Edit2 class="w-4 h-4 text-emerald-400" />
            Editar Mídia
          </h3>
          <button @click="showEditModal = false" class="text-zinc-400 hover:text-zinc-200">
            <X class="w-4 h-4" />
          </button>
        </div>

        <div class="space-y-3">
          <div>
            <label class="block text-xs font-medium text-zinc-400 mb-1">Título da Música</label>
            <input v-model="editTitle" type="text" class="w-full bg-zinc-700 text-zinc-100 text-xs rounded-lg px-3 py-2 outline-none focus:ring-1 focus:ring-emerald-500" />
          </div>

          <div>
            <label class="block text-xs font-medium text-zinc-400 mb-1">Categoria</label>
            <div class="flex gap-2">
              <select v-model="editCategory" class="flex-1 bg-zinc-700 text-zinc-100 text-xs rounded-lg px-3 py-2 outline-none focus:ring-1 focus:ring-emerald-500">
                <option value="">-- Nenhuma Categoria --</option>
                <option v-for="cat in categoriesList" :key="cat" :value="cat">{{ cat }}</option>
              </select>
              <input v-model="editCategory" placeholder="ou digite customizada..." type="text" class="flex-1 bg-zinc-700 text-zinc-100 text-xs rounded-lg px-3 py-2 outline-none focus:ring-1 focus:ring-emerald-500" />
            </div>
          </div>

          <div>
            <label class="block text-xs font-medium text-zinc-400 mb-1">Tema</label>
            <div class="flex gap-2">
              <select v-model="editTheme" class="flex-1 bg-zinc-700 text-zinc-100 text-xs rounded-lg px-3 py-2 outline-none focus:ring-1 focus:ring-emerald-500">
                <option value="">-- Nenhum Tema --</option>
                <option v-for="thm in themesList" :key="thm" :value="thm">{{ thm }}</option>
              </select>
              <input v-model="editTheme" placeholder="ou digite customizado..." type="text" class="flex-1 bg-zinc-700 text-zinc-100 text-xs rounded-lg px-3 py-2 outline-none focus:ring-1 focus:ring-emerald-500" />
            </div>
          </div>
        </div>

        <div class="flex justify-end gap-2 pt-2 border-t border-zinc-700">
          <button @click="showEditModal = false" class="px-3 py-1.5 bg-zinc-700 hover:bg-zinc-600 text-zinc-300 rounded-lg text-xs font-medium transition-colors">
            Cancelar
          </button>
          <button @click="handleSaveEdit" :disabled="savingEdit" class="px-4 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:bg-zinc-600 text-white rounded-lg text-xs font-medium transition-colors flex items-center gap-1.5">
            <Check class="w-3.5 h-3.5" />
            {{ savingEdit ? 'Salvando...' : 'Salvar Alterações' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, X, SkipForward, Copy, Folder, Library, Search, RefreshCw, Youtube, Key, Play, Disc, MessageSquare, Check, Edit2, Tag, Filter } from 'lucide-vue-next'
import { useStationStore } from '../stores/station'
import { uploadMusic, getRepository, getGlobalLibrary, downloadFromYouTube, saveYouTubeCookies, getCookiesStatus, getStation, updateTrackMetadata } from '../services/api'
import { useWebSocket } from '../composables/useWebSocket'
import { getSavedStations, saveStation, removeStation } from '../services/storage'
import Player from '../components/Player.vue'

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
const savingEdit = ref(false)

const categoriesList = ['Rock', 'Pop', 'Sertanejo', 'MPB', 'Eletrônica', 'Gospel', 'Funk', 'Hip-Hop', 'Reggae', 'Efeitos / Trilhas', 'Outros']
const themesList = ['Abertura', 'Vinheta', 'Fundo Musical', 'Comercial', 'Encerramento', 'Entrevista', 'Geral']

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
  showEditModal.value = true
}

async function handleSaveEdit() {
  if (!editingTrack.value) return
  savingEdit.value = true
  try {
    const updated = await updateTrackMetadata(
      store.stationId,
      editingTrack.value.id,
      { title: editTitle.value, category: editCategory.value, theme: editTheme.value },
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

function leave() {
  disconnect()
  removeStation(store.stationId)
  store.reset()
  router.push('/')
}
</script>
