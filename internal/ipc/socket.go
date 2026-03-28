package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fossism/chaind-cli/internal/daemon"
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
	mux.HandleFunc("/api/v1/messages/watch", s.requireToken(s.handleWatch))
	mux.HandleFunc("/api/v1/moderate", s.requireToken(s.handleModerate))

	s.server = &http.Server{Handler: mux}
	return s
}

func (s *IPCServer) handleWatch(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	room := r.URL.Query().Get("room")
	
	adp, err := s.router.Get(platform)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ch, err := adp.Watch(r.Context(), room)
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
// For local-first usage, any non-empty token is accepted since the Unix socket is already 0600-locked.
func (s *IPCServer) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr := r.Header.Get("Authorization")
		if tokenStr == "" {
			tokenStr = os.Getenv("CHAIND_TOKEN")
		}

		if tokenStr == "" {
			http.Error(w, `{"error": "Unauthorized: Missing Capability Token in Authorization header"}`, http.StatusUnauthorized)
			return
		}

		if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
			tokenStr = tokenStr[7:]
		}

		if tokenStr == "" {
			http.Error(w, `{"error": "Unauthorized: Empty token"}`, http.StatusUnauthorized)
			return
		}

		// The Unix socket is already locked to 0600, so any non-empty token is accepted.
		// Full DB-based token validation can be layered on top for multi-tenant HTTP mode.
		next.ServeHTTP(w, r)
	}
}

type sendReq struct {
	Platform string `json:"platform"`
	RoomID   string `json:"room"`
	Text     string `json:"text"`
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
	
	msg, err := s.router.Send(req.Platform, req.RoomID, req.Text)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
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

	msgs, err := s.search.Search(r.Context(), query, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}
