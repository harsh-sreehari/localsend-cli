package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	lscrypto "github.com/localsend-cli/internal/crypto"
	"github.com/localsend-cli/internal/discovery"
	"github.com/localsend-cli/internal/protocol"
	"github.com/localsend-cli/internal/receiver"
	"github.com/localsend-cli/internal/sender"
)

const version = "2.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "send":
		cmdSend(os.Args[2:])
	case "receive", "recv":
		cmdReceive(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

// ---------- send ----------

func cmdSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	alias := fs.String("alias", hostname(), "Device alias shown to peers")
	timeout := fs.Duration("timeout", 3*time.Second, "Discovery timeout")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: localsend send [flags] <file> [file ...]")
		fmt.Fprintln(os.Stderr, "\nFlags:")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	files := fs.Args()
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no files specified")
		fs.Usage()
		os.Exit(1)
	}

	// Validate all paths exist before doing anything
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	cert, err := lscrypto.LoadOrGenerateCert()
	if err != nil {
		fatalf("Failed to load cert: %v", err)
	}
	fp := lscrypto.Fingerprint(cert)

	self := protocol.Announcement{
		Alias:       *alias,
		Version:     version,
		DeviceType:  "headless",
		DeviceModel: "localsend-cli",
		Fingerprint: fp,
		Port:        53317,
		Protocol:    "https",
		Download:    false,
		Announce:    true,
	}

	fmt.Printf("🔍 Discovering devices (%.0fs)...\n", timeout.Seconds())
	ctx := context.Background()
	devices := discovery.Discover(ctx, self, cert, *timeout)

	if len(devices) == 0 {
		fmt.Println("No LocalSend devices found on the network.")
		os.Exit(1)
	}

	// Display numbered list
	fmt.Println()
	for i, d := range devices {
		model := d.DeviceModel
		if model == "" {
			model = d.DeviceType
		}
		fmt.Printf("  [%d] %-25s %-15s  %s\n", i+1, d.Alias, d.IP, model)
	}
	fmt.Println()

	target := devices[0]
	if len(devices) > 1 {
		fmt.Printf("Send to which device? [1-%d]: ", len(devices))
		var input string
		fmt.Scanln(&input) //nolint:errcheck
		input = strings.TrimSpace(input)
		n, err := strconv.Atoi(input)
		if err != nil || n < 1 || n > len(devices) {
			fmt.Fprintln(os.Stderr, "Invalid selection")
			os.Exit(1)
		}
		target = devices[n-1]
	} else {
		fmt.Printf("Only device found: %s — sending automatically.\n", target.Alias)
	}

	fmt.Printf("\n📤 Sending %d file(s) to %s (%s)...\n\n", len(files), target.Alias, target.IP)

	// Make paths absolute so the sender works regardless of cwd
	absPaths := make([]string, len(files))
	for i, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			fatalf("Cannot resolve path %s: %v", f, err)
		}
		absPaths[i] = abs
	}

	// Update self to not announce (we're the client now)
	self.Announce = false

	if err := sender.Send(ctx, self, target, absPaths); err != nil {
		fatalf("Send failed: %v", err)
	}
}

// ---------- receive ----------

func cmdReceive(args []string) {
	fs := flag.NewFlagSet("receive", flag.ExitOnError)
	alias := fs.String("alias", hostname(), "Device alias shown to senders")
	output := fs.String("output", defaultDownloads(), "Directory to save received files")
	autoAccept := fs.Bool("a", false, "Auto-accept all incoming transfers without prompting")
	keepAlive := fs.Bool("k", false, "Keep running after a transfer completes (accept multiple)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: localsend receive [flags]")
		fmt.Fprintln(os.Stderr, "\nFlags:")
		fs.PrintDefaults()
	}
	fs.Parse(args) //nolint:errcheck

	cert, err := lscrypto.LoadOrGenerateCert()
	if err != nil {
		fatalf("Failed to load cert: %v", err)
	}
	fp := lscrypto.Fingerprint(cert)

	self := protocol.Announcement{
		Alias:       *alias,
		Version:     version,
		DeviceType:  "headless",
		DeviceModel: "localsend-cli",
		Fingerprint: fp,
		Port:        53317,
		Protocol:    "https",
		Download:    false,
		Announce:    true,
	}

	if *autoAccept {
		fmt.Println("⚡ Auto-accept mode enabled — all transfers will be accepted automatically.")
	}
	if *keepAlive {
		fmt.Println("🔁 Keep-alive mode — will stay running after each transfer.")
	}
	fmt.Printf("📂 Saving files to: %s\n", *output)

	// Announce ourselves periodically so peers keep finding us
	go func() {
		ctx := context.Background()
		for {
			discovery.Announce(ctx, self, cert)
			time.Sleep(5 * time.Second)
		}
	}()

	srv := receiver.New(self, *output, *autoAccept, *keepAlive)
	if err := srv.ListenAndServe(); err != nil {
		fatalf("Server error: %v", err)
	}
}

// ---------- helpers ----------

func printUsage() {
	fmt.Print(`localsend-cli — LocalSend from the terminal

Usage:
  localsend send [flags] <file> [file ...]   Discover devices and send files
  localsend receive [flags]                  Listen for incoming transfers

Send flags:
  --alias NAME     Device name shown to peers  (default: hostname)
  --timeout 3s     Discovery scan duration

Receive flags:
  --alias NAME     Device name shown to senders (default: hostname)
  --output DIR     Where to save received files  (default: ~/Downloads)
  -a               Auto-accept all transfers (no prompt)
  -k               Keep running after a transfer (accept multiple in a row)

Examples:
  localsend send photo.jpg
  localsend send file1.pdf file2.zip report.docx
  localsend receive
  localsend receive -a -k --output /tmp/received
`)
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "localsend-cli"
	}
	return h
}

func defaultDownloads() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
