package ws

import (
	"bufio"
	"context"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/LEFTEQ/lovinka-deployik/internal/api/middleware"
	"github.com/LEFTEQ/lovinka-deployik/internal/authz"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// MemberContainerResolver resolves the running application container for a
// project in a given environment (and, for preview, a specific branch). It
// returns ("", false, nil) when nothing is live for that target — distinct
// from a real database error. The concrete implementation is
// db.ResolveLiveContainer; the handler takes a function value to keep
// internal/ws free of build/services dependencies.
type MemberContainerResolver func(projectID, environment, branch string) (containerName string, found bool, err error)

// MemberLogsHandler streams `docker logs --follow` of an app member's live
// container to a WebSocket client. It mirrors ServiceLogsHandler (no pub-sub
// Hub; one docker-logs subprocess per connection) but targets the deployed app
// container instead of a sidecar service, resolving (env, branch) → container.
//
// On connect: auth → load project → resolve live container → upgrade WS. Then
// a logs goroutine pipes `docker logs --follow` output through a line scanner
// into the WS as text frames, while a watcher goroutine cancels the context
// (killing the subprocess) when the client disconnects.
func MemberLogsHandler(
	database *db.DB,
	resolve MemberContainerResolver,
	streamLogs ServiceLogsStreamer,
	jwtSecret string,
	allowedOrigins []string,
) http.HandlerFunc {
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

		projectID := r.PathValue("id")
		environment := r.URL.Query().Get("environment")
		branch := r.URL.Query().Get("branch")
		if projectID == "" {
			http.Error(w, "missing project id", http.StatusBadRequest)
			return
		}
		if environment != "preview" && environment != "production" {
			http.Error(w, "invalid environment", http.StatusBadRequest)
			return
		}

		project, err := authz.LoadProject(database, claims, projectID)
		if err != nil || project == nil {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}

		containerName, found, err := resolve(project.ID, environment, branch)
		if err != nil {
			http.Error(w, "failed to resolve container", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "no running container for this target", http.StatusNotFound)
			return
		}

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return middleware.OriginAllowed(r.Header.Get("Origin"), allowedOrigins)
			},
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("MemberLogsHandler: upgrade: %v", err)
			return
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Pipe: streamLogs writes raw output to the writer side; the reader
		// side feeds a bufio.Scanner that emits one WS frame per line.
		pr, pw := io.Pipe()

		go func() {
			defer pw.Close()
			_ = streamLogs(ctx, containerName, pw)
		}()

		// Client-disconnect detector: any read error cancels the context,
		// which stops the docker-logs subprocess.
		go func() {
			defer cancel()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			if err := conn.WriteMessage(websocket.TextMessage, scanner.Bytes()); err != nil {
				return
			}
		}
	}
}
