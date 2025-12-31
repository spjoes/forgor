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
	"forgor/internal/sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Tab int

const (
	TabVault Tab = iota
	TabNearby
	TabFriends
	TabSync
)

var tabNames = []string{"Vault", "Nearby", "Friends", "Sync"}

type App struct {
	store      *storage.Store
	isLocked   bool
	isNewVault bool
	activeTab  Tab
	width      int
	height     int

	lockScreen     LockScreen
	vaultScreen    VaultScreen
	nearbyScreen   NearbyScreen
	friendsScreen  FriendsScreen
	syncScreen     SyncScreen
	incomingScreen IncomingShareScreen

	device    *models.Device
	localAddr string

	peerAddresses map[string]string

	peerChan  chan models.Peer
	shareChan chan models.IncomingShare

	syncEngine *sync.Engine
	syncState  *sync.SyncState

	statusMsg     string
	statusIsError bool
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
		syncScreen:     NewSyncScreen(),
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
			case "4":
				a.activeTab = TabSync
				return a, nil
			case "tab":
				a.activeTab = Tab((int(a.activeTab) + 1) % 4)
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

	case SyncPushEntryMsg:
		return a, a.handleSyncPushEntry(msg.Entry, msg.Op)

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
		case TabSync:
			a.syncScreen, _ = a.syncScreen.Update(msg)
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

	case SetupSyncMsg:
		return a, a.handleSetupSync(msg.ServerURL, msg.Action)

	case RegisterDeviceMsg:
		return a, a.handleRegisterDevice(msg.ServerURL)

	case SyncNowMsg:
		return a, tea.Batch(
			func() tea.Msg {
				return SyncStatusUpdateMsg{Status: "syncing"}
			},
			a.handleSyncNow(),
		)

	case SyncStatusUpdateMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
		return a, nil

	case SyncNowCompleteMsg:
		if err := a.store.SaveEntries(msg.Entries); err != nil {
			a.syncScreen, _ = a.syncScreen.Update(SyncStatusUpdateMsg{Status: "error"})
			a.syncScreen, _ = a.syncScreen.Update(StatusMsg{Message: "Failed to save: " + err.Error(), IsError: true})
			return a, nil
		}
		a.vaultScreen.SetEntries(msg.Entries)
		a.syncScreen, _ = a.syncScreen.Update(SyncStatusUpdateMsg{
			Status:   "synced",
			LastSync: msg.LastSync,
			Members:  msg.Members,
		})
		if msg.Warning != nil {
			a.syncScreen, _ = a.syncScreen.Update(StatusMsg{
				Message: "Synced with warnings: " + msg.Warning.Error(),
				IsError: true,
			})
		}
		return a, nil

	case SyncNowFailMsg:
		a.syncScreen, _ = a.syncScreen.Update(SyncStatusUpdateMsg{Status: "error"})
		a.syncScreen, _ = a.syncScreen.Update(StatusMsg{Message: "Sync failed: " + msg.Err.Error(), IsError: true})
		return a, nil

	case InviteDeviceMsg:
		return a, a.handleInviteDevice(msg.TargetDeviceID)

	case AcceptInviteMsg:
		return a, a.handleAcceptInvite(msg.InviteCode, msg.ServerURL)

	case InviteCreatedMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
		return a, nil

	case InviteAcceptedMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
		a.initSyncFromState()
		return a, nil

	case InviteFailMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
		return a, nil

	case LeaveSyncVaultMsg:
		return a, a.handleLeaveVault()

	case LeaveVaultCompleteMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
		a.initSyncFromState()
		return a, nil

	case LeaveVaultFailMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
		return a, nil

	case SyncRegisterCompleteMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
		a.initSyncFromState()
		return a, nil

	case SyncRegisterFailMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
		return a, nil

	case SyncSetupCompleteMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
		a.initSyncFromState()
		return a, nil

	case SyncSetupFailMsg:
		a.syncScreen, _ = a.syncScreen.Update(msg)
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
		case TabSync:
			var cmd tea.Cmd
			a.syncScreen, cmd = a.syncScreen.Update(msg)
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

	a.initSyncFromState()

	return a, nil
}

func (a *App) initSyncFromState() {
	a.syncState = nil
	a.syncEngine = nil
	a.syncScreen.SetConfigured(false)
	a.syncScreen.SetVaultID("")

	vaultKey := a.store.GetVaultKey()
	if vaultKey == nil {
		return
	}

	syncState, err := sync.NewSyncState(a.store.GetDB(), vaultKey)
	if err != nil {
		return
	}

	if keys, err := syncState.GetDeviceKeys(); err == nil {
		a.syncScreen.SetDeviceFingerprint(string(keys.DeviceID))
	}

	if vaultID, err := syncState.GetVaultID(); err == nil {
		a.syncScreen.SetVaultID(vaultID.String())
	}

	serverURL, err := syncState.GetServerURL()
	if err != nil {
		return
	}
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return
	}

	client := sync.NewClient(serverURL)
	a.syncState = syncState
	a.syncEngine = sync.NewEngine(client, syncState, a.store)
	a.syncScreen.SetServerURL(serverURL)
	if syncState.IsConfigured() {
		a.syncScreen.SetConfigured(true)
	}
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

		emptyKey := [32]byte{}
		if peer.PubKey == emptyKey {
			if addr == "" {
				return PairingFailMsg{Err: fmt.Errorf("cannot verify peer: no address available")}
			}
			verifiedPeer, err := server.FetchAndPair(addr)
			if err != nil {
				return PairingFailMsg{Err: fmt.Errorf("failed to verify peer: %w", err)}
			}
			if verifiedPeer.Fingerprint != peer.Fingerprint {
				return PairingFailMsg{Err: fmt.Errorf("fingerprint mismatch - possible spoofing")}
			}
			peer.PubKey = verifiedPeer.PubKey
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

	return tea.Batch(
		func() tea.Msg {
			return SaveEntriesMsg{Entries: newEntries}
		},
		func() tea.Msg {
			return SyncPushEntryMsg{Entry: entry, Op: "upsert"}
		},
	)
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
	case TabSync:
		b.WriteString(a.syncScreen.View())
	}

	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("1-4 switch tabs • ctrl+l lock • ctrl+c quit"))

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
	case TabSync:
		return a.syncScreen.IsInputActive()
	}
	return false
}

func (a *App) handleSetupSync(serverURL, action string) tea.Cmd {
	entries := append([]models.Entry(nil), a.vaultScreen.GetEntries()...)
	return func() tea.Msg {
		if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
			serverURL = "http://" + serverURL
		}

		vaultKey := a.store.GetVaultKey()
		if vaultKey == nil {
			return SyncSetupFailMsg{Err: fmt.Errorf("vault not unlocked")}
		}

		syncState, err := sync.NewSyncState(a.store.GetDB(), vaultKey)
		if err != nil {
			return SyncSetupFailMsg{Err: fmt.Errorf("failed to initialize sync state: %w", err)}
		}

		if err := syncState.SetServerURL(serverURL); err != nil {
			return SyncSetupFailMsg{Err: fmt.Errorf("failed to save server URL: %w", err)}
		}

		if err := a.initDeviceKeysIfNeeded(syncState); err != nil {
			return SyncSetupFailMsg{Err: fmt.Errorf("failed to init device keys: %w", err)}
		}

		client := sync.NewClient(serverURL)
		engine := sync.NewEngine(client, syncState, a.store)

		if err := engine.RegisterDevice(); err != nil {
			return SyncSetupFailMsg{Err: fmt.Errorf("failed to register device: %w", err)}
		}

		if action == "create" {
			if err := engine.CreateVault(); err != nil {
				return SyncSetupFailMsg{Err: fmt.Errorf("failed to create vault: %w", err)}
			}
			if len(entries) > 0 {
				for _, entry := range entries {
					if err := engine.PushEntry(entry, "upsert"); err != nil {
						return SyncSetupFailMsg{Err: fmt.Errorf("failed to seed vault entries: %w", err)}
					}
				}
			}
		}

		return SyncSetupCompleteMsg{}
	}
}

func (a *App) initDeviceKeysIfNeeded(syncState *sync.SyncState) error {
	_, err := syncState.GetDeviceKeys()
	if err == nil {
		return nil
	}

	keys, err := sync.GenerateDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to generate device keys: %w", err)
	}

	return syncState.SetDeviceKeys(keys)
}

func (a *App) handleLeaveVault() tea.Cmd {
	return func() tea.Msg {
		vaultKey := a.store.GetVaultKey()
		if vaultKey == nil {
			return LeaveVaultFailMsg{Err: fmt.Errorf("vault not unlocked")}
		}

		syncState, err := sync.NewSyncState(a.store.GetDB(), vaultKey)
		if err != nil {
			return LeaveVaultFailMsg{Err: fmt.Errorf("failed to initialize sync state: %w", err)}
		}

		if err := syncState.ClearVaultState(); err != nil {
			return LeaveVaultFailMsg{Err: fmt.Errorf("failed to clear vault state: %w", err)}
		}
		if err := syncState.ClearVerifiedMembers(); err != nil {
			return LeaveVaultFailMsg{Err: fmt.Errorf("failed to clear verified members: %w", err)}
		}
		if err := syncState.ClearEventHeads(); err != nil {
			return LeaveVaultFailMsg{Err: fmt.Errorf("failed to clear event heads: %w", err)}
		}
		if err := syncState.ClearPendingEntries(); err != nil {
			return LeaveVaultFailMsg{Err: fmt.Errorf("failed to clear pending changes: %w", err)}
		}

		return LeaveVaultCompleteMsg{}
	}
}

func (a *App) handleRegisterDevice(serverURL string) tea.Cmd {
	return func() tea.Msg {
		if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
			serverURL = "http://" + serverURL
		}

		vaultKey := a.store.GetVaultKey()
		if vaultKey == nil {
			return SyncRegisterFailMsg{Err: fmt.Errorf("vault not unlocked")}
		}

		syncState, err := sync.NewSyncState(a.store.GetDB(), vaultKey)
		if err != nil {
			return SyncRegisterFailMsg{Err: fmt.Errorf("failed to initialize sync state: %w", err)}
		}

		if err := syncState.SetServerURL(serverURL); err != nil {
			return SyncRegisterFailMsg{Err: fmt.Errorf("failed to save server URL: %w", err)}
		}

		if err := a.initDeviceKeysIfNeeded(syncState); err != nil {
			return SyncRegisterFailMsg{Err: fmt.Errorf("failed to init device keys: %w", err)}
		}

		client := sync.NewClient(serverURL)
		engine := sync.NewEngine(client, syncState, a.store)

		if err := engine.RegisterDevice(); err != nil {
			return SyncRegisterFailMsg{Err: fmt.Errorf("failed to register device: %w", err)}
		}

		return SyncRegisterCompleteMsg{}
	}
}

func (a *App) handleAcceptInvite(inviteCode, serverURL string) tea.Cmd {
	return func() tea.Msg {
		inviteCode = strings.TrimSpace(inviteCode)
		if inviteCode == "" {
			return InviteFailMsg{Err: fmt.Errorf("invite code is required")}
		}

		if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
			serverURL = "http://" + serverURL
		}

		vaultKey := a.store.GetVaultKey()
		if vaultKey == nil {
			return InviteFailMsg{Err: fmt.Errorf("vault not unlocked")}
		}

		syncState, err := sync.NewSyncState(a.store.GetDB(), vaultKey)
		if err != nil {
			return InviteFailMsg{Err: fmt.Errorf("failed to initialize sync state: %w", err)}
		}

		if err := syncState.SetServerURL(serverURL); err != nil {
			return InviteFailMsg{Err: fmt.Errorf("failed to save server URL: %w", err)}
		}

		if err := a.initDeviceKeysIfNeeded(syncState); err != nil {
			return InviteFailMsg{Err: fmt.Errorf("failed to init device keys: %w", err)}
		}

		client := sync.NewClient(serverURL)
		engine := sync.NewEngine(client, syncState, a.store)

		if err := engine.RegisterDevice(); err != nil {
			return InviteFailMsg{Err: fmt.Errorf("failed to register device: %w", err)}
		}

		inviteID, err := sync.ParseUUID(inviteCode)
		if err != nil {
			return InviteFailMsg{Err: fmt.Errorf("invalid invite code: %w", err)}
		}

		if err := engine.JoinVault(inviteID); err != nil {
			return InviteFailMsg{Err: err}
		}

		return InviteAcceptedMsg{}
	}
}

func (a *App) handleInviteDevice(targetDeviceID string) tea.Cmd {
	entries := append([]models.Entry(nil), a.vaultScreen.GetEntries()...)
	return func() tea.Msg {
		if a.syncState == nil || a.syncEngine == nil {
			return InviteFailMsg{Err: fmt.Errorf("sync not configured")}
		}

		if err := a.seedLocalEntriesIfNeeded(entries); err != nil {
			return InviteFailMsg{Err: fmt.Errorf("failed to seed vault entries: %w", err)}
		}

		targetDeviceID = strings.TrimSpace(targetDeviceID)
		if targetDeviceID == "" {
			return InviteFailMsg{Err: fmt.Errorf("target device ID is required")}
		}
		if err := sync.DeviceID(targetDeviceID).Validate(); err != nil {
			return InviteFailMsg{Err: fmt.Errorf("invalid target device ID: %w", err)}
		}

		invite, err := a.syncEngine.InviteDeviceByID(targetDeviceID)
		if err != nil {
			return InviteFailMsg{Err: err}
		}

		return InviteCreatedMsg{InviteCode: invite.InviteID.String()}
	}
}

func (a *App) handleSyncNow() tea.Cmd {
	entries := append([]models.Entry(nil), a.vaultScreen.GetEntries()...)
	return func() tea.Msg {
		if a.syncState == nil || a.syncEngine == nil {
			return SyncNowFailMsg{Err: fmt.Errorf("sync not configured")}
		}

		if err := a.acceptInviteClaimsIfOwner(); err != nil {
			return SyncNowFailMsg{Err: err}
		}

		var warnErr error
		if err := a.syncEngine.RefreshMembership(); err != nil {
			warnErr = mergeSyncWarning(warnErr, fmt.Errorf("failed to refresh vault members: %w", err))
		}
		if err := a.seedLocalEntriesIfNeeded(entries); err != nil {
			warnErr = mergeSyncWarning(warnErr, fmt.Errorf("some changes could not be pushed yet: %w", err))
		}

		if err := a.syncEngine.FlushPendingEntries(); err != nil && warnErr == nil {
			warnErr = mergeSyncWarning(warnErr, fmt.Errorf("some changes could not be pushed yet: %w", err))
		}

		newEntries, err := a.syncEngine.SyncEntries(entries)
		if err != nil {
			return SyncNowFailMsg{Err: err}
		}

		memberCount := 0
		if members, err := a.syncState.GetVerifiedMembers(); err == nil {
			memberCount = len(members)
		}

		return SyncNowCompleteMsg{
			Entries:  newEntries,
			LastSync: time.Now(),
			Members:  memberCount,
			Warning:  warnErr,
		}
	}
}

func mergeSyncWarning(current error, next error) error {
	if next == nil {
		return current
	}
	if current == nil {
		return next
	}
	return fmt.Errorf("%v; %v", current.Error(), next.Error())
}

func (a *App) seedLocalEntriesIfNeeded(entries []models.Entry) error {
	if a.syncState == nil || a.syncEngine == nil {
		return fmt.Errorf("sync not configured")
	}
	if len(entries) == 0 {
		return nil
	}

	keys, err := a.syncState.GetDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to get device keys: %w", err)
	}
	head, err := a.syncState.GetEventHead(keys.DeviceID)
	if err != nil {
		return fmt.Errorf("failed to get event head: %w", err)
	}
	if head.LastCounter != 0 {
		return nil
	}

	var firstErr error
	for _, entry := range entries {
		if err := a.syncEngine.PushEntry(entry, "upsert"); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			_ = a.syncState.AddPendingEntry("upsert", entry)
			continue
		}
		_ = a.syncState.RemovePendingEntry(entry.ID)
	}

	return firstErr
}

func (a *App) isSyncOwner() (bool, error) {
	if a.syncState == nil {
		return false, fmt.Errorf("sync not configured")
	}

	keys, err := a.syncState.GetDeviceKeys()
	if err != nil {
		return false, fmt.Errorf("failed to get device keys: %w", err)
	}

	owner, err := a.syncState.GetOwnerDeviceID()
	if err != nil {
		return false, nil
	}

	return owner == keys.DeviceID, nil
}

func (a *App) acceptInviteClaimsIfOwner() error {
	isOwner, err := a.isSyncOwner()
	if err != nil {
		return err
	}
	if !isOwner {
		return nil
	}

	if a.syncEngine == nil {
		return fmt.Errorf("sync not configured")
	}

	if err := a.syncEngine.AcceptPendingInviteClaims(); err != nil {
		return err
	}

	return nil
}

func (a *App) handleSyncPushEntry(entry models.Entry, op string) tea.Cmd {
	return func() tea.Msg {
		if a.syncState == nil || a.syncEngine == nil {
			return nil
		}
		if op != "upsert" && op != "delete" {
			return StatusMsg{Message: "Sync failed: invalid operation", IsError: true}
		}

		if err := a.syncEngine.PushEntry(entry, op); err != nil {
			_ = a.syncState.AddPendingEntry(op, entry)
			return StatusMsg{Message: "Sync push failed (queued for retry): " + err.Error(), IsError: true}
		}
		_ = a.syncState.RemovePendingEntry(entry.ID)

		return SyncStatusUpdateMsg{
			Status:   "synced",
			LastSync: time.Now(),
		}
	}
}
