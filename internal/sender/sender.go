// Package sender implements the LocalSend v2.1 Upload API (section 4).
package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	lscrypto "github.com/localsend-cli/internal/crypto"
	"github.com/localsend-cli/internal/protocol"
	"github.com/schollz/progressbar/v3"
)

// Send sends one or more files to the target device.
// It follows the prepare-upload → upload flow from section 4 of the protocol.
func Send(ctx context.Context, self protocol.Announcement, target protocol.Device, paths []string) error {
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: lscrypto.ClientTLSConfig(),
		},
	}

	scheme := target.Protocol
	if scheme == "" {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s:%d", scheme, target.IP, target.Port)

	// Build file metadata map
	files := make(map[string]protocol.FileInfo, len(paths))
	for i, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("stat %s: %w", p, err)
		}
		id := fmt.Sprintf("file_%d", i)
		files[id] = protocol.FileInfo{
			ID:       id,
			FileName: filepath.Base(p),
			Size:     info.Size(),
			FileType: mimeType(p),
		}
	}

	// 4.1 Prepare upload
	prepReq := protocol.PrepareUploadRequest{
		Info:  self,
		Files: files,
	}
	prepBody, _ := json.Marshal(prepReq)

	prepURL := baseURL + "/api/localsend/v2/prepare-upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, prepURL, bytes.NewReader(prepBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	fmt.Printf("  → Sending transfer request to %s (%s)...\n", target.Alias, target.IP)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("prepare-upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("transfer rejected by %s", target.Alias)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("prepare-upload status %d: %s", resp.StatusCode, string(body))
	}

	var prepResp protocol.PrepareUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&prepResp); err != nil {
		return fmt.Errorf("decode prepare-upload response: %w", err)
	}

	fmt.Printf("  ✓ Transfer accepted (session: %s)\n", prepResp.SessionID)

	// 4.2 Upload each file
	for id, token := range prepResp.Files {
		// Find the path for this file id
		pathIdx := fileIndexFromID(id)
		if pathIdx < 0 || pathIdx >= len(paths) {
			return fmt.Errorf("unknown file id %s in response", id)
		}
		filePath := paths[pathIdx]
		fileInfo := files[id]

		if err := uploadFile(ctx, httpClient, baseURL, prepResp.SessionID, id, token, filePath, fileInfo); err != nil {
			// Cancel the session on error
			_ = cancelSession(httpClient, baseURL, prepResp.SessionID)
			return fmt.Errorf("upload %s: %w", filepath.Base(filePath), err)
		}
	}

	fmt.Println("  ✓ All files sent successfully!")
	return nil
}

func uploadFile(ctx context.Context, client *http.Client, baseURL, sessionID, fileID, token, path string, info protocol.FileInfo) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	uploadURL := fmt.Sprintf("%s/api/localsend/v2/upload?sessionId=%s&fileId=%s&token=%s",
		baseURL,
		url.QueryEscape(sessionID),
		url.QueryEscape(fileID),
		url.QueryEscape(token),
	)

	fmt.Printf("  ↑ Uploading %-30s (%s)\n", info.FileName, humanSize(info.Size))

	bar := progressbar.NewOptions(
		int(info.Size),
		progressbar.OptionSetDescription(info.FileName),
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			fmt.Println()
		}),
		progressbar.OptionThrottle(100*time.Millisecond),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bar)
	if err != nil {
		return err
	}
	req.ContentLength = info.Size
	req.Header.Set("Content-Type", info.FileType)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	fmt.Printf("  ✓ %-30s done\n", info.FileName)
	return nil
}

func cancelSession(client *http.Client, baseURL, sessionID string) error {
	cancelURL := fmt.Sprintf("%s/api/localsend/v2/cancel?sessionId=%s", baseURL, url.QueryEscape(sessionID))
	resp, err := client.Post(cancelURL, "application/json", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func fileIndexFromID(id string) int {
	var idx int
	fmt.Sscanf(id, "file_%d", &idx)
	return idx
}

func mimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	t := mime.TypeByExtension(ext)
	if t == "" {
		return "application/octet-stream"
	}
	return t
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
