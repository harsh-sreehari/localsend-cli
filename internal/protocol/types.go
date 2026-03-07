// Package protocol defines shared types for the LocalSend Protocol v2.1.
package protocol

// Device represents a discovered LocalSend peer.
type Device struct {
	Alias       string `json:"alias"`
	Version     string `json:"version"`
	DeviceModel string `json:"deviceModel,omitempty"`
	DeviceType  string `json:"deviceType,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"` // "http" | "https"
	Download    bool   `json:"download,omitempty"`
	Announce    bool   `json:"announce,omitempty"`

	// Filled in locally after discovery, not part of JSON payload
	IP string `json:"-"`
}

// Announcement is the UDP multicast message sent to discover peers.
type Announcement struct {
	Alias       string `json:"alias"`
	Version     string `json:"version"`
	DeviceModel string `json:"deviceModel,omitempty"`
	DeviceType  string `json:"deviceType,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	Download    bool   `json:"download"`
	Announce    bool   `json:"announce"`
}

// FileInfo is the metadata for a single file in a prepare-upload request.
type FileInfo struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	FileType string `json:"fileType"`
	SHA256   string `json:"sha256,omitempty"`
}

// PrepareUploadRequest is the body sent to /api/localsend/v2/prepare-upload.
type PrepareUploadRequest struct {
	Info  Announcement        `json:"info"`
	Files map[string]FileInfo `json:"files"`
}

// PrepareUploadResponse is the response from /api/localsend/v2/prepare-upload.
type PrepareUploadResponse struct {
	SessionID string            `json:"sessionId"`
	Files     map[string]string `json:"files"` // fileId -> token
}

// ErrorResponse is a generic API error body.
type ErrorResponse struct {
	Message string `json:"message"`
}
