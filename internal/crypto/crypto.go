package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

var configDir = filepath.Join(os.Getenv("HOME"), ".config", "localsend-cli")

const certFile = "cert.pem"
const keyFile = "key.pem"

// LoadOrGenerateCert loads a cached TLS cert/key or generates a new self-signed one.
func LoadOrGenerateCert() (tls.Certificate, error) {
	certPath := filepath.Join(configDir, certFile)
	keyPath := filepath.Join(configDir, keyFile)

	if _, err := os.Stat(certPath); err == nil {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err == nil {
			return cert, nil
		}
	}

	return generateAndSaveCert(certPath, keyPath)
}

func generateAndSaveCert(certPath, keyPath string) (tls.Certificate, error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return tls.Certificate{}, err
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localsend-cli"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(0, 0, 0, 0), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
		IsCA:         true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes, _ := x509.MarshalECPrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// Fingerprint computes SHA-256 of the DER-encoded leaf certificate.
func Fingerprint(cert tls.Certificate) string {
	if len(cert.Certificate) == 0 {
		return "unknown"
	}
	sum := sha256.Sum256(cert.Certificate[0])
	return formatHex(sum[:])
}

func formatHex(b []byte) string {
	const hexchars = "0123456789abcdef"
	buf := make([]byte, len(b)*2)
	for i, v := range b {
		buf[i*2] = hexchars[v>>4]
		buf[i*2+1] = hexchars[v&0xf]
	}
	return string(buf)
}

// ServerTLSConfig returns a tls.Config for the HTTPS server.
func ServerTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
}

// ClientTLSConfig returns a tls.Config that skips cert verification (LocalSend spec).
func ClientTLSConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec
}
