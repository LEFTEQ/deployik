package ws

import (
	"bufio"
	"context"
	"io"
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/LEFTEQ/lovinka-deployik/internal/api/middleware"
	"github.com/LEFTEQ/lovinka-deployik/internal/auth"
	"github.com/LEFTEQ/lovinka-deployik/internal/authz"
	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

// ServiceSpecLookup resolves the running container for a (project, env,
// service-type) tuple. It returns (nil, nil) when no service is attached —
// distinct from a real database / decryption error. The concrete
// implementation lives in internal/services (Manager.GetSpec); the WS handler
// takes a function value to avoid an internal/ws → internal/services →
// internal/build → internal/ws import cycle (the build package already
// imports ws for its log hub).
type ServiceSpecLookup func(project *db.Project, environment string, svcType db.ServiceType) (containerName string, found bool, err error)

// ServiceLogsStreamer runs `docker logs --follow` for the given container
// name and copies its combined stdout+stderr into w until ctx is cancelled or
// the process exits. The concrete implementation lives in internal/services
// (services.Logs).
type ServiceLogsStreamer func(ctx context.Context, containerName string, w io.Writer) error

// ServiceLogsHandler streams `docker logs --follow` of a project's sidecar
// container (postgres in v1) to a WebSocket client. Unlike LogsHandler /
// DomainLogsHandler, there's no pub-sub Hub here — there's only ever one
// consumer per (project, env) at a time (the dashboard's Logs panel), and the
// source is a `docker logs --follow` child process spawned per connection.
//
// On connect: auth → load project → resolve service spec → upgrade WS.
// Then start two goroutines:
//   - reader: pumps the docker-logs output through a pipe into the WS as
//     text frames (line-buffered for readability)
//   - watcher: reads from the WS (only to detect client disconnect) and
//     cancels the context, which kills the docker-logs subprocess.
func ServiceLogsHandler(
	database *db.DB,
	lookupSpec ServiceSpecLookup,
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
		claims, err := auth.ValidateAccessToken(jwtSecret, tokenStr)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		projectID := r.PathValue("id")
		environment := r.PathValue("env")
		if projectID == "" || environment == "" {
			http.Error(w, "missing project id or environment", http.StatusBadRequest)
			return
		}

		project, err := authz.LoadProject(database, claims, projectID)
		if err != nil || project == nil {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}

		containerName, found, err := lookupSpec(project, environment, db.ServiceTypePostgres)
		if err != nil {
			http.Error(w, "failed to load service spec", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "no service attached for this environment", http.StatusNotFound)
			return
		}

		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return middleware.OriginAllowed(r.Header.Get("Origin"), allowedOrigins)
			},
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ServiceLogsHandler: upgrade: %v", err)
			return
		}
		defer conn.Close()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Pipe: streamLogs writes raw output to the writer side; the reader
		// side feeds a bufio.Scanner that emits one WS frame per line.
		pr, pw := io.Pipe()

		// Logs goroutine: runs `docker logs --follow` until ctx is cancelled
		// (client disconnect) or the container exits.
		go func() {
			defer pw.Close()
			_ = streamLogs(ctx, containerName, pw)
		}()

		// Client-disconnect detector: read in a loop; on any read error
		// (client closed the WS), cancel the context to stop the logs stream.
		go func() {
			defer cancel()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		// Line pump: scan stdout/stderr of the docker logs subprocess and
		// forward each line to the WS as a text frame.
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			if err := conn.WriteMessage(websocket.TextMessage, scanner.Bytes()); err != nil {
				return
			}
		}
	}
}
