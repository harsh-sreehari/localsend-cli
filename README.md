# localsend-cli

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go" alt="Go Version">
  <img src="https://img.shields.io/github/v/release/harsh-sreehari/localsend-cli" alt="Release">
  <img src="https://img.shields.io/github/license/harsh-sreehari/localsend-cli" alt="License">
</p>

A standalone, zero-dependency Command Line Interface (CLI) for [LocalSend](https://localsend.org). Send and receive files directly from your terminal using the LocalSend Protocol v2.1 — no GUI required.

## Why CLI?

- **No GUI overhead** — Lightweight, fast, perfect for servers/containers
- **Scriptable** — Integrate into automation scripts, cron jobs, or pipelines
- **Remote-friendly** — SSH into a machine and transfer files without setting up scp/sftp
- **Minimal resources** — Single binary, <10MB, zero runtime dependencies

## Features

- **Zero Dependencies** — Built with pure Go (stdlib only)
- **Full Protocol Support** — Implements LocalSend v2.1 (UDP multicast discovery + HTTPS)
- **Progress Indicators** — Real-time transfer progress with speed and ETA
- **Multi-file Support** — Send multiple files in a single session
- **Smart Receiver** — Auto-accept mode, keep-alive for multiple transfers
- **Cross-Platform** — Linux, macOS, Windows

## Installation

### Homebrew (macOS/Linux)

```bash
brew install harsh-sreehari/tap/localsend-cli
```

### Go Install

```bash
go install github.com/harsh-sreehari/localsend-cli/cmd/localsend@latest
```

### From Source

```bash
git clone https://github.com/harsh-sreehari/localsend-cli.git
cd localsend-cli
make install
```

This installs the binary to `~/.local/bin/localsend`. Ensure this directory is in your `$PATH`.

### Pre-built Binaries

Download the latest release from the [GitHub Releases](https://github.com/harsh-sreehari/localsend-cli/releases) page.

## Quick Start

### Send Files

```bash
# Send a single file
localsend send photo.jpg

# Send multiple files
localsend send file1.pdf file2.zip report.docx

# Send to a specific device with custom alias
localsend send --alias "My-Laptop" document.pdf
```

### Receive Files

```bash
# Basic receive (prompts y/n for each transfer)
localsend receive

# Auto-accept all transfers
localsend receive -a

# Save to specific folder
localsend receive -a --output ~/Downloads/received

# Keep running for multiple transfers
localsend receive -a -k
```

## Usage

```
localsend-cli — LocalSend from the terminal

Usage:
  localsend send [flags] <file> [file ...]   Discover devices and send files
  localsend receive [flags]                  Listen for incoming transfers
  localsend --version                         Show version info
```

### Send Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--alias` | hostname | Device name shown to peers |
| `--timeout` | 3s | How long to scan for devices |

### Receive Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--alias` | hostname | Device name shown to senders |
| `--output` | ~/Downloads | Where to save received files |
| `-a` | false | Auto-accept all transfers |
| `-k` | false | Keep running after transfer completes |

## Examples

### Send files to your phone

```bash
# On your computer
localsend send ~/Photos/vacation.jpg

# Your phone (with LocalSend app) will receive the file
```

### Server-to-server transfer

```bash
# Server A: Start receiver
ssh serverA "localsend receive -a -k"

# Server B: Send files
localsend send --alias "ServerB" backup.tar.gz
```

### Batch file transfer

```bash
# Send all files in a directory
localsend send *.log
localsend send ./dist/*
```

## Technical Details

- **Protocol**: [LocalSend Protocol v2.1](https://github.com/localsend/protocol)
- **Discovery**: UDP multicast on `224.0.0.167:53317`
- **Transfer**: HTTPS with self-signed ECDSA P-256 certificates
- **Certificate Storage**: `~/.config/localsend-cli/`

## Troubleshooting

### "No LocalSend devices found"

- Ensure both devices are on the same network
- Check that LocalSend (or another localsend-cli) is running in receive mode
- Some VPNs can block multicast — try disabling VPN temporarily

### Permission denied when receiving

- Check that the output directory exists and is writable
- Default: `~/Downloads`

### Transfer fails

- Ensure firewall allows traffic on port 53317
- Check that both devices aren't behind strict NATs

## License

MIT — See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please open an issue or submit a PR.
