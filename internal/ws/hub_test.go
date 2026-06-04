package ws_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/vladyslavkondratiuk/postup/internal/db"
	"github.com/vladyslavkondratiuk/postup/internal/handlers"
	"github.com/vladyslavkondratiuk/postup/internal/middleware"
	"github.com/vladyslavkondratiuk/postup/internal/session"
	"github.com/vladyslavkondratiuk/postup/internal/ws"
)

// wsURL converts http://... to ws://...
func wsURL(srv *httptest.Server) string {
	return "ws" + srv.URL[4:]
}

// wsConnect dials and registers t.Cleanup to close.
func wsConnect(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(srv), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// serveWS upgrades the request and registers the client in hub.
func serveWS(hub *ws.Hub, retroID, userID int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &ws.Client{
			Hub:     hub,
			Conn:    conn,
			Send:    make(chan []byte, 256),
			RetroID: retroID,
			UserID:  userID,
		}
		hub.Register(client)
		go client.WritePump()
		go client.ReadPump()
	}
}

// waitFor polls cond every 10ms for up to 2s.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 2s")
}

// readMsg reads one data frame from the connection with a 2s deadline.
func readMsg(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return msg
}

// Тест 1 — підключення реєструє клієнта в кімнаті хаба
func TestHubRegisterClient(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	const retroID = int64(42)
	srv := httptest.NewServer(serveWS(hub, retroID, 1))
	defer srv.Close()

	_ = wsConnect(t, srv)

	waitFor(t, func() bool { return hub.RoomSize(retroID) == 1 })
}

// Тест 2 — broadcast одному клієнту, клієнт отримує type="card_created"
func TestHubBroadcastOneClient(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	const retroID = int64(10)
	srv := httptest.NewServer(serveWS(hub, retroID, 1))
	defer srv.Close()

	conn := wsConnect(t, srv)
	waitFor(t, func() bool { return hub.RoomSize(retroID) == 1 })

	// senderID=0 → жоден клієнт не пропускається
	hub.Broadcast(retroID, 0, "card_created", map[string]any{"content": "test"})

	msg := readMsg(t, conn)
	if msg["type"] != "card_created" {
		t.Errorf("expected type=card_created, got %v", msg["type"])
	}
}

// Тест 3 — broadcast двом клієнтам в одній кімнаті, обидва отримують
func TestHubBroadcastTwoClients(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	const retroID = int64(20)
	srv := httptest.NewServer(serveWS(hub, retroID, 1))
	defer srv.Close()

	conn1 := wsConnect(t, srv)
	conn2 := wsConnect(t, srv)
	waitFor(t, func() bool { return hub.RoomSize(retroID) == 2 })

	hub.Broadcast(retroID, 0, "card_created", map[string]any{"content": "hello"})

	msg1 := readMsg(t, conn1)
	msg2 := readMsg(t, conn2)

	if msg1["type"] != "card_created" {
		t.Errorf("client1: expected card_created, got %v", msg1["type"])
	}
	if msg2["type"] != "card_created" {
		t.Errorf("client2: expected card_created, got %v", msg2["type"])
	}
}

// Тест 4 — ізоляція кімнат: broadcast в retro 1 не досягає клієнта retro 2
func TestHubRoomIsolation(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	srvA := httptest.NewServer(serveWS(hub, 1, 10))
	defer srvA.Close()
	srvB := httptest.NewServer(serveWS(hub, 2, 20))
	defer srvB.Close()

	connA := wsConnect(t, srvA)
	connB := wsConnect(t, srvB)
	waitFor(t, func() bool {
		return hub.RoomSize(1) == 1 && hub.RoomSize(2) == 1
	})

	hub.Broadcast(1, 0, "card_created", map[string]any{"id": 1})

	// A отримує
	msgA := readMsg(t, connA)
	if msgA["type"] != "card_created" {
		t.Errorf("A: expected card_created, got %v", msgA["type"])
	}

	// B не отримує — deadline закінчується без даних
	connB.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := connB.ReadMessage()
	if err == nil {
		t.Error("retro 2 client should not receive event from retro 1")
	}
}

// Тест 5 — відключення клієнта видаляє його з хаба
func TestHubUnregisterOnDisconnect(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()

	const retroID = int64(30)
	srv := httptest.NewServer(serveWS(hub, retroID, 1))
	defer srv.Close()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(srv), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	waitFor(t, func() bool { return hub.RoomSize(retroID) == 1 })

	conn.Close()

	waitFor(t, func() bool { return hub.RoomSize(retroID) == 0 })
}

// Тест 6 — HandleCardsCreate → WS клієнт отримує card_created з content і author_name
func TestHubBroadcastOnCardCreate(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	hub := ws.NewHub()
	go hub.Run()

	now := time.Now().UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	res, _ := database.Exec(
		`INSERT INTO users (email, password_hash, first_name, last_name, role, created_at, updated_at)
		 VALUES ('alice@test.com', '', 'Аліса', 'Коваль', 'member', ?, ?)`,
		now, now,
	)
	userID, _ := res.LastInsertId()

	res, _ = database.Exec(`INSERT INTO teams (name, created_at) VALUES ('Alpha', ?)`, now)
	teamID, _ := res.LastInsertId()

	res, _ = database.Exec(`INSERT INTO templates (name, status, created_at) VALUES ('T', 'active', ?)`, now)
	tmplID, _ := res.LastInsertId()
	database.Exec(
		`INSERT INTO template_columns (template_id, title, position, type) VALUES (?, 'Добре', 1, 'regular')`,
		tmplID,
	)
	var colID int64
	database.QueryRow(`SELECT id FROM template_columns WHERE template_id = ?`, tmplID).Scan(&colID)

	res, _ = database.Exec(
		`INSERT INTO retros (team_id, template_id, date, vote_limit, status, created_at) VALUES (?, ?, ?, 10, 'active', ?)`,
		teamID, tmplID, future, now,
	)
	retroID, _ := res.LastInsertId()
	database.Exec(`INSERT INTO retro_participants (retro_id, user_id) VALUES (?, ?)`, retroID, userID)

	// WS listener — userID 999, не є постером, отримає broadcast
	wsSrv := httptest.NewServer(serveWS(hub, retroID, 999))
	defer wsSrv.Close()

	// HTTP server для HandleCardsCreate
	auth := middleware.NewAuth(database)
	mux := http.NewServeMux()
	mux.Handle("POST /retros/{id}/cards", auth.RequireAuth(handlers.HandleCardsCreate(database, hub)))
	apiSrv := httptest.NewServer(mux)
	defer apiSrv.Close()

	listener := wsConnect(t, wsSrv)
	waitFor(t, func() bool { return hub.RoomSize(retroID) == 1 })

	sess, err := session.Create(database, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	body := fmt.Sprintf(`{"column_id":%d,"content":"Відмінна робота"}`, colID)
	req, _ := http.NewRequest(
		"POST",
		fmt.Sprintf("%s/retros/%d/cards", apiSrv.URL, retroID),
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST cards: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	msg := readMsg(t, listener)

	if got := msg["type"]; got != "card_created" {
		t.Errorf("expected type=card_created, got %v", got)
	}
	payload, ok := msg["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload object, got %T", msg["payload"])
	}
	if payload["content"] != "Відмінна робота" {
		t.Errorf("expected content=Відмінна робота, got %v", payload["content"])
	}
	if payload["author_name"] != "Аліса Коваль" {
		t.Errorf("expected author_name=Аліса Коваль, got %v", payload["author_name"])
	}
}
