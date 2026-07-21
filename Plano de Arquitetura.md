# **Plano de Projeto: Plataforma de Rádio Web Sincronizada**

## **1\. Visão Geral do Projeto**

Este documento descreve a arquitetura e o planejamento para a construção de uma plataforma web de rádio interativa, semelhante a serviços de escuta sincronizada. A premissa central é permitir que usuários criem "estações" (salas) onde um DJ controla a playlist e a reprodução, enquanto todos os ouvintes conectados escutam a mesma música simultaneamente. Para minimizar os custos e a carga no servidor, a transferência do áudio é feita por download estático no cliente, enquanto a sincronização do estado da reprodução é mantida via WebSockets.

## **2\. Arquitetura do Sistema**

> * **Frontend (Vue 3):** Interface de usuário construída com Vue 3 (Composition API). Gerencia a reprodução de áudio localmente via HTML5 Audio API e responde aos eventos recebidos pelo WebSocket para manter o áudio sincronizado.  
> * **Backend (Go):** Servidor de alta performance e concorrência. Responsável por servir os arquivos estáticos de áudio e gerenciar as conexões WebSocket (Gorilla WebSockets), o estado das salas e a lógica de permissões.  
> * **Formato de Áudio (Opus):** Todas as faixas serão armazenadas e servidas no formato .opus (container Ogg). Isso garante alta fidelidade sonora com um consumo de banda significativamente menor (baixo bitrate) e transições sem falhas (gapless playback) entre as músicas da playlist.

## **3\. Estrutura de Dados e Estado**

O servidor Go manterá o estado de cada estação em memória, utilizando Mutexes (sync.RWMutex) para garantir segurança na concorrência, mapeando as conexões e o status da reprodução:

| Estrutura / Entidade | Descrição | Principais Atributos   |
| :---- | :---- | :---- |
| **PlaybackState** | Define o status exato da música em execução. | IsPlaying (booleano), StartedAt (timestamp em ms), SeekOffset (segundos pulados), CurrentSong (URL da música .opus). |
| **Station** | Representa uma sala ativa na plataforma. | ID, DJ (ID do usuário no controle), Clients (mapa de conexões), Broadcast (canal Go para mensagens), State (instância de PlaybackState), Repository (todas as músicas upadas), Playlist (a fila em si). |

### **3.1 Gestão de Repositório vs. Playlist**

Para separar as músicas disponíveis das músicas que efetivamente vão tocar, a Station mantém dois locais distintos:

> * **Repositório:** O "estoque" da rádio. Contém todas as músicas (convertidas e com os metadados lidos) enviadas pelo DJ para o servidor via upload.  
> * **Playlist:** A fila de reprodução (sequência de faixas). É alimentada a partir das músicas que já existem no repositório.

## **4\. Sincronização de Áudio Client-Side**

Para garantir que todos os ouvintes estejam escutando a música no mesmo segundo, o cálculo de sincronização é feito ativamente no cliente (Vue), compensando os atrasos de rede usando o timestamp base recebido do servidor Go:

> 1. O DJ clica no play, e o servidor Go registra o timestamp exato desse evento (StartedAt \= time.Now().UnixMilli()) e faz o broadcast para a sala.  
> 2. O cliente recebe a mensagem e inicia o cálculo de onde o player deveria estar:

>    Posição Atual \= ((Timestamp Atual do Cliente \- StartedAt) / 1000\) \+ SeekOffset

> 3. O cliente avalia a diferença entre a posição calculada e a propriedade currentTime da tag de áudio. Se a diferença for superior a um limite (ex: 1.0 ou 1.5 segundos), o player força o currentTime atualizando-o para corrigir a dessincronização.

## **5\. Implementação Técnica Principal**

### **5.1 Configuração Go (Backend)**

O servidor precisará declarar expressamente o MIME type para o formato Opus antes de inicializar o servidor de arquivos HTTP, evitando falhas de decodificação no navegador por tratar o formato como binário opaco.

import (  
	"log"  
	"mime"  
	"net/http"  
)

func main() {  
	// Força o MIME type correto para arquivos Opus  
	err := mime.AddExtensionType(".opus", "audio/ogg")  
	if err \!= nil {  
		log.Fatal("Erro MIME Opus:", err)  
	}

	fs := http.FileServer(http.Dir("./musicas"))  
	http.Handle("/musicas/", http.StripPrefix("/musicas/", fs))  
}

### **5.2 Configuração Vue 3 (Frontend)**

No componente de player do Vue, a tag de áudio usará explicitamente o MIME type audio/ogg; codecs=opus na source. A lógica de sincronização deve ficar isolada num Composable (ex: usePlayer.js) para manter a reatividade enxuta.

\<\!-- Player.vue \--\>  
\<template\>  
  \<audio ref="audioRef" preload="auto"\>  
    \<source :src="currentTrackUrl" type="audio/ogg; codecs=opus" /\>  
  \</audio\>  
\</template\>

### **5.3 Conversão de Mídia (Upload e Transcodificação)**

Para suportar o envio de arquivos nos formatos MP3, MP4 e WAV, o backend atua como um *wrapper* para o **FFmpeg** utilizando o pacote os/exec do Go. A função a seguir isola a trilha de áudio (descartando o vídeo, caso aplicável), converte-a para Opus utilizando bitrate variável (VBR) a 96kbps, e gerencia corretamente as saídas de erro para facilitar a depuração.

package media

import (  
	"bytes"  
	"fmt"  
	"os/exec"  
	"path/filepath"  
	"strings"  
)

var supportedFormats \= map\[string\]bool{  
	".mp3": true,  
	".mp4": true,  
	".wav": true,  
}

func ConvertToOpus(inputPath, outputPath string) error {  
	ext := strings.ToLower(filepath.Ext(inputPath))  
	if \!supportedFormats\[ext\] {  
		return fmt.Errorf("formato não suportado: %s", ext)  
	}

	args := \[\]string{  
		"-i", inputPath,  
		"-c:a", "libopus",  
		"-b:a", "96k",  
		"-vbr", "on",  
		"-vn",  
		"-y",  
		outputPath,  
	}

	cmd := exec.Command("ffmpeg", args...)  
	var stderr bytes.Buffer  
	cmd.Stderr \= \&stderr

	err := cmd.Run()  
	if err \!= nil {  
		return fmt.Errorf("falha na conversão: %v | Log: %s", err, strings.TrimSpace(stderr.String()))  
	}  
	return nil  
}

### **5.4 Otimização de Tráfego: Janela Deslizante (Top 3\)**

Em vez de enviar toda a Playlist para o ouvinte via WebSocket (o que geraria downloads excessivos e desperdício de banda caso o cliente saia da rádio sem ouvir tudo), a plataforma usa a técnica de Janela Deslizante (ou Pre-buffer):

> * **Filtro de Payload:** Sempre que há uma alteração, o backend em Go recorta o array Playlist para incluir apenas a música atual (índice 0\) e as duas seguintes (índices 1 e 2). O cliente (Vue) recebe exclusivamente esse array com, no máximo, 3 itens.  
> * **Evento TRACK\_CHANGED:** Quando a música em execução acaba, o Go remove o índice 0 da fila. A antiga música 2 passa a ser a 1, a 3 passa a ser a 2, e uma nova música passa a ocupar a 3ª posição do Payload. O cliente recebe esse payload, nota que há uma música inédita no array e, através do \<audio preload="auto"\>, já realiza o download silencioso e antecipado em background daquela faixa, garantindo a transição *gapless* sem sobrecarregar a rede do servidor abruptamente.  
> * **Evento PLAYLIST\_UPDATED:** Disparado quando o DJ altera ou insere manualmente uma nova música nas 3 primeiras posições. O cliente atualiza sua fila local e, se houver alteração brusca na música em execução, o Vue ajusta o player de acordo para reproduzir o arquivo correto.

## **6\. Planejamento da API HTTP e WebSockets**

Para separar responsabilidades e evitar conflitos de roteamento com arquivos estáticos (como o Vue gerando assets ou os arquivos .opus servidos em /musicas/), o tráfego HTTP tradicional será exposto sob o prefixo /api/v1/ para operações pesadas e controle de estado, enquanto o controle de reprodução ocorrerá via WebSocket.

### **6.1 Rotas de Gerenciamento de Estações (Rooms)**

> * **POST /api/v1/stations:** Cria uma nova estação de rádio. O usuário que faz a requisição se torna o DJ. Retorna (201 Created) um stationId e um djToken (para ser armazenado no localStorage do Vue).  
> * **GET /api/v1/stations/{stationId}:** Usado pelos ouvintes para verificar se a sala existe antes de tentar abrir o WebSocket. Retorna (200 OK) os metadados da rádio.

### **6.2 Rotas do Repositório e Upload (Apenas DJ)**

Requer cabeçalho Authorization: Bearer \<djToken\>.

> * **POST /api/v1/stations/{stationId}/upload:** Recebe o arquivo de áudio via multipart/form-data. Executa a conversão via FFmpeg e adiciona ao Repositório da rádio. Retorna (202 Accepted) o objeto da faixa que foi gerado.  
> * **GET /api/v1/stations/{stationId}/repository:** Retorna (200 OK) todas as músicas que o DJ já fez upload para o estoque da rádio.

### **6.3 Conexão WebSocket**

> * **GET /ws/stations/{stationId}:** Rota para o *Upgrade* da requisição HTTP para a conexão bidirecional WebSocket. Pode receber opcionalmente um parâmetro de query ?role=dj\&token=\<djToken\>. Caso o token seja validado, a conexão recebe permissão de emissão de comandos (Play/Pause, Add/Remove na playlist). Sem o token, a conexão atua como um mero ouvinte (apenas leitura do broadcast).

### **6.4 Fluxos de Consumo (Frontend Vue)**

> * **Ciclo do DJ:** Cria a rádio na API, armazena o token, carrega o repositório, abre a conexão WebSocket de administrador e faz upload de arquivos via POST. Quando finalizado, o DJ envia comandos JSON pelo WebSocket para inserir as faixas na playlist.  
> * **Ciclo do Ouvinte:** Acessa a URL pública da rádio, valida a existência da sala via GET, abre a conexão WebSocket padrão e começa a receber a janela das Top 3 músicas (Payload) junto do estado do player, acionando as reproduções locais.

Como sou uma IA baseada em texto, não tenho acesso para editar seu documento diretamente. No entanto, preparei o conteúdo abaixo formatado para que você possa copiar e colar facilmente no seu arquivo, antes da seção de Roadmap.

**7\. Stack Tecnológico e Pacotes7.1 Backend (Go)**

* [**github.com/gorilla/websocket**](https://github.com/gorilla/websocket)**:** Para o Upgrade da requisição HTTP e gerenciamento dos eventos WebSocket em tempo real.  
* [**github.com/go-chi/chi/v5**](https://github.com/go-chi/chi/v5)**:** Roteador HTTP minimalista e muito rápido, compatível com os handlers nativos do Go, para criar o grupo de rotas `/api/v1/` e injetar middlewares.  
* [**github.com/rs/cors**](https://github.com/rs/cors)**:** Middleware para evitar bloqueios de Cross-Origin Resource Sharing, permitindo a comunicação entre frontend e backend em domínios/portas diferentes.  
* [**github.com/google/uuid**](https://github.com/google/uuid)**:** Para gerar IDs únicos e seguros para as estações (`stationId`) e arquivos de música (`trackId`).  
* [**github.com/golang-jwt/jwt/v5**](https://github.com/golang-jwt/jwt/v5)**:** Para emitir e validar o `djToken` nas rotas protegidas.  
* [**github.com/joho/godotenv**](https://github.com/joho/godotenv)**:** Para gerenciar variáveis de ambiente de forma limpa.

**7.2 Frontend (Vue 3\)**

* **`vite`:** Bundler extremamente rápido, ideal para a Composition API.  
* **`vue-router (v4)`:** Para gerenciar o fluxo da Single Page Application (SPA), lidando com as rotas dinâmicas como `/radio/:stationId`.  
* **`pinia`:** Para armazenamento global do estado da aplicação, permitindo que a barra de progresso, a lista lateral e os controles reajam instantaneamente ao WebSocket.  
* **`axios`:** Cliente HTTP focado na comunicação com a API REST, facilitando upload de `multipart/form-data` e injeção de tokens.  
* **`tailwindcss`:** Framework CSS utilitário para prototipação veloz da interface.  
* **`lucide-vue-next`:** Biblioteca de ícones SVG limpos para os controles da plataforma.

## **8\. Roadmap e Fases de Desenvolvimento**

| Fase | Objetivo | Entregáveis   |
| :---- | :---- | :---- |
| **Fase 1: MVP** | Estabelecer a conexão e sincronização básica. | Servidor Go enviando arquivos estáticos .opus; WebSockets (echo) estabelecidos; player simples em Vue que toca a música local. |
| **Fase 2: Gestão de Salas e Fluxo do DJ** | Lidar com o tráfego multi-usuário e papéis. | Modelagem do PlaybackState, do Repositório e da Playlist no Go; implementação da API HTTP (REST); upload e conversão das músicas. |
| **Fase 3: Otimizações e Retenção** | Recursos maduros de retenção e navegação. | Implementação da Janela Deslizante (regra das Top 3 músicas e pre-buffer); telas interativas para contornar bloqueio de Autoplay. |

## **9\. Pontos de Atenção e Desafios**

> * **Restrições de Autoplay:** Navegadores modernos como Chrome e Firefox bloqueiam a reprodução de áudio não-silenciado antes de uma interação direta do usuário no DOM. É fundamental incorporar uma etapa com um botão de "Entrar na Estação" que os ouvintes devam clicar antes que a API execute o .play() acionado via WebSocket.  
> * **Duração do Arquivo (Metadados):** Para automatizar a transição de faixas na fila de reprodução, o backend Go necessita da duração exata do arquivo .opus. Como a extração nativa de containers Ogg é complexa, a estratégia recomendada é rodar um sub-processo via os/exec chamando o FFmpeg (ffprobe) no momento que a música é indexada na playlist.  
> * **Tratamento de Desconexões (Ping/Pong):** O ecossistema Gorilla WebSockets exige que o servidor gerencie ativamente a vitalidade das conexões. Deve-se implementar lógicas de Ping/Pong para identificar rapidamente se a conexão de um ouvinte caiu silenciosamente, liberando as *goroutines* e otimizando a memória consumida pela Station.

