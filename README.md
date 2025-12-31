# Forgor - HackClub Password Manager with LAN Sharing
> Please note: this project was made for HackClub's "HackVault" and is experimental software. While no data leaves your device without your approval, all on-device data is encrypted, and all outgoing connections are E2E encrypted, you are liable for any passwords you choose to store with Forgor.

A terminal-based password manager written in Go with a unique feature: secure end-to-end encrypted password sharing between nearby devices on your network.

## Features

Your vault is encrypted with Argon2id + XChaCha20-Poly1305

### Nearby Device Sharing
- **mDNS based Discovery** - Automatically find other **Forgor** instances on your LAN
- **Fingerprint Pairing** - Verify the devices you connect to before trusting them
- **E2E Encrypted Sharing** - Share entries using NaCl box (public-key encryption)
- **Accept/Decline** - Full control and transparency over incoming shares.
- **Manual Fallback** - When mDNS fails, you can still add devices via IP

### Cloud Sync (Requires a **Forgor Coordination Server**)
- **Self-hosted Sync** - Create or join a vault via a selfhosted coordination server
- **End-to-End Encrypted** - The server never sees plaintext vault data
- **Device Invites** - Invite devices using their 64-hex Device ID
- **Owner Approval** - After a device joins, the owner should run Sync Now once to accept the invite claim
- **Share Carefully** - Invite codes grant access to your vault. Only people on the same coordination server can connect to the same vault

## Installation

### Prerequisites
- Go 1.21 or later

### Build from source
```bash
git clone https://github.com/spjoes/forgor
cd forgor
go build .
```

### Run
```bash
./forgor        # Linux/macOS
forgor.exe      # Windows
```

## Usage

### First Run
1. Launch Forgor
2. Set your master password (minimum 8 characters)
3. Name your device (e.g., "laptop", "desktop")

### Vault Tab (1)
- `↑/↓` or `j/k` - Navigate entries
- `Enter` - View entry details
- `a` - Add new entry
- `e` - Edit entry
- `d` - Delete entry
- `/` - Search
- `u` - Copy username
- `c` - Copy password
- `p` - Toggle password visibility

### Nearby Tab (2)
- See devices running Forgor on your network
- `Enter` or `p` - Initiate pairing
- `m` - Add device manually by IP
- `r` - Refresh discovery

### Friends Tab (3)
- View paired devices
- `s` - Share selected entry (select in Vault first)
- `d` - Remove friend

### Sync Tab (4)
- `Enter` - Select action (Setup Sync, Sync Now, Invite Device, Leave Vault)
- `y` - Copy your Device ID
- `c` - Create a new sync vault (on Setup screen)
- `j` - Join an existing vault (on Setup screen)
- `g` - Generate an invite code (on Invite screen)
- `i` - Copy invite code (on Invite screen)

### Cloud Sync Setup (Requires a Coordination Server)
1. Host a [coordination server](https://github.com/spjoes/forgor-server)
2. Open the Sync tab in your client, select Setup Sync, and enter the server URL
3. Press `c` to create a new vault, or `j` to join using an invite code
4. To invite another device, open Invite Device and enter the target Device ID (shown on that device's Sync tab)
5. The invited device clicks "Setup Sync", enters the coordination server's URL, clicks "j", and enters the invite code given by the owner client
6. The owner runs Sync Now once to accept the invite claim

### Global Keys
- `1/2/3/4` or `Tab` - Switch tabs
- `Ctrl+L` - Lock vault
- `Ctrl+C` - Quit

## Security

- **KDF**: Argon2id (3 iterations, 64MB memory, 4 threads)
- **Vault Encryption**: XChaCha20-Poly1305 (authenticated encryption)
- **Device-to-Device**: NaCl box (Curve25519 + XSalsa20-Poly1305)
- **Storage**: Single encrypted blob in BoltDB (no plaintext on disk)

## Data Storage

> Please note: these are the default database locations. This location can be changed via command arguments when running Forgor

| Platform | Location |
|----------|----------|
| Windows  | `%APPDATA%\forgor\vault.db` |
| macOS    | `~/Library/Application Support/forgor/vault.db` |
| Linux    | `~/.local/share/forgor/vault.db` |

## Network
> Please note: port 8765 is the default port. This port can be changed via command arguments when running Forgor

- **mDNS Service**: `_pwshare._tcp` on port 8765
- **HTTP Endpoints**:
  - `GET /whoami` - Device info for pairing
  - `POST /share` - Receive encrypted entry

## Dependencies

- [BubbleTea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Styling
- [BoltDB](https://github.com/etcd-io/bbolt) - Embedded database
- [hashicorp/mdns](https://github.com/hashicorp/mdns) - mDNS discovery
- [x/crypto](https://pkg.go.dev/golang.org/x/crypto) - Argon2, ChaCha20, NaCl
