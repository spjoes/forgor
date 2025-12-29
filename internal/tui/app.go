package tui

import (
	"fmt"
	"net"
	"strings"
	"time"

	"forgor/internal/clipboard"
	"forgor/internal/models"
	"forgor/internal/server"
	"forgor/internal/storage"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Tab int

const (
	TabVault Tab = iota
	TabNearby
	TabFriends
)

var tabNames = []string{"Vault", "Nearby", "Friends"}

type App struct {
	store          *storage.Store
	isLocked       bool
	isNewVault     bool
	activeTab      Tab
	width          int
	height         int

	lockScreen     LockScreen
	vaultScreen    VaultScreen
	nearbyScreen   NearbyScreen
	friendsScreen  FriendsScreen
	incomingScreen IncomingShareScreen

	device         *models.Device
	localAddr      string

	peerAddresses  map[string]string

	peerChan       chan models.Peer
	shareChan      chan models.IncomingShare

	statusMsg      string
	statusIsError  bool
}

func NewApp(store *storage.Store, peerChan chan models.Peer, shareChan chan models.IncomingShare, port int) *App {
	isNew := !store.IsInitialized()
	localAddr := fmt.Sprintf("%s:%d", getOutboundIP(), port)
	
	return &App{
		store:          store,
		isLocked:       true,
		isNewVault:     isNew,
		lockScreen:     NewLockScreen(isNew),
		nearbyScreen:   NewNearbyScreen(),
		friendsScreen:  NewFriendsScreen(),
		incomingScreen: NewIncomingShareScreen(),
		peerChan:       peerChan,
		shareChan:      shareChan,
		localAddr:      localAddr,
		peerAddresses:  make(map[string]string),
	}
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{
		a.lockScreen.Init(),
		a.listenForPeers(),
		a.listenForShares(),
	}
	return tea.Batch(cmds...)
}

func (a *App) listenForPeers() tea.Cmd {
	return func() tea.Msg {
		if a.peerChan == nil {
			return nil
		}
		peer, ok := <-a.peerChan
		if !ok {
			return nil
		}
		return PeerFoundMsg{Peer: peer}
	}
}

func (a *App) listenForShares() tea.Cmd {
	return func() tea.Msg {
		if a.shareChan == nil {
			return nil
		}
		share, ok := <-a.shareChan
		if !ok {
			return nil
		}
		return IncomingShareMsg{Share: share}
	}
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			a.store.Lock()
			return a, tea.Quit
		case "ctrl+l":
			if !a.isLocked {
				a.store.Lock()
				a.isLocked = true
				a.isNewVault = false
				a.lockScreen = NewLockScreen(false)
				return a, nil
			}
		}

		if !a.isLocked && !a.incomingScreen.IsVisible() && !a.isInputActive() {
			switch msg.String() {
			case "1":
				a.activeTab = TabVault
				return a, nil
			case "2":
				a.activeTab = TabNearby
				return a, nil
			case "3":
				a.activeTab = TabFriends
				return a, nil
			case "tab":
				a.activeTab = Tab((int(a.activeTab) + 1) % 3)
				return a, nil
			}
		}

	case UnlockRequestMsg:
		entries, err := a.store.Unlock(msg.Password)
		if err != nil {
			a.lockScreen.SetError("Invalid password")
			return a, nil
		}
		return a.handleUnlock(entries)

	case InitRequestMsg:
		if err := a.store.Initialize(msg.Password, msg.DeviceName); err != nil {
			a.lockScreen.SetError("Failed to initialize: " + err.Error())
			return a, nil
		}
		entries, err := a.store.Unlock(msg.Password)
		if err != nil {
			a.lockScreen.SetError("Failed to unlock: " + err.Error())
			return a, nil
		}
		return a.handleUnlock(entries)

	case SaveEntriesMsg:
		if err := a.store.SaveEntries(msg.Entries); err != nil {
			a.statusMsg = "Failed to save: " + err.Error()
			a.statusIsError = true
		} else {
			a.vaultScreen.SetEntries(msg.Entries)
		}
		return a, nil

	case CopyToClipboardMsg:
		return a, a.copyToClipboard(msg.Text, msg.Label)

	case PeerFoundMsg:
		friends, _ := a.store.GetAllFriends()
		for _, f := range friends {
			if f.Fingerprint == msg.Peer.Fingerprint {
				msg.Peer.IsPaired = true
				break
			}
		}
		if msg.Peer.Host != "" && msg.Peer.Port != 0 {
			a.peerAddresses[msg.Peer.Fingerprint] = fmt.Sprintf("%s:%d", msg.Peer.Host, msg.Peer.Port)
		}
		a.nearbyScreen, _ = a.nearbyScreen.Update(msg)
		return a, a.listenForPeers()

	case ManualPeerRequestMsg:
		return a, a.handleManualPeer(msg.Address)

	case StatusMsg:
		switch a.activeTab {
		case TabVault:
			a.vaultScreen, _ = a.vaultScreen.Update(msg)
		case TabNearby:
			a.nearbyScreen, _ = a.nearbyScreen.Update(msg)
		case TabFriends:
			a.friendsScreen, _ = a.friendsScreen.Update(msg)
		}
		return a, nil

	case ConfirmPairingMsg:
		return a, a.handlePairing(msg.Peer)

	case PairingCompleteMsg:
		a.nearbyScreen, _ = a.nearbyScreen.Update(msg)
		friends, _ := a.store.GetAllFriends()
		a.friendsScreen.SetFriends(friends)
		return a, nil

	case PairingFailMsg:
		a.nearbyScreen, _ = a.nearbyScreen.Update(msg)
		return a, nil

	case SendShareMsg:
		return a, a.handleSendShare(msg.Friend, msg.Entry)

	case ShareSentMsg:
		a.friendsScreen, _ = a.friendsScreen.Update(msg)
		return a, nil

	case ShareFailMsg:
		a.friendsScreen, _ = a.friendsScreen.Update(msg)
		return a, nil

	case IncomingShareMsg:
		a.incomingScreen, _ = a.incomingScreen.Update(msg)
		cmds = append(cmds, a.listenForShares())
		return a, tea.Batch(cmds...)

	case AcceptShareMsg:
		return a, a.handleAcceptShare(msg.Share)

	case DeleteFriendMsg:
		if err := a.store.DeleteFriend(msg.Fingerprint); err != nil {
			a.friendsScreen.statusMsg = "Failed to remove: " + err.Error()
			a.friendsScreen.isError = true
		} else {
			friends, _ := a.store.GetAllFriends()
			a.friendsScreen.SetFriends(friends)
			a.nearbyScreen.UpdatePeerPairedStatus(friends)
		}
		a.friendsScreen.mode = friendsModeList
		return a, nil

	case RefreshFriendsMsg:
		a.friendsScreen.SetFriends(msg.Friends)
		return a, nil
	}

	if a.isLocked {
		var cmd tea.Cmd
		a.lockScreen, cmd = a.lockScreen.Update(msg)
		cmds = append(cmds, cmd)
	} else if a.incomingScreen.IsVisible() {
		var cmd tea.Cmd
		a.incomingScreen, cmd = a.incomingScreen.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		switch a.activeTab {
		case TabVault:
			var cmd tea.Cmd
			a.vaultScreen, cmd = a.vaultScreen.Update(msg)
			cmds = append(cmds, cmd)
			a.friendsScreen.SetSelectedEntry(a.vaultScreen.GetSelectedEntry())
		case TabNearby:
			var cmd tea.Cmd
			a.nearbyScreen, cmd = a.nearbyScreen.Update(msg)
			cmds = append(cmds, cmd)
		case TabFriends:
			var cmd tea.Cmd
			a.friendsScreen, cmd = a.friendsScreen.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return a, tea.Batch(cmds...)
}

func (a *App) handleUnlock(entries []models.Entry) (*App, tea.Cmd) {
	a.isLocked = false
	a.vaultScreen = NewVaultScreen(entries)

	device, err := a.store.GetDevice()
	if err == nil {
		a.device = device
	}

	friends, _ := a.store.GetAllFriends()
	a.friendsScreen.SetFriends(friends)
	a.nearbyScreen.UpdatePeerPairedStatus(friends)

	for _, f := range friends {
		if f.LastAddr != "" {
			a.peerAddresses[f.Fingerprint] = f.LastAddr
		}
	}

	return a, nil
}

func (a *App) handleManualPeer(address string) tea.Cmd {
	return func() tea.Msg {
		peer, err := server.FetchAndPair(address)
		if err != nil {
			return StatusMsg{Message: "Failed to connect: " + err.Error(), IsError: true}
		}
		return PeerFoundMsg{Peer: *peer}
	}
}

func (a *App) handlePairing(peer models.Peer) tea.Cmd {
	return func() tea.Msg {
		addr := ""
		if peer.Host != "" && peer.Port != 0 {
			addr = fmt.Sprintf("%s:%d", peer.Host, peer.Port)
		}

		friend := models.Friend{
			Fingerprint: peer.Fingerprint,
			Name:        peer.Name,
			PubKey:      peer.PubKey,
			AddedAt:     time.Now(),
			LastAddr:    addr,
		}

		if err := a.store.SaveFriend(friend); err != nil {
			return PairingFailMsg{Err: err}
		}

		if addr != "" {
			a.peerAddresses[peer.Fingerprint] = addr
		}

		return PairingCompleteMsg{Friend: friend}
	}
}

func (a *App) handleSendShare(friend models.Friend, entry models.Entry) tea.Cmd {
	return func() tea.Msg {
		addr, ok := a.peerAddresses[friend.Fingerprint]
		if !ok && friend.LastAddr != "" {
			addr = friend.LastAddr
			a.peerAddresses[friend.Fingerprint] = addr
		}
		if addr == "" {
			return ShareFailMsg{Err: fmt.Errorf("no address for peer - re-add them in Nearby tab")}
		}

		host, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			return ShareFailMsg{Err: fmt.Errorf("invalid peer address: %w", err)}
		}
		var port int
		fmt.Sscanf(portStr, "%d", &port)

		err = server.SendShare(host, port, entry, a.device, &friend.PubKey)
		if err != nil {
			return ShareFailMsg{Err: fmt.Errorf("failed (is peer online at %s?): %w", addr, err)}
		}

		return ShareSentMsg{}
	}
}

func (a *App) handleAcceptShare(share models.IncomingShare) tea.Cmd {
	currentEntries := a.vaultScreen.GetEntries()

	entry := models.NewEntry(
		share.Entry.Website,
		share.Entry.Username,
		share.Entry.Password,
		share.Entry.Notes+" (shared by "+share.FromName+")",
		share.Entry.Tags,
	)

	newEntries := append(currentEntries, entry)

	return func() tea.Msg {
		return SaveEntriesMsg{Entries: newEntries}
	}
}

func (a *App) copyToClipboard(text, label string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.Copy(text); err != nil {
			return StatusMsg{Message: "Failed to copy: " + err.Error(), IsError: true}
		}
		return StatusMsg{Message: label + " copied!", IsError: false}
	}
}

func (a App) View() string {
	var b strings.Builder

	if a.isLocked {
		content := a.lockScreen.View()
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
	}

	if a.incomingScreen.IsVisible() {
		content := a.incomingScreen.View()
		return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
	}

	b.WriteString(a.renderTabs())
	b.WriteString("\n\n")

	switch a.activeTab {
	case TabVault:
		b.WriteString(a.vaultScreen.View())
	case TabNearby:
		b.WriteString(a.nearbyScreen.View())
	case TabFriends:
		b.WriteString(a.friendsScreen.View())
	}

	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("1-3 switch tabs • ctrl+l lock • ctrl+c quit"))

	if a.device != nil {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Device: %s [%s]", a.device.Name, a.device.Fingerprint())))
		b.WriteString("\n")
		b.WriteString(successStyle.Render(fmt.Sprintf("Manual add: %s", a.localAddr)))
	}

	return b.String()
}

func (a App) renderTabs() string {
	var tabs []string
	for i, name := range tabNames {
		if Tab(i) == a.activeTab {
			tabs = append(tabs, activeTabStyle.Render(name))
		} else {
			tabs = append(tabs, tabStyle.Render(name))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (a *App) GetDevice() *models.Device {
	return a.device
}

func (a *App) isInputActive() bool {
	switch a.activeTab {
	case TabVault:
		return a.vaultScreen.IsInputActive()
	case TabNearby:
		return a.nearbyScreen.IsInputActive()
	}
	return false
}
