package ws

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// LogsHandler handles WebSocket connections for build log streaming.
func LogsHandler(hub *Hub, jwtSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Authenticate via query param
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		_, err := auth.ValidateAccessToken(jwtSecret, tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		deploymentID := r.PathValue("did")
		if deploymentID == "" {
			http.Error(w, "missing deployment id", http.StatusBadRequest)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// Subscribe to log lines
		ch := hub.Subscribe(deploymentID)
		defer hub.Unsubscribe(deploymentID, ch)

		// Read goroutine (handles client disconnect)
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		// Write log lines to WebSocket
		for {
			select {
			case line, ok := <-ch:
				if !ok {
					return
				}
				if err := conn.WriteJSON(line); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}
}
