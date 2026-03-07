// Package receiver implements the LocalSend v2.1 receiver (Upload API server).
package receiver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	lscrypto "github.com/localsend-cli/internal/crypto"
	"github.com/localsend-cli/internal/protocol"
)

const port = 53317

// session tracks an in-progress upload session.
type session struct {
	files     map[string]protocol.FileInfo
	tokens    map[string]string
	remaining int // files not yet uploaded
}

// Server is the LocalSend receiver HTTPS server.
type Server struct {
	Self       protocol.Announcement
	OutputDir  string
	AutoAccept bool
	KeepAlive  bool // if false, exit after first completed transfer

	mu          sync.Mutex
	sessions    map[string]*session
	seenPeers   map[string]bool // fingerprint -> logged already
	done        chan struct{}
}

// New creates a Server ready to call ListenAndServe.
func New(self protocol.Announcement, outputDir string, autoAccept, keepAlive bool) *Server {
	return &Server{
		Self:       self,
		OutputDir:  outputDir,
		AutoAccept: autoAccept,
		KeepAlive:  keepAlive,
		sessions:   make(map[string]*session),
		seenPeers:  make(map[string]bool),
		done:       make(chan struct{}),
	}
}

// ListenAndServe starts the HTTPS server; blocks until done or error.
func (s *Server) ListenAndServe() error {
	cert, err := lscrypto.LoadOrGenerateCert()
	if err != nil {
		return fmt.Errorf("load cert: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/localsend/v2/register", s.handleRegister)
	mux.HandleFunc("/api/localsend/v2/prepare-upload", s.handlePrepareUpload)
	mux.HandleFunc("/api/localsend/v2/upload", s.handleUpload)
	mux.HandleFunc("/api/localsend/v2/cancel", s.handleCancel)
	mux.HandleFunc("/api/localsend/v2/info", s.handleInfo)

	srv := &http.Server{
		Addr:      fmt.Sprintf(":%d", port),
		Handler:   mux,
		TLSConfig: lscrypto.ServerTLSConfig(cert),
	}

	fmt.Printf("Listening on https://0.0.0.0:%d  (alias: %s)\n", port, s.Self.Alias)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for transfer completion or server error
	select {
	case <-s.done:
		srv.Shutdown(context.Background()) //nolint:errcheck
		return nil
	case err := <-errCh:
		return err
	}
}

// handleRegister responds to peer registration — logs each peer only once.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var peer protocol.Device
	if err := json.NewDecoder(r.Body).Decode(&peer); err == nil {
		peer.IP = remoteIP(r)
		s.mu.Lock()
		first := !s.seenPeers[peer.Fingerprint]
		s.seenPeers[peer.Fingerprint] = true
		s.mu.Unlock()
		if first {
			fmt.Printf("  ↔  Discovered: %s (%s)\n", peer.Alias, peer.IP)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.Self)
}

// handleInfo returns our device info (section 6.1, legacy).
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.Self)
}

// handlePrepareUpload handles section 4.1 – decides whether to accept.
func (s *Server) handlePrepareUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req protocol.PrepareUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	senderAlias := req.Info.Alias
	senderIP := remoteIP(r)

	fileLines := make([]string, 0, len(req.Files))
	for _, f := range req.Files {
		fileLines = append(fileLines, fmt.Sprintf("  • %-30s %s", f.FileName, humanSize(f.Size)))
	}

	accept := s.AutoAccept
	if !accept {
		fmt.Printf("\n📥 Incoming transfer from %s (%s):\n%s\n",
			senderAlias, senderIP, strings.Join(fileLines, "\n"))
		fmt.Print("Accept? [y/N]: ")
		var ans string
		fmt.Scanln(&ans) //nolint:errcheck
		accept = strings.ToLower(strings.TrimSpace(ans)) == "y"
	} else {
		fmt.Printf("\n📥 Auto-accepting transfer from %s (%s):\n%s\n",
			senderAlias, senderIP, strings.Join(fileLines, "\n"))
	}

	if !accept {
		writeError(w, http.StatusForbidden, "rejected by user")
		return
	}

	sessionID := randomHex(16)
	tokens := make(map[string]string, len(req.Files))
	for id := range req.Files {
		tokens[id] = randomHex(16)
	}

	s.mu.Lock()
	s.sessions[sessionID] = &session{
		files:     req.Files,
		tokens:    tokens,
		remaining: len(req.Files),
	}
	s.mu.Unlock()

	resp := protocol.PrepareUploadResponse{SessionID: sessionID, Files: tokens}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleUpload handles section 4.2 – receives raw file bytes.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	sessionID := q.Get("sessionId")
	fileID := q.Get("fileId")
	token := q.Get("token")

	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	s.mu.Unlock()

	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	expectedToken := sess.tokens[fileID]
	if expectedToken == "" || expectedToken != token {
		writeError(w, http.StatusForbidden, "invalid token")
		return
	}

	fileInfo, ok := sess.files[fileID]
	if !ok {
		writeError(w, http.StatusNotFound, "file not found in session")
		return
	}

	if err := os.MkdirAll(s.OutputDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "cannot create output dir")
		return
	}

	outPath := uniquePath(filepath.Join(s.OutputDir, sanitize(fileInfo.FileName)))
	f, err := os.Create(outPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot create file")
		return
	}
	defer f.Close()

	fmt.Printf("  ↓ Receiving %-40s → %s\n", fileInfo.FileName, outPath)
	n, err := io.Copy(f, r.Body)
	if err != nil {
		fmt.Printf("  ✗ Error receiving %s: %v\n", fileInfo.FileName, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fmt.Printf("  ✓ Saved %-40s (%s)\n", fileInfo.FileName, humanSize(n))
	w.WriteHeader(http.StatusOK)

	// Decrement remaining; signal done when session is complete
	s.mu.Lock()
	sess.remaining--
	allDone := sess.remaining == 0
	s.mu.Unlock()

	if allDone && !s.KeepAlive {
		fmt.Println("\n✅ Transfer complete.")
		close(s.done)
	}
}

// handleCancel handles section 4.3.
func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
	fmt.Printf("  ✗ Session %s cancelled by sender\n", sessionID)
	w.WriteHeader(http.StatusOK)
}

// --- helpers ---

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(protocol.ErrorResponse{Message: msg})
}

func remoteIP(r *http.Request) string {
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

func sanitize(name string) string { return filepath.Base(name) }

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
