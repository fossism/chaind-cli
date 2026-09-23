package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// writeJSONError emits a safely-escaped JSON error without leaking
// internal details into broken JSON. Full errors stay server-side in logs.
func writeJSONError(w http.ResponseWriter, code int, err error, public string) {
	if err != nil {
		log.Debug().Err(err).Int("code", code).Msg("IPC request failed")
	}
	if public == "" && err != nil {
		public = "internal error"
	}
	if public == "" {
		public = "request failed"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": public})
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
			writeJSONError(w, http.StatusBadRequest, errGet, "unknown platform")
			return
		}
		ch, err = adp.Watch(r.Context(), room)
	}

	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err, "watch failed")
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
		ScrubMessage(r.Context(), &msg)
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
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("IPC Server panicked and recovered")
		}
	}()

	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "chaind")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
	}

	sockPath := filepath.Join(configDir, "chaind.sock")

	// Remove dead socket if exists
	if _, err := os.Stat(sockPath); err == nil {
		os.Remove(sockPath)
	}

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket %s: %w", sockPath, err)
	}

	if err := os.Chmod(sockPath, 0600); err != nil {
		return fmt.Errorf("failed to secure socket %s: %w", sockPath, err)
	}

	log.Info().Str("socket", sockPath).Msg("IPC Unix Socket API active and listening")

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

	err = s.server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *IPCServer) handleGetRecentMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	msgs, err := s.store.GetRecentMessages(r.Context(), 50)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err, "failed to read messages")
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
	buf    *bytes.Buffer
	status int
	header http.Header
}

func newScrubWriter(w http.ResponseWriter) *scrubWriter {
	return &scrubWriter{ResponseWriter: w, buf: &bytes.Buffer{}, status: http.StatusOK, header: make(http.Header)}
}

func (rw *scrubWriter) Header() http.Header         { return rw.header }
func (rw *scrubWriter) WriteHeader(code int)        { rw.status = code }
func (rw *scrubWriter) Write(p []byte) (int, error) { return rw.buf.Write(p) }

func (rw *scrubWriter) flushTo(w http.ResponseWriter, ctx context.Context) {
	for k, vv := range rw.header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rw.status)
	_, _ = w.Write(ScrubJSON(ctx, rw.buf.Bytes()))
}

// tokenAllowsRoom reports whether tok may act on room.
// Tier 0 and "*" scopes bypass. Empty room never matches a scoped token.
func tokenAllowsRoom(tok *store.Token, room string) bool {
	if tok == nil {
		return false
	}
	if tok.Tier == 0 || tok.Rooms == "*" {
		return true
	}
	if room == "" {
		return false
	}
	for _, ar := range strings.Split(tok.Rooms, ",") {
		if strings.TrimSpace(ar) == room {
			return true
		}
	}
	return false
}

// tokenFromRequest returns the capability token bound by requireToken.
func tokenFromRequest(r *http.Request) *store.Token {
	tok, _ := r.Context().Value(tokenKey).(*store.Token)
	return tok
}

// idBasedPath reports endpoints whose target room is resolved from a
// stored message ID rather than a "room" field. These skip the coarse
// wildcard gate in requireToken; handlers enforce per-message room scope.
func idBasedPath(path string) bool {
	switch path {
	case "/api/v1/messages/reply", "/api/v1/messages/react", "/api/v1/messages/delete",
		"/api/v1/queue/exec", "/api/v1/queue/deny":
		return true
	}
	return false
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

		if tok.IsExpired(time.Now()) {
			http.Error(w, `{"error": "Unauthorized: Token expired"}`, http.StatusUnauthorized)
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

		// ID-based endpoints resolve room from stored state in the handler.
		if idBasedPath(r.URL.Path) {
			// Still bind token below; per-message scope checked by handler.
		} else if reqRoom == "*" || reqRoom == "" {
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
				sw := newScrubWriter(w)
				next.ServeHTTP(sw, r.WithContext(ctx))
				sw.flushTo(w, ctx)
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
		writeJSONError(w, http.StatusInternalServerError, err, "send failed")
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
	if req.MsgID == "" || req.Text == "" {
		http.Error(w, `{"error":"missing id or text"}`, http.StatusBadRequest)
		return
	}
	if orig, err := s.store.GetMessage(r.Context(), req.MsgID); err != nil {
		writeJSONError(w, http.StatusNotFound, err, "message not found")
		return
	} else if !tokenAllowsRoom(tokenFromRequest(r), orig.Room.ID) {
		http.Error(w, `{"error": "Forbidden: Token lacks capability for this room"}`, http.StatusForbidden)
		return
	}

	msg, err := s.router.Reply(req.Platform, req.MsgID, req.Text)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err, "reply failed")
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
	if req.MsgID == "" || req.Emoji == "" {
		http.Error(w, `{"error":"missing id or emoji"}`, http.StatusBadRequest)
		return
	}
	if orig, err := s.store.GetMessage(r.Context(), req.MsgID); err != nil {
		writeJSONError(w, http.StatusNotFound, err, "message not found")
		return
	} else if !tokenAllowsRoom(tokenFromRequest(r), orig.Room.ID) {
		http.Error(w, `{"error": "Forbidden: Token lacks capability for this room"}`, http.StatusForbidden)
		return
	}

	err := s.router.React(req.Platform, req.MsgID, req.Emoji)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err, "react failed")
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
	if req.MsgID == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}
	if orig, err := s.store.GetMessage(r.Context(), req.MsgID); err != nil {
		writeJSONError(w, http.StatusNotFound, err, "message not found")
		return
	} else if !tokenAllowsRoom(tokenFromRequest(r), orig.Room.ID) {
		http.Error(w, `{"error": "Forbidden: Token lacks capability for this room"}`, http.StatusForbidden)
		return
	}

	err := s.router.DeleteMessage(req.Platform, req.MsgID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err, "delete failed")
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
		writeJSONError(w, http.StatusInternalServerError, err, "moderation failed")
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
		if n, err := fmt.Sscanf(limitStr, "%d", &limit); n != 1 || err != nil {
			limit = 20
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	if len(query) > 200 {
		query = query[:200]
	}

	since := r.URL.Query().Get("since")

	msgs, err := s.search.Search(r.Context(), query, limit, since)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err, "search failed")
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
		writeJSONError(w, http.StatusInternalServerError, err, "failed to list queue")
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
	if err := json.Unmarshal([]byte(payloadStr), &req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err, "corrupt queue payload")
		return
	}
	if !tokenAllowsRoom(tokenFromRequest(r), req.RoomID) {
		http.Error(w, `{"error": "Forbidden: Token lacks capability for this room"}`, http.StatusForbidden)
		return
	}

	msg, err := s.router.Send(req.Platform, req.RoomID, req.Text)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err, "queued send failed")
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

	var qRoom string
	if err := s.store.DB().GetContext(r.Context(), &qRoom, "SELECT room_id FROM approval_queue WHERE id = ?", id); err != nil {
		http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
		return
	}
	if !tokenAllowsRoom(tokenFromRequest(r), qRoom) {
		http.Error(w, `{"error": "Forbidden: Token lacks capability for this room"}`, http.StatusForbidden)
		return
	}

	res, err := s.store.DB().ExecContext(r.Context(), "DELETE FROM approval_queue WHERE id = ?", id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err, "failed to deny request")
		return
	}

	affected, _ := res.RowsAffected()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "denied", "deleted": affected})
}
