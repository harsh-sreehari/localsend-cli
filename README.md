# localsend-cli

A standalone, zero-dependency Command Line Interface (CLI) for [LocalSend](https://localsend.org). Send and receive files directly from your terminal using the LocalSend Protocol v2.1.

## Features

- **Zero Dependencies**: Built with pure Go (stdlib only). No need for Python, Node.js, or the LocalSend GUI.
- **Standalone Binary**: Single executable that runs anywhere.
- **Full Protocol Support**: Implements LocalSend v2.1 (UDP multicast discovery + HTTPS file transfer).
- **Multi-file Support**: Send multiple files in a single session.
- **Smart Receiver**: Dedupes discovery logs and can auto-exit after transfer or stay running (`-k`).
- **Cross-Platform**: Works on Linux, macOS, and Windows.

## Installation

### From Source

```bash
git clone https://github.com/harsh-sreehari/localsend-cli.git
cd localsend-cli
make install
```
*This installs the binary to `~/.local/bin/localsend`. Ensure this directory is in your `$PATH`.*

### Using Go

```bash
go install github.com/harsh-sreehari/localsend-cli/cmd/localsend@latest
```

## Usage

### Sending Files

Discover active devices on your network and send files:

```bash
localsend send photo.jpg
localsend send file1.pdf file2.zip report.docx
```

### Receiving Files

Wait for incoming transfers:

```bash
# Basic receive (prompts y/n for each transfer)
localsend receive

# Auto-accept and save to a specific folder
localsend receive -a --output ~/Downloads/received

# Stay running after a transfer completes
localsend receive -k
```

## Flags

### `send`
- `--alias NAME`: Device name shown to peers (default: hostname)
- `--timeout 3s`: How long to scan for devices

### `receive`
- `--alias NAME`: Device name shown to senders
- `--output DIR`: Where to save received files (default: `~/Downloads`)
- `-a`: Auto-accept all transfers without prompting
- `-k`: Keep running after a transfer finishes

## Technical Details

- **Security**: Uses self-signed ECDSA P-256 certificates for HTTPS, matching the official LocalSend security model. Certificates are cached in `~/.config/localsend-cli/`.
- **Discovery**: Uses UDP multicast on `224.0.0.167:53317`.
- **Protocol**: Implements the official [LocalSend Protocol v2.1](https://github.com/localsend/protocol).

## License

MIT
