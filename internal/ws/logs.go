package ws

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/LEFTEQ/lovinka-deployik/internal/api/middleware"
	"github.com/LEFTEQ/lovinka-deployik/internal/authz"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// LogsHandler handles WebSocket connections for build log streaming.
func LogsHandler(hub *Hub, database *db.DB, jwtSecret string, allowedOrigins []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := middleware.ExtractAccessToken(r)
		if tokenStr == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		claims, err := middleware.AuthenticateToken(database, jwtSecret, tokenStr, middleware.IsBearer(r))
		if err != nil || claims == nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		deploymentID := r.PathValue("did")
		if deploymentID == "" {
			http.Error(w, "missing deployment id", http.StatusBadRequest)
			return
		}
		deployment, err := authz.LoadDeployment(database, claims, deploymentID)
		if err != nil {
			http.Error(w, "failed to load deployment", http.StatusInternalServerError)
			return
		}
		if deployment == nil {
			http.Error(w, "deployment not found", http.StatusNotFound)
			return
		}

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return middleware.OriginAllowed(r.Header.Get("Origin"), allowedOrigins)
			},
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
