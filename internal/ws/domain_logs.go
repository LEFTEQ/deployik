package ws

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/LEFTEQ/lovinka-deployik/internal/api/middleware"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/authz"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// DomainLogsHandler handles WebSocket connections for domain verification log streaming.
func DomainLogsHandler(hub *Hub, database *db.DB, jwtSecret string, allowedOrigins []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := middleware.ExtractAccessToken(r)
		if tokenStr == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}

		claims, err := auth.ValidateAccessToken(jwtSecret, tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		domainID := r.PathValue("did")
		if domainID == "" {
			http.Error(w, "missing domain id", http.StatusBadRequest)
			return
		}

		// Authorize: find the domain, then check the user can access its project
		domain, err := database.GetDomainByID(domainID)
		if err != nil {
			http.Error(w, "failed to load domain", http.StatusInternalServerError)
			return
		}
		if domain == nil {
			http.Error(w, "domain not found", http.StatusNotFound)
			return
		}

		project, err := authz.LoadProject(database, claims, domain.ProjectID)
		if err != nil {
			http.Error(w, "failed to load project", http.StatusInternalServerError)
			return
		}
		if project == nil {
			http.Error(w, "project not found", http.StatusNotFound)
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

		topic := "domain:" + domainID
		ch := hub.Subscribe(topic)
		defer hub.Unsubscribe(topic, ch)

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
