// Gerenciador de Reprodução de Áudios Reais do Mixer
class AmbientAudioService {
  constructor() {
    this.channels = new Map() // id -> { channel, audioEl, isPlaying }
    this.localVolumeMultipliers = new Map() // id -> multiplier (0.0 a 1.0)
    this.listenerPersonalMixEnabled = false
  }

  // Obtém a URL do arquivo de áudio real do canal
  getAudioUrl(ch) {
    if (ch.trackUrl) return ch.trackUrl
    if (ch.presetKey) return `/sounds/${ch.presetKey}.mp3`
    return ''
  }

  // Sincroniza a reprodução de todos os canais do mixer com a rádio
  sync(channels = [], isStationPlaying = true, masterVolume = 1.0, isMuted = false) {
    const activeIds = new Set()

    for (const ch of channels) {
      activeIds.add(ch.id)
      let chState = this.channels.get(ch.id)
      const url = this.getAudioUrl(ch)

      if (!url) continue

      if (!chState) {
        const audio = new Audio()
        audio.src = url
        audio.loop = false
        audio.preload = 'auto'

        // Trata erro caso o arquivo ainda não tenha sido colocado na pasta
        audio.onerror = () => {
          console.warn(`[Mixer] Áudio não encontrado ou erro ao reproduzir: ${url}`)
        }

        chState = {
          channel: ch,
          audioEl: audio,
          isPlaying: false,
        }
        this.channels.set(ch.id, chState)
      } else {
        chState.channel = ch
        // Se a URL mudou
        if (chState.audioEl.src !== url && !chState.audioEl.src.endsWith(url)) {
          chState.audioEl.src = url
        }
      }

      // Calcula volume efetivo (estação x ajuste local do ouvinte x master)
      const baseVol = typeof ch.volume === 'number' ? ch.volume : 0.5
      let localMult = 1.0
      if (this.listenerPersonalMixEnabled && this.localVolumeMultipliers.has(ch.id)) {
        localMult = this.localVolumeMultipliers.get(ch.id)
      }

      // Permite tocar os efeitos sonoros/ambiente independentemente de haver música tocando
      const shouldPlay = ch.isPlaying && !ch.muted && !isMuted
      const targetVolume = shouldPlay ? Math.max(0, Math.min(1.0, baseVol * localMult * masterVolume)) : 0.0

      if (chState.audioEl) {
        chState.audioEl.volume = targetVolume
        chState.audioEl.loop = ch.loop === true

        chState.audioEl.onended = () => {
          if (!ch.loop) {
            chState.isPlaying = false
            chState.audioEl.currentTime = 0
            if (this.onChannelEnded) {
              this.onChannelEnded(ch.id)
            }
          }
        }

        if (shouldPlay && targetVolume > 0) {
          if (chState.audioEl.paused) {
            if (chState.audioEl.ended || (!ch.loop && !chState.isPlaying)) {
              chState.audioEl.currentTime = 0
            }
            chState.audioEl.play().then(() => {
              chState.isPlaying = true
            }).catch(() => {
              // Bloqueio de autoplay do navegador antes de interação do usuário
            })
          }
        } else {
          if (!chState.audioEl.paused) {
            chState.audioEl.pause()
          }
          chState.isPlaying = false
        }
      }
    }

    // Remove canais que foram deletados do mixer
    for (const [id, chState] of this.channels.entries()) {
      if (!activeIds.has(id)) {
        this.stopChannel(chState)
        this.channels.delete(id)
      }
    }
  }

  stopChannel(chState) {
    if (!chState || !chState.audioEl) return
    try {
      chState.audioEl.pause()
      chState.audioEl.src = ''
      chState.audioEl.load()
    } catch {}
    chState.isPlaying = false
  }

  setLocalMultiplier(channelId, multiplier) {
    this.localVolumeMultipliers.set(channelId, Math.max(0, Math.min(1.0, multiplier)))
  }

  setPersonalMixEnabled(enabled) {
    this.listenerPersonalMixEnabled = !!enabled
  }

  stopAll() {
    for (const chState of this.channels.values()) {
      this.stopChannel(chState)
    }
    this.channels.clear()
  }
}

export const ambientAudio = new AmbientAudioService()
