// Package discovery implements LocalSend v2.1 device discovery via UDP multicast
// and a minimal HTTPS /register endpoint.
package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	lscrypto "github.com/localsend-cli/internal/crypto"
	"github.com/localsend-cli/internal/protocol"
)

const (
	multicastAddr = "224.0.0.167"
	port          = 53317
	apiVersion    = "v2"
)

// Discover sends a multicast announcement and collects responding devices
// for the given duration. It also starts an ephemeral HTTPS register server
// to capture TCP responses.
func Discover(ctx context.Context, self protocol.Announcement, cert tls.Certificate, timeout time.Duration) []protocol.Device {
	var mu sync.Mutex
	seen := map[string]bool{self.Fingerprint: true} // skip self
	var devices []protocol.Device

	addDevice := func(d protocol.Device) {
		mu.Lock()
		defer mu.Unlock()
		if seen[d.Fingerprint] {
			return
		}
		seen[d.Fingerprint] = true
		devices = append(devices, d)
	}

	// --- Ephemeral HTTPS register server (TCP response path) ---
	mux := http.NewServeMux()
	mux.HandleFunc("/api/localsend/"+apiVersion+"/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var peer protocol.Device
		if err := json.NewDecoder(r.Body).Decode(&peer); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		peer.IP = extractIP(r.RemoteAddr)
		addDevice(peer)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(self)
	})

	srv := &http.Server{
		Addr:      fmt.Sprintf(":%d", port),
		Handler:   mux,
		TLSConfig: lscrypto.ServerTLSConfig(cert),
	}

	go func() {
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			// Port busy (receive mode running). UDP path still works.
			log.Printf("[discovery] HTTPS register server: %v", err)
		}
	}()
	defer srv.Shutdown(context.Background()) //nolint:errcheck

	// --- UDP multicast announcement ---
	sendAnnouncement(self)

	// --- UDP multicast listener (fallback UDP responses) ---
	udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multicastAddr, port))
	if err == nil {
		conn, err := net.ListenMulticastUDP("udp4", bestInterface(), udpAddr)
		if err == nil {
			conn.SetReadDeadline(time.Now().Add(timeout))
			go func() {
				buf := make([]byte, 4096)
				for {
					n, src, err := conn.ReadFromUDP(buf)
					if err != nil {
						return
					}
					var peer protocol.Device
					if err := json.Unmarshal(buf[:n], &peer); err != nil {
						continue
					}
					if peer.Announce { // only response messages have announce==false
						continue
					}
					peer.IP = src.IP.String()
					addDevice(peer)
				}
			}()
			defer conn.Close()
		}
	}

	select {
	case <-ctx.Done():
	case <-time.After(timeout):
	}

	return devices
}

// Announce sends a single UDP multicast announcement.
// Call this periodically in receive mode so this device stays visible to peers.
func Announce(ctx context.Context, self protocol.Announcement, cert tls.Certificate) {
	sendAnnouncement(self)
}

func sendAnnouncement(self protocol.Announcement) {
	payload, err := json.Marshal(self)
	if err != nil {
		return
	}
	conn, err := net.Dial("udp", fmt.Sprintf("%s:%d", multicastAddr, port))
	if err != nil {
		log.Printf("[discovery] UDP dial: %v", err)
		return
	}
	defer conn.Close()
	_, _ = conn.Write(payload)
}

func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func bestInterface() *net.Interface {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if _, ok := a.(*net.IPNet); ok {
				return &iface
			}
		}
	}
	return nil
}
