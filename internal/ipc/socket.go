package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"io"

	"github.com/fossism/chaind-cli/internal/daemon"
	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/search"
	"github.com/fossism/chaind-cli/internal/store"
	"github.com/rs/zerolog/log"
)

type IPCServer struct {
	store  *store.Store
	router *daemon.AdapterRouter
	search *search.SearchEngine
	server *http.Server
}

func NewIPCServer(store *store.Store, router *daemon.AdapterRouter) *IPCServer {
	mux := http.NewServeMux()
	
	s := &IPCServer{
		store:  store,
		router: router,
		search: search.NewSearchEngine(store),
	}

	mux.HandleFunc("/api/v1/messages/recent", s.requireToken(s.handleGetRecentMessages))
	mux.HandleFunc("/api/v1/messages/search", s.requireToken(s.handleSearch))
	mux.HandleFunc("/api/v1/adapters/status", s.requireToken(s.handleGetStatus))
	mux.HandleFunc("/api/v1/messages/send", s.requireToken(s.handleSendMessage))
	mux.HandleFunc("/api/v1/messages/reply", s.requireToken(s.handleReply))
	mux.HandleFunc("/api/v1/messages/react", s.requireToken(s.handleReact))
	mux.HandleFunc("/api/v1/messages/delete", s.requireToken(s.handleDeleteMessage))
	mux.HandleFunc("/api/v1/messages/watch", s.requireToken(s.handleWatch))
	mux.HandleFunc("/api/v1/moderate", s.requireToken(s.handleModerate))

	mux.HandleFunc("/api/v1/queue", s.requireToken(s.handleQueueList))
	mux.HandleFunc("/api/v1/queue/exec", s.requireToken(s.handleQueueExec))
	mux.HandleFunc("/api/v1/queue/deny", s.requireToken(s.handleQueueDeny))

	s.server = &http.Server{Handler: mux}
	return s
}

func (s *IPCServer) handleWatch(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	room := r.URL.Query().Get("room")
	
	var ch <-chan schema.Message
	var err error

	if platform == "" {
		ch, err = s.router.WatchAll(r.Context())
	} else {
		adp, errGet := s.router.Get(platform)
		if errGet != nil {
			http.Error(w, errGet.Error(), http.StatusBadRequest)
			return
		}
		ch, err = adp.Watch(r.Context(), room)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	for msg := range ch {
		data, _ := json.Marshal(msg)
		fmt.Fprintf(w, "data: %s\n\n", string(data))
		flusher.Flush()
	}
}

// StartOnListener serves on the provided listener. Used by tests to bind to a temp Unix socket.
func (s *IPCServer) StartOnListener(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		s.server.Shutdown(context.Background())
	}()
	return s.server.Serve(ln)
}

func (s *IPCServer) Start(ctx context.Context) error {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "chaind")
	os.MkdirAll(configDir, 0700)
	
	sockPath := filepath.Join(configDir, "chaind.sock")
	
	// Remove dead socket if exists
	os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", sockPath, err)
	}

	if err := os.Chmod(sockPath, 0600); err != nil {
		return fmt.Errorf("failed to secure socket %s: %w", sockPath, err)
	}

	log.Info().Str("socket", sockPath).Msg("IPC Unix Socket API active")

	// Setup HTTP listener if Docker/prefer_http is requested
	if os.Getenv("CHAIND_PREFER_HTTP") == "true" {
		go func() {
			httpPort := os.Getenv("CHAIND_HTTP_PORT")
			if httpPort == "" {
				httpPort = "7432"
			}
			addr := ":" + httpPort
			log.Info().Str("addr", addr).Msg("HTTP IPC API mirror active")
			if err := http.ListenAndServe(addr, s.server.Handler); err != nil && err != http.ErrServerClosed {
				log.Error().Err(err).Msg("HTTP IPC stopped")
			}
		}()
	}

	go func() {
		<-ctx.Done()
		log.Info().Msg("Shutting down IPC server...")
		s.server.Shutdown(context.Background())
		os.Remove(sockPath)
	}()

	return s.server.Serve(listener)
}

func (s *IPCServer) handleGetRecentMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	msgs, err := s.store.GetRecentMessages(r.Context(), 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

func (s *IPCServer) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"daemon": "running",
		"store":  "connected",
	})
}

// requireToken is an interception middleware ensuring the request has a valid Capability Token.
type contextKey string
const tokenKey contextKey = "ipc_token"

type scrubWriter struct {
	http.ResponseWriter
	buf *bytes.Buffer
}

func (rw *scrubWriter) Write(p []byte) (int, error) {
	return rw.buf.Write(p)
}

func (s *IPCServer) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.Header.Get("Authorization")
		if tokenStr == "" {
			tokenStr = os.Getenv("CHAIND_TOKEN")
		}

		if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
			tokenStr = tokenStr[7:]
		}

		if tokenStr == "" {
			http.Error(w, `{"error": "Unauthorized: Missing Capability Token in Authorization header"}`, http.StatusUnauthorized)
			return
		}

		tok, err := s.store.GetToken(r.Context(), tokenStr)
		if err != nil || tok == nil {
			http.Error(w, `{"error": "Unauthorized: Invalid token metadata"}`, http.StatusUnauthorized)
			return
		}
		
		if tok.Revoked {
			http.Error(w, `{"error": "Unauthorized: Token revoked"}`, http.StatusUnauthorized)
			return
		}

		// Peek request room
		var reqRoom string
		if r.Method == http.MethodGet {
			reqRoom = r.URL.Query().Get("room")
		} else if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			var peek map[string]interface{}
			if err := json.Unmarshal(body, &peek); err == nil {
				if v, ok := peek["room"].(string); ok {
					reqRoom = v
				}
			}
		}

		// Validate wildcard
		if reqRoom == "*" || reqRoom == "" {
			if tok.Tier != 0 {
				http.Error(w, `{"error": "Forbidden: Tier 0 admin required for wildcard/global access"}`, http.StatusForbidden)
				return
			}
		} else if reqRoom != "" {
			allowed := false
			if tok.Tier == 0 || tok.Rooms == "*" {
				allowed = true
			} else {
				allowedRooms := strings.Split(tok.Rooms, ",")
				for _, ar := range allowedRooms {
					if strings.TrimSpace(ar) == reqRoom {
						allowed = true
						break
					}
				}
			}
			if !allowed {
				http.Error(w, `{"error": "Forbidden: Token lacks capability for this room"}`, http.StatusForbidden)
				return
			}
		}

		ctx := context.WithValue(r.Context(), tokenKey, tok)

		if tok.PiiScrub != "" && r.Method == http.MethodGet {
			if r.URL.Path == "/api/v1/messages/recent" || r.URL.Path == "/api/v1/messages/search" {
				sw := &scrubWriter{ResponseWriter: w, buf: &bytes.Buffer{}}
				next.ServeHTTP(sw, r.WithContext(ctx))
				
				out := sw.buf.Bytes()
				pattern, err := regexp.Compile(tok.PiiScrub)
				if err == nil {
					out = pattern.ReplaceAll(out, []byte("[REDACTED PII]"))
				}
				w.Write(out)
				return
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

type sendReq struct {
	Platform        string `json:"platform"`
	RoomID          string `json:"room"`
	Text            string `json:"text"`
	RequireApproval bool   `json:"require_approval"`
}

func (s *IPCServer) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req sendReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	
	if req.RequireApproval {
		payloadBytes, _ := json.Marshal(req)
		id := "queue_" + time.Now().Format("20060102150405")
		_, err := s.store.DB().ExecContext(r.Context(), "INSERT INTO approval_queue (id, action_type, platform, room_id, payload, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))", id, "send", req.Platform, req.RoomID, string(payloadBytes))
		if err != nil {
			http.Error(w, `{"error": "Failed to enqueue"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "queued for approval", "id": id})
		return
	}

	msg, err := s.router.Send(req.Platform, req.RoomID, req.Text)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

type replyReq struct {
	Platform string `json:"platform"`
	MsgID    string `json:"id"`
	Text     string `json:"text"`
}

func (s *IPCServer) handleReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req replyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	msg, err := s.router.Reply(req.Platform, req.MsgID, req.Text)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

type reactReq struct {
	Platform string `json:"platform"`
	MsgID    string `json:"id"`
	Emoji    string `json:"emoji"`
}

func (s *IPCServer) handleReact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req reactReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	err := s.router.React(req.Platform, req.MsgID, req.Emoji)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "reacted"})
}

type deleteReq struct {
	Platform string `json:"platform"`
	MsgID    string `json:"id"`
}

func (s *IPCServer) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req deleteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	err := s.router.DeleteMessage(req.Platform, req.MsgID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

type modReq struct {
	Platform string `json:"platform"`
	RoomID   string `json:"room"`
	UserID   string `json:"user"`
	Reason   string `json:"reason"`
}

func (s *IPCServer) handleModerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req modReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	err := s.router.Ban(req.Platform, req.RoomID, req.UserID, req.Reason)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "moderated",
		"target": req.UserID,
	})
}

func (s *IPCServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, `{"error":"missing query parameter 'q'"}`, http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	since := r.URL.Query().Get("since")

	msgs, err := s.search.Search(r.Context(), query, limit, since)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

func (s *IPCServer) handleQueueList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var items []map[string]interface{}
	err := s.store.DB().SelectContext(r.Context(), &items, "SELECT id, action_type, platform, room_id, payload, created_at FROM approval_queue ORDER BY created_at ASC")
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (s *IPCServer) handleQueueExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, `{"error":"Missing id parameter"}`, http.StatusBadRequest)
		return
	}

	var payloadStr string
	err := s.store.DB().GetContext(r.Context(), &payloadStr, "SELECT payload FROM approval_queue WHERE id = ?", id)
	if err != nil {
		http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
		return
	}

	var req sendReq
	json.Unmarshal([]byte(payloadStr), &req)

	msg, err := s.router.Send(req.Platform, req.RoomID, req.Text)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	
	s.store.DB().ExecContext(r.Context(), "DELETE FROM approval_queue WHERE id = ?", id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

func (s *IPCServer) handleQueueDeny(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, `{"error":"Missing id parameter"}`, http.StatusBadRequest)
		return
	}
	
	res, err := s.store.DB().ExecContext(r.Context(), "DELETE FROM approval_queue WHERE id = ?", id)
	if err != nil {
		http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	affected, _ := res.RowsAffected()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "denied", "deleted": affected})
}
