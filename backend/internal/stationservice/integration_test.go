package stationservice

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"radio-backend/internal/auth"
	"radio-backend/internal/handler"
	"radio-backend/internal/model"
	"radio-backend/internal/storage"
	"radio-backend/internal/syncservice"

	"github.com/go-chi/chi/v5"
	gorillaWS "github.com/gorilla/websocket"
)

// TestINT04_ReplicaStationConsistency valida o cenário INT-04 (ADR-0006):
// "Estação criada pela réplica A é lida pela réplica B com os mesmos dados".
func TestINT04_ReplicaStationConsistency(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste-int04-com-32-bytes!!")

	sharedRepo := storage.NewMemoryRepository()
	sharedStore := storage.NewMemoryObjectStore(storage.DefaultMusicBucket)
	sharedBroker := syncservice.NewMemoryBroker()

	syncA := syncservice.NewMemorySyncStorageWithBroker("station-replica-a", sharedBroker)
	syncB := syncservice.NewMemorySyncStorageWithBroker("station-replica-b", sharedBroker)

	replicaA := NewService(Config{
		InstanceID:  "station-replica-a",
		Repo:        sharedRepo,
		ObjectStore: sharedStore,
		SyncStorage: syncA,
	})
	replicaB := NewService(Config{
		InstanceID:  "station-replica-b",
		Repo:        sharedRepo,
		ObjectStore: sharedStore,
		SyncStorage: syncB,
	})

	srvA := httptest.NewServer(replicaA.Router())
	defer srvA.Close()
	srvB := httptest.NewServer(replicaB.Router())
	defer srvB.Close()

	// 1. Cria estação na Réplica A
	respCreate, err := http.Post(srvA.URL+"/api/v1/stations", "application/json", nil)
	if err != nil {
		t.Fatalf("erro no POST /api/v1/stations na réplica A: %v", err)
	}
	defer respCreate.Body.Close()
	if respCreate.StatusCode != http.StatusCreated {
		t.Fatalf("esperava HTTP 201 na réplica A, obteve %d", respCreate.StatusCode)
	}

	var created struct {
		StationID string `json:"stationId"`
		DJToken   string `json:"djToken"`
	}
	if err := json.NewDecoder(respCreate.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if len(created.StationID) != 6 || created.DJToken == "" {
		t.Fatalf("resposta de criação inválida: %+v", created)
	}

	// 2. Popula faixa, playlist e áudio no ObjectStore via Réplica A / Repositório compartilhado
	ctx := context.Background()
	track := model.Track{
		ID:       "track-int04",
		Title:    "Daft Punk - Voyager",
		Filename: "track-int04.opus",
		URL:      "/musicas/track-int04.opus",
		Duration: 227.5,
		Category: "French House",
	}
	if err := sharedRepo.AddTrackToStation(ctx, created.StationID, track); err != nil {
		t.Fatal(err)
	}
	if err := sharedStore.Put(ctx, track.Filename, bytes.NewReader([]byte("OggS-voyager-opus")), 17, "audio/ogg"); err != nil {
		t.Fatal(err)
	}

	plBody, _ := json.Marshal(map[string]interface{}{
		"playlist": []model.Track{track},
	})
	reqPl, _ := http.NewRequest(http.MethodPut, srvA.URL+"/api/v1/stations/"+created.StationID+"/playlist", bytes.NewReader(plBody))
	reqPl.Header.Set("Authorization", "Bearer "+created.DJToken)
	reqPl.Header.Set("Content-Type", "application/json")
	respPl, err := http.DefaultClient.Do(reqPl)
	if err != nil || respPl.StatusCode != http.StatusOK {
		t.Fatalf("falha ao atualizar playlist na réplica A: status=%v err=%v", respPl, err)
	}
	respPl.Body.Close()

	// 3. Lê estação na Réplica B como DJ e verifica paridade completa de dados
	reqGetB, _ := http.NewRequest(http.MethodGet, srvB.URL+"/api/v1/stations/"+created.StationID, nil)
	reqGetB.Header.Set("Authorization", "Bearer "+created.DJToken)
	respGetB, err := http.DefaultClient.Do(reqGetB)
	if err != nil {
		t.Fatalf("erro no GET /api/v1/stations/{id} na réplica B: %v", err)
	}
	defer respGetB.Body.Close()
	if respGetB.StatusCode != http.StatusOK {
		t.Fatalf("esperava HTTP 200 na réplica B, obteve %d", respGetB.StatusCode)
	}

	var stationOnB struct {
		ID         string        `json:"id"`
		Role       string        `json:"role"`
		DJToken    string        `json:"djToken"`
		Playlist   []model.Track `json:"playlist"`
		Repository []model.Track `json:"repository"`
	}
	if err := json.NewDecoder(respGetB.Body).Decode(&stationOnB); err != nil {
		t.Fatal(err)
	}

	if stationOnB.ID != created.StationID {
		t.Errorf("ID divergente na réplica B: esperado %s, obtido %s", created.StationID, stationOnB.ID)
	}
	if stationOnB.Role != "dj" || stationOnB.DJToken == "" {
		t.Errorf("réplica B não reconheceu token de DJ criado na réplica A: role=%s", stationOnB.Role)
	}
	if len(stationOnB.Playlist) != 1 || stationOnB.Playlist[0].Title != "Daft Punk - Voyager" {
		t.Errorf("playlist lida na réplica B difere da gravada na réplica A: %+v", stationOnB.Playlist)
	}
	if len(stationOnB.Repository) != 1 || stationOnB.Repository[0].ID != "track-int04" {
		t.Errorf("repositório lido na réplica B difere do gravado na réplica A: %+v", stationOnB.Repository)
	}

	// 4. Baixa o áudio na Réplica B a partir do MinIO/ObjectStore sem disco local
	respAudioB, err := http.Get(srvB.URL + "/musicas/track-int04.opus")
	if err != nil {
		t.Fatal(err)
	}
	defer respAudioB.Body.Close()
	audioBytes, _ := io.ReadAll(respAudioB.Body)
	if respAudioB.StatusCode != http.StatusOK || string(audioBytes) != "OggS-voyager-opus" {
		t.Errorf("áudio servido pela réplica B inválido: status=%d body=%q", respAudioB.StatusCode, string(audioBytes))
	}
}

// TestINT05_StranglerFigContractParity valida o cenário INT-05 (ADR-0003):
// "Rotas migradas pelo gateway respondem igual ao monolito (mesmo status e corpo)".
func TestINT05_StranglerFigContractParity(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste-int05-com-32-bytes!!")

	// Prepara Monolito
	monoRouter := chi.NewRouter()
	monoRouter.Post("/api/v1/stations", handler.CreateStation)
	monoRouter.Get("/api/v1/stations/{stationId}", handler.GetStation)
	monoRouter.Post("/api/v1/stations/{stationId}/claim-dj", handler.ClaimDJRole)
	monoRouter.Get("/api/v1/stations/{stationId}/repository", handler.GetRepository)
	monoRouter.Put("/api/v1/stations/{stationId}/tracks/{trackId}", handler.UpdateTrackMetadata)
	monoRouter.Get("/api/v1/library", handler.GetGlobalLibrary)
	monoSrv := httptest.NewServer(monoRouter)
	defer monoSrv.Close()

	// Prepara Station Service (destino das rotas no Traefik)
	repo := storage.NewMemoryRepository()
	store := storage.NewMemoryObjectStore(storage.DefaultMusicBucket)
	syncStore := syncservice.NewMemorySyncStorage("station-strangler")
	stationSvc := NewService(Config{
		InstanceID:  "station-strangler",
		Repo:        repo,
		ObjectStore: store,
		SyncStorage: syncStore,
	})
	stationSrv := httptest.NewServer(stationSvc.Router())
	defer stationSrv.Close()

	// 1. POST /api/v1/stations -> ambos retornam 201 Created com stationId (6 chars) e djToken válido
	resMonoCreate, err := http.Post(monoSrv.URL+"/api/v1/stations", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resMonoCreate.Body.Close()

	resSvcCreate, err := http.Post(stationSrv.URL+"/api/v1/stations", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resSvcCreate.Body.Close()

	if resMonoCreate.StatusCode != resSvcCreate.StatusCode || resSvcCreate.StatusCode != http.StatusCreated {
		t.Fatalf("status POST /stations divergente: mono=%d station=%d", resMonoCreate.StatusCode, resSvcCreate.StatusCode)
	}

	var bodyMonoCreate, bodySvcCreate map[string]string
	_ = json.NewDecoder(resMonoCreate.Body).Decode(&bodyMonoCreate)
	_ = json.NewDecoder(resSvcCreate.Body).Decode(&bodySvcCreate)

	monoID, monoToken := bodyMonoCreate["stationId"], bodyMonoCreate["djToken"]
	svcID, svcToken := bodySvcCreate["stationId"], bodySvcCreate["djToken"]
	if len(monoID) != len(svcID) || !auth.ValidateDJToken(svcToken, svcID) {
		t.Fatalf("contrato de POST /stations divergente: mono=%v station=%v", bodyMonoCreate, bodySvcCreate)
	}

	// 2. GET /api/v1/stations/{id} (como ouvinte e como DJ) -> mesmas chaves JSON, status 200 e Cache-Control: no-store para DJ
	checkGetParity := func(authBearerMono, authBearerSvc, expectedRole string) {
		reqM, _ := http.NewRequest(http.MethodGet, monoSrv.URL+"/api/v1/stations/"+monoID, nil)
		reqS, _ := http.NewRequest(http.MethodGet, stationSrv.URL+"/api/v1/stations/"+svcID, nil)
		if authBearerMono != "" {
			reqM.Header.Set("Authorization", "Bearer "+authBearerMono)
			reqS.Header.Set("Authorization", "Bearer "+authBearerSvc)
		}
		respM, _ := http.DefaultClient.Do(reqM)
		respS, _ := http.DefaultClient.Do(reqS)
		defer respM.Body.Close()
		defer respS.Body.Close()

		if respM.StatusCode != respS.StatusCode {
			t.Fatalf("status GET /stations/{id} (%s) divergente: mono=%d station=%d", expectedRole, respM.StatusCode, respS.StatusCode)
		}
		if respM.Header.Get("Cache-Control") != respS.Header.Get("Cache-Control") {
			t.Errorf("Cache-Control divergente (%s): mono=%q station=%q", expectedRole, respM.Header.Get("Cache-Control"), respS.Header.Get("Cache-Control"))
		}

		var mapM, mapS map[string]interface{}
		_ = json.NewDecoder(respM.Body).Decode(&mapM)
		_ = json.NewDecoder(respS.Body).Decode(&mapS)

		for _, key := range []string{"id", "playlist", "state", "repository", "suggestions", "mixer", "hasDJOnline", "role"} {
			if _, okM := mapM[key]; !okM {
				t.Errorf("chave %s ausente no monolito", key)
			}
			if _, okS := mapS[key]; !okS {
				t.Errorf("chave %s ausente no Station Service", key)
			}
		}
		if mapM["role"] != expectedRole || mapS["role"] != expectedRole {
			t.Errorf("role divergente: mono=%v station=%v esperado=%s", mapM["role"], mapS["role"], expectedRole)
		}
	}

	checkGetParity("", "", "listener")
	checkGetParity(monoToken, svcToken, "dj")

	// 3. GET /api/v1/stations/{id} para estação inexistente -> ambos 404
	rM404, _ := http.Get(monoSrv.URL + "/api/v1/stations/NAOEXI")
	rS404, _ := http.Get(stationSrv.URL + "/api/v1/stations/NAOEXI")
	rM404.Body.Close()
	rS404.Body.Close()
	if rM404.StatusCode != rS404.StatusCode || rS404.StatusCode != http.StatusNotFound {
		t.Errorf("status 404 divergente: mono=%d station=%d", rM404.StatusCode, rS404.StatusCode)
	}

	// 4. POST /api/v1/stations/{id}/claim-dj -> ambos 200 OK com stationId e djToken
	rMClaim, _ := http.Post(monoSrv.URL+"/api/v1/stations/"+monoID+"/claim-dj", "application/json", nil)
	rSClaim, _ := http.Post(stationSrv.URL+"/api/v1/stations/"+svcID+"/claim-dj", "application/json", nil)
	defer rMClaim.Body.Close()
	defer rSClaim.Body.Close()
	if rMClaim.StatusCode != rSClaim.StatusCode || rSClaim.StatusCode != http.StatusOK {
		t.Errorf("status claim-dj divergente: mono=%d station=%d", rMClaim.StatusCode, rSClaim.StatusCode)
	}

	// 5. GET /api/v1/stations/{id}/repository sem token -> ambos 401 Unauthorized
	rMRepo401, _ := http.Get(monoSrv.URL + "/api/v1/stations/" + monoID + "/repository")
	rSRepo401, _ := http.Get(stationSrv.URL + "/api/v1/stations/" + svcID + "/repository")
	rMRepo401.Body.Close()
	rSRepo401.Body.Close()
	if rMRepo401.StatusCode != rSRepo401.StatusCode || rSRepo401.StatusCode != http.StatusUnauthorized {
		t.Errorf("status repository sem token divergente: mono=%d station=%d", rMRepo401.StatusCode, rSRepo401.StatusCode)
	}
}

// TestRES05_PostgresDown_ListenersContinueAndCreateStationFailsClearly valida o cenário RES-05:
// "Postgres fora do ar, ouvintes continuam escutando (estado no Redis) e a criação de estação falha com erro claro".
func TestRES05_PostgresDown_ListenersContinueAndCreateStationFailsClearly(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste-res05-com-32-bytes!!")

	repo := storage.NewMemoryRepository()
	store := storage.NewMemoryObjectStore(storage.DefaultMusicBucket)
	broker := syncservice.NewMemoryBroker()
	syncStore := syncservice.NewMemorySyncStorageWithBroker("sync-res05", broker)

	stationSvc := NewService(Config{
		InstanceID:  "station-res05",
		Repo:        repo,
		ObjectStore: store,
		SyncStorage: syncStore,
	})
	stationSrv := httptest.NewServer(stationSvc.Router())
	defer stationSrv.Close()

	// 1. Com Postgres online, cria uma estação ativa
	respCreate, err := http.Post(stationSrv.URL+"/api/v1/stations", "application/json", nil)
	if err != nil || respCreate.StatusCode != http.StatusCreated {
		t.Fatalf("falha ao criar estação inicial: status=%v err=%v", respCreate, err)
	}
	var created struct {
		StationID string `json:"stationId"`
		DJToken   string `json:"djToken"`
	}
	_ = json.NewDecoder(respCreate.Body).Decode(&created)
	respCreate.Body.Close()

	// 2. Sobe o Sync Service (WebSocket) com DJ e Ouvinte conectados na estação
	syncSvc := syncservice.NewSyncService(syncStore, "sync-res05", nil)
	defer syncSvc.Close()

	wsRouter := chi.NewRouter()
	wsRouter.Get("/ws/stations/{stationId}", syncSvc.HandleWebSocket)
	wsSrv := httptest.NewServer(wsRouter)
	defer wsSrv.Close()

	wsURL := "ws" + strings.TrimPrefix(wsSrv.URL, "http") + "/ws/stations/" + created.StationID

	// Conecta DJ usando subprotocolo seguro
	djHeader := http.Header{
		"Sec-WebSocket-Protocol": []string{"radiosync.v1, radiosync.auth." + created.DJToken},
	}
	djConn, _, err := gorillaWS.DefaultDialer.Dial(wsURL, djHeader)
	if err != nil {
		t.Fatalf("erro ao conectar DJ no Sync Service: %v", err)
	}
	defer djConn.Close()

	// Conecta Ouvinte
	listenerConn, _, err := gorillaWS.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("erro ao conectar Ouvinte no Sync Service: %v", err)
	}
	defer listenerConn.Close()

	// 3. Simula queda do PostgreSQL (RES-05)
	repo.SetAvailable(false)

	// 4. Tentativa de criar nova estação deve falhar com HTTP 503 e mensagem de erro clara
	respFail, err := http.Post(stationSrv.URL+"/api/v1/stations", "application/json", nil)
	if err != nil {
		t.Fatalf("erro de transporte HTTP ao testar falha do Postgres: %v", err)
	}
	defer respFail.Body.Close()

	if respFail.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("RES-05: esperava HTTP 503 Service Unavailable com Postgres fora do ar, obteve %d", respFail.StatusCode)
	}
	var errPayload map[string]string
	if err := json.NewDecoder(respFail.Body).Decode(&errPayload); err != nil || !strings.Contains(strings.ToLower(errPayload["error"]), "indisponível") {
		t.Fatalf("RES-05: esperava erro JSON claro explicando indisponibilidade do banco, obteve: %+v", errPayload)
	}

	// 5. Enquanto o Postgres está fora do ar, o DJ envia PLAY e o Ouvinte continua recebendo normalmente (estado no Redis)
	playCmd := []byte(`{"type":"PLAY"}`)
	if err := djConn.WriteMessage(gorillaWS.TextMessage, playCmd); err != nil {
		t.Fatalf("RES-05: falha ao enviar comando PLAY pelo DJ durante queda do Postgres: %v", err)
	}

	_ = listenerConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	receivedPlay := false
	for i := 0; i < 6; i++ {
		_, msgBytes, err := listenerConn.ReadMessage()
		if err != nil {
			break
		}
		var msg struct {
			Type  string               `json:"type"`
			State *model.PlaybackState `json:"state"`
		}
		if json.Unmarshal(msgBytes, &msg) == nil && msg.Type == "STATE_UPDATE" && msg.State != nil && msg.State.IsPlaying {
			receivedPlay = true
			break
		}
	}
	if !receivedPlay {
		t.Fatalf("RES-05: ouvinte não recebeu STATE_UPDATE (isPlaying=true) enquanto o Postgres estava fora do ar")
	}

	// 6. Restaura o Postgres e verifica que a criação de estação volta a funcionar imediatamente
	repo.SetAvailable(true)
	respRecovered, err := http.Post(stationSrv.URL+"/api/v1/stations", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer respRecovered.Body.Close()
	if respRecovered.StatusCode != http.StatusCreated {
		t.Fatalf("esperava HTTP 201 após retorno do Postgres, obteve %d", respRecovered.StatusCode)
	}
}
