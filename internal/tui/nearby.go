package tui

import (
	"fmt"
	"strings"

	"forgor/internal/models"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type nearbyMode int

const (
	nearbyModeList nearbyMode = iota
	nearbyModeManual
	nearbyModePairing
)

type NearbyScreen struct {
	peers      []models.Peer
	cursor     int
	mode       nearbyMode
	manualIP   textinput.Model
	pairingPeer *models.Peer
	statusMsg  string
	isError    bool
}

func NewNearbyScreen() NearbyScreen {
	manual := textinput.New()
	manual.Placeholder = "IP:Port (e.g., 192.168.1.50:8765)"
	manual.Width = 40

	return NearbyScreen{
		peers:    []models.Peer{},
		manualIP: manual,
		mode:     nearbyModeList,
	}
}

func (n NearbyScreen) Init() tea.Cmd {
	return nil
}

func (n NearbyScreen) Update(msg tea.Msg) (NearbyScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case PeerFoundMsg:
		found := false
		for i, p := range n.peers {
			if p.Fingerprint == msg.Peer.Fingerprint {
				n.peers[i] = msg.Peer
				found = true
				break
			}
		}
		if !found {
			n.peers = append(n.peers, msg.Peer)
		}
		return n, nil

	case PeerLostMsg:
		newPeers := make([]models.Peer, 0, len(n.peers)-1)
		for _, p := range n.peers {
			if p.Fingerprint != msg.Fingerprint {
				newPeers = append(newPeers, p)
			}
		}
		n.peers = newPeers
		if n.cursor >= len(n.peers) && n.cursor > 0 {
			n.cursor--
		}
		return n, nil

	case PairingCompleteMsg:
		n.mode = nearbyModeList
		n.pairingPeer = nil
		n.statusMsg = fmt.Sprintf("Paired with %s!", msg.Friend.Name)
		n.isError = false
		for i, p := range n.peers {
			if p.Fingerprint == msg.Friend.Fingerprint {
				n.peers[i].IsPaired = true
				break
			}
		}
		return n, nil

	case PairingFailMsg:
		n.mode = nearbyModeList
		n.pairingPeer = nil
		n.statusMsg = "Pairing failed: " + msg.Err.Error()
		n.isError = true
		return n, nil

	case StatusMsg:
		n.statusMsg = msg.Message
		n.isError = msg.IsError
		return n, nil

	case tea.KeyMsg:
		switch n.mode {
		case nearbyModeList:
			return n.updateList(msg)
		case nearbyModeManual:
			return n.updateManual(msg)
		case nearbyModePairing:
			return n.updatePairing(msg)
		}
	}

	return n, nil
}

func (n NearbyScreen) updateList(msg tea.KeyMsg) (NearbyScreen, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if n.cursor > 0 {
			n.cursor--
		}
	case "down", "j":
		if n.cursor < len(n.peers)-1 {
			n.cursor++
		}
	case "enter", "p":
		if len(n.peers) > 0 {
			peer := n.peers[n.cursor]
			if !peer.IsPaired {
				n.pairingPeer = &peer
				n.mode = nearbyModePairing
			} else {
				n.statusMsg = "Already paired with this device"
				n.isError = false
			}
		}
	case "m":
		n.mode = nearbyModeManual
		n.manualIP.Focus()
		return n, textinput.Blink
	case "r":
		return n, func() tea.Msg {
			return RefreshDiscoveryMsg{}
		}
	}
	return n, nil
}

func (n NearbyScreen) updateManual(msg tea.KeyMsg) (NearbyScreen, tea.Cmd) {
	switch msg.String() {
	case "esc":
		n.mode = nearbyModeList
		n.manualIP.SetValue("")
		return n, nil
	case "enter":
		addr := strings.TrimSpace(n.manualIP.Value())
		if addr == "" {
			n.statusMsg = "Please enter an address"
			n.isError = true
			return n, nil
		}
		n.manualIP.SetValue("")
		n.mode = nearbyModeList
		return n, func() tea.Msg {
			return ManualPeerRequestMsg{Address: addr}
		}
	}

	var cmd tea.Cmd
	n.manualIP, cmd = n.manualIP.Update(msg)
	return n, cmd
}

func (n NearbyScreen) updatePairing(msg tea.KeyMsg) (NearbyScreen, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if n.pairingPeer != nil {
			peer := *n.pairingPeer
			return n, func() tea.Msg {
				return ConfirmPairingMsg{Peer: peer}
			}
		}
	case "n", "N", "esc":
		n.mode = nearbyModeList
		n.pairingPeer = nil
	}
	return n, nil
}

func (n NearbyScreen) View() string {
	var b strings.Builder

	switch n.mode {
	case nearbyModeList:
		b.WriteString(n.viewList())
	case nearbyModeManual:
		b.WriteString(n.viewManual())
	case nearbyModePairing:
		b.WriteString(n.viewPairing())
	}

	if n.statusMsg != "" {
		b.WriteString("\n")
		if n.isError {
			b.WriteString(errorStyle.Render("⚠ " + n.statusMsg))
		} else {
			b.WriteString(successStyle.Render("✓ " + n.statusMsg))
		}
	}

	return b.String()
}

func (n NearbyScreen) viewList() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Nearby Devices"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Devices discovered via mDNS"))
	b.WriteString("\n\n")

	if len(n.peers) == 0 {
		b.WriteString(mutedStyle.Render("No devices found. Make sure other devices are running forgor."))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Press 'm' to add a device manually by IP."))
	} else {
		for i, peer := range n.peers {
			cursor := "  "
			style := normalStyle
			if i == n.cursor {
				cursor = "▸ "
				style = selectedStyle
			}

			status := ""
			if peer.IsPaired {
				status = successStyle.Render(" ✓ paired")
			}

			line := fmt.Sprintf("%s%s", cursor, style.Render(peer.Name))
			line += mutedStyle.Render(fmt.Sprintf(" [%s]", peer.Fingerprint[:8]))
			line += status
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate • enter/p pair • m manual add • r refresh"))

	return b.String()
}

func (n NearbyScreen) viewManual() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Add Device Manually"))
	b.WriteString("\n\n")
	b.WriteString("Enter the device's IP and port:\n")
	b.WriteString(focusedInputStyle.Render(n.manualIP.View()))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("enter connect • esc cancel"))

	return boxStyle.Render(b.String())
}

func (n NearbyScreen) viewPairing() string {
	if n.pairingPeer == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("Pair with Device"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Device Name: %s\n", n.pairingPeer.Name))
	b.WriteString(fmt.Sprintf("Fingerprint: %s\n\n", n.pairingPeer.Fingerprint))
	b.WriteString("Verify this fingerprint matches what the other device shows.\n\n")
	b.WriteString("Do you want to pair with this device?\n\n")
	b.WriteString(helpStyle.Render("y confirm • n cancel"))

	return boxStyle.Render(b.String())
}

func (n *NearbyScreen) SetPeers(peers []models.Peer) {
	n.peers = peers
}

func (n *NearbyScreen) UpdatePeerPairedStatus(friends []models.Friend) {
	friendMap := make(map[string]bool)
	for _, f := range friends {
		friendMap[f.Fingerprint] = true
	}
	for i := range n.peers {
		n.peers[i].IsPaired = friendMap[n.peers[i].Fingerprint]
	}
}

func (n NearbyScreen) IsInputActive() bool {
	return n.mode == nearbyModeManual
}

type RefreshDiscoveryMsg struct{}

type ManualPeerRequestMsg struct {
	Address string
}

type ConfirmPairingMsg struct {
	Peer models.Peer
}
