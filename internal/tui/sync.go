package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type syncMode int

const (
	syncModeList syncMode = iota
	syncModeSetup
	syncModeInvite
	syncModeAcceptInvite
	syncModeManageDevices
	syncModeConfirmRemove
)

type syncStatus string

const (
	syncStatusDisconnected syncStatus = "disconnected"
	syncStatusSyncing      syncStatus = "syncing"
	syncStatusSynced       syncStatus = "synced"
	syncStatusError        syncStatus = "error"
)

type SyncScreen struct {
	mode              syncMode
	serverURL         textinput.Model
	inviteCode        textinput.Model
	targetFingerprint textinput.Model
	statusMsg         string
	isError           bool
	syncStatus        syncStatus
	lastSyncTime      time.Time
	memberCount       int
	deviceFingerprint string
	vaultID           string
	cursor            int
	configured        bool
	generatedInvite   string
	members           []string
	isOwner           bool
	manageCursor      int
	confirmRemoveID   string
}

func NewSyncScreen() SyncScreen {
	serverURL := textinput.New()
	serverURL.Placeholder = "https://forgor.example.com"
	serverURL.Width = 50

	inviteCode := textinput.New()
	inviteCode.Placeholder = "Enter invite code"
	inviteCode.Width = 40

	targetFingerprint := textinput.New()
	targetFingerprint.Placeholder = "Target device ID (64-hex)"
	targetFingerprint.Width = 40

	return SyncScreen{
		mode:              syncModeList,
		serverURL:         serverURL,
		inviteCode:        inviteCode,
		targetFingerprint: targetFingerprint,
		syncStatus:        syncStatusDisconnected,
	}
}

func (s SyncScreen) Init() tea.Cmd {
	return nil
}

func (s SyncScreen) Update(msg tea.Msg) (SyncScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case SyncStatusUpdateMsg:
		s.syncStatus = syncStatus(msg.Status)
		if !msg.LastSync.IsZero() {
			s.lastSyncTime = msg.LastSync
		}
		if msg.Members != 0 {
			s.memberCount = msg.Members
		}
		if msg.Status == string(syncStatusSynced) || msg.Status == string(syncStatusSyncing) {
			s.configured = true
		}
		return s, nil

	case SyncSetupCompleteMsg:
		s.mode = syncModeList
		s.configured = true
		s.statusMsg = "Sync configured successfully!"
		s.isError = false
		s.serverURL.SetValue("")
		return s, nil

	case SyncSetupFailMsg:
		s.statusMsg = "Setup failed: " + msg.Err.Error()
		s.isError = true
		return s, nil

	case InviteCreatedMsg:
		s.generatedInvite = msg.InviteCode
		s.statusMsg = "Invite created!"
		s.isError = false
		return s, nil

	case InviteAcceptedMsg:
		s.mode = syncModeList
		s.statusMsg = "Successfully joined vault!"
		s.isError = false
		s.inviteCode.SetValue("")
		return s, nil

	case InviteFailMsg:
		s.statusMsg = "Invite failed: " + msg.Err.Error()
		s.isError = true
		return s, nil

	case LeaveVaultCompleteMsg:
		s.mode = syncModeList
		s.configured = false
		s.statusMsg = "Left vault"
		s.isError = false
		s.generatedInvite = ""
		s.targetFingerprint.SetValue("")
		return s, nil

	case LeaveVaultFailMsg:
		s.statusMsg = "Leave vault failed: " + msg.Err.Error()
		s.isError = true
		return s, nil

	case SyncRegisterCompleteMsg:
		s.statusMsg = "Device registered. Share your Device ID to receive an invite."
		s.isError = false
		return s, nil

	case SyncRegisterFailMsg:
		s.statusMsg = "Device registration failed: " + msg.Err.Error()
		s.isError = true
		return s, nil

	case StatusMsg:
		s.statusMsg = msg.Message
		s.isError = msg.IsError
		return s, nil

	case tea.KeyMsg:
		switch s.mode {
		case syncModeList:
			return s.updateList(msg)
		case syncModeSetup:
			return s.updateSetup(msg)
		case syncModeInvite:
			return s.updateInvite(msg)
		case syncModeAcceptInvite:
			return s.updateAcceptInvite(msg)
		case syncModeManageDevices:
			return s.updateManageDevices(msg)
		case syncModeConfirmRemove:
			return s.updateRemoveConfirm(msg)
		}
	}

	return s, nil
}

func (s SyncScreen) updateList(msg tea.KeyMsg) (SyncScreen, tea.Cmd) {
	options := s.listOptions()
	maxCursor := len(options) - 1
	if s.cursor > maxCursor {
		s.cursor = maxCursor
	}

	switch msg.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < maxCursor {
			s.cursor++
		}
	case "y":
		if s.deviceFingerprint == "" {
			s.statusMsg = "Device ID not available"
			s.isError = true
			return s, nil
		}
		return s, func() tea.Msg {
			return CopyToClipboardMsg{Text: s.deviceFingerprint, Label: "Device ID"}
		}
	case "enter":
		if !s.configured {
			s.mode = syncModeSetup
			s.serverURL.Focus()
			return s, textinput.Blink
		}
		switch options[s.cursor] {
		case "Setup Sync":
			s.mode = syncModeSetup
			s.serverURL.Focus()
			return s, textinput.Blink
		case "Sync Now":
			return s, func() tea.Msg {
				return SyncNowMsg{}
			}
		case "Invite Device":
			s.mode = syncModeInvite
			s.generatedInvite = ""
			s.targetFingerprint.Focus()
			return s, textinput.Blink
		case "Manage Devices":
			s.mode = syncModeManageDevices
			s.manageCursor = 0
			s.confirmRemoveID = ""
			return s, nil
		case "Leave Vault":
			return s, func() tea.Msg {
				return LeaveSyncVaultMsg{}
			}
		}
	}
	return s, nil
}

func (s SyncScreen) listOptions() []string {
	if !s.configured {
		return []string{"Setup Sync"}
	}
	options := []string{"Setup Sync", "Sync Now", "Invite Device"}
	if s.isOwner {
		options = append(options, "Manage Devices")
	}
	options = append(options, "Leave Vault")
	return options
}

func (s SyncScreen) updateManageDevices(msg tea.KeyMsg) (SyncScreen, tea.Cmd) {
	if len(s.members) == 0 {
		s.manageCursor = 0
	} else if s.manageCursor >= len(s.members) {
		s.manageCursor = len(s.members) - 1
	}

	switch msg.String() {
	case "esc":
		s.mode = syncModeList
		s.cursor = 0
		s.confirmRemoveID = ""
		return s, nil
	case "up", "k":
		if s.manageCursor > 0 {
			s.manageCursor--
		}
	case "down", "j":
		if s.manageCursor < len(s.members)-1 {
			s.manageCursor++
		}
	case "y":
		if len(s.members) == 0 {
			s.statusMsg = "No devices to copy"
			s.isError = true
			return s, nil
		}
		deviceID := s.members[s.manageCursor]
		return s, func() tea.Msg {
			return CopyToClipboardMsg{Text: deviceID, Label: "Device ID"}
		}
	case "enter":
		if len(s.members) == 0 {
			return s, nil
		}
		deviceID := s.members[s.manageCursor]
		if deviceID == s.deviceFingerprint {
			s.statusMsg = "Cannot remove the current device"
			s.isError = true
			return s, nil
		}
		s.confirmRemoveID = deviceID
		s.mode = syncModeConfirmRemove
		return s, nil
	}
	return s, nil
}

func (s SyncScreen) updateRemoveConfirm(msg tea.KeyMsg) (SyncScreen, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		deviceID := s.confirmRemoveID
		s.confirmRemoveID = ""
		s.mode = syncModeManageDevices
		if deviceID == "" {
			return s, nil
		}
		return s, func() tea.Msg {
			return RemoveDeviceMsg{DeviceID: deviceID}
		}
	case "n", "N", "esc":
		s.confirmRemoveID = ""
		s.mode = syncModeManageDevices
		return s, nil
	}
	return s, nil
}

func (s SyncScreen) updateSetup(msg tea.KeyMsg) (SyncScreen, tea.Cmd) {
	switch msg.String() {
	case "esc":
		s.mode = syncModeList
		s.serverURL.SetValue("")
		s.cursor = 0
		return s, nil
	case "enter":
		url := strings.TrimSpace(s.serverURL.Value())
		if url == "" {
			s.statusMsg = "Please enter a server URL"
			s.isError = true
			return s, nil
		}
		return s, nil
	case "c":
		url := strings.TrimSpace(s.serverURL.Value())
		if url == "" {
			s.statusMsg = "Please enter a server URL first"
			s.isError = true
			return s, nil
		}
		return s, func() tea.Msg {
			return SetupSyncMsg{ServerURL: url, Action: "create"}
		}
	case "j":
		url := strings.TrimSpace(s.serverURL.Value())
		if url == "" {
			s.statusMsg = "Please enter a server URL first"
			s.isError = true
			return s, nil
		}
		s.mode = syncModeAcceptInvite
		s.inviteCode.Focus()
		return s, tea.Batch(
			textinput.Blink,
			func() tea.Msg {
				return RegisterDeviceMsg{ServerURL: url}
			},
		)
	}

	var cmd tea.Cmd
	s.serverURL, cmd = s.serverURL.Update(msg)
	return s, cmd
}

func (s SyncScreen) updateInvite(msg tea.KeyMsg) (SyncScreen, tea.Cmd) {
	switch msg.String() {
	case "esc":
		s.mode = syncModeList
		s.generatedInvite = ""
		s.targetFingerprint.SetValue("")
		s.cursor = 0
		return s, nil
	case "g":
		targetID := strings.TrimSpace(s.targetFingerprint.Value())
		if targetID == "" {
			s.statusMsg = "Please enter a target device ID"
			s.isError = true
			return s, nil
		}
		return s, func() tea.Msg {
			return InviteDeviceMsg{TargetDeviceID: targetID}
		}
	case "i":
		if s.generatedInvite == "" {
			s.statusMsg = "No invite code to copy"
			s.isError = true
			return s, nil
		}
		return s, func() tea.Msg {
			return CopyToClipboardMsg{Text: s.generatedInvite, Label: "Invite code"}
		}
	case "y":
		if s.deviceFingerprint == "" {
			s.statusMsg = "Device ID not available"
			s.isError = true
			return s, nil
		}
		return s, func() tea.Msg {
			return CopyToClipboardMsg{Text: s.deviceFingerprint, Label: "Device ID"}
		}
	}

	var cmd tea.Cmd
	s.targetFingerprint, cmd = s.targetFingerprint.Update(msg)
	return s, cmd
}

func (s SyncScreen) updateAcceptInvite(msg tea.KeyMsg) (SyncScreen, tea.Cmd) {
	switch msg.String() {
	case "esc":
		s.mode = syncModeSetup
		s.inviteCode.SetValue("")
		return s, nil
	case "enter":
		code := strings.TrimSpace(s.inviteCode.Value())
		if code == "" {
			s.statusMsg = "Please enter an invite code"
			s.isError = true
			return s, nil
		}
		url := strings.TrimSpace(s.serverURL.Value())
		return s, func() tea.Msg {
			return AcceptInviteMsg{InviteCode: code, ServerURL: url}
		}
	case "y":
		if s.deviceFingerprint == "" {
			s.statusMsg = "Device ID not available"
			s.isError = true
			return s, nil
		}
		return s, func() tea.Msg {
			return CopyToClipboardMsg{Text: s.deviceFingerprint, Label: "Device ID"}
		}
	}

	var cmd tea.Cmd
	s.inviteCode, cmd = s.inviteCode.Update(msg)
	return s, cmd
}

func (s SyncScreen) View() string {
	var b strings.Builder

	switch s.mode {
	case syncModeList:
		b.WriteString(s.viewList())
	case syncModeSetup:
		b.WriteString(s.viewSetup())
	case syncModeInvite:
		b.WriteString(s.viewInvite())
	case syncModeAcceptInvite:
		b.WriteString(s.viewAcceptInvite())
	case syncModeManageDevices:
		b.WriteString(s.viewManageDevices())
	case syncModeConfirmRemove:
		b.WriteString(s.viewConfirmRemove())
	}

	if s.statusMsg != "" {
		b.WriteString("\n")
		if s.isError {
			b.WriteString(errorStyle.Render("⚠ " + s.statusMsg))
		} else {
			b.WriteString(successStyle.Render("✓ " + s.statusMsg))
		}
	}

	return b.String()
}

func (s SyncScreen) viewList() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Cloud Sync"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Sync your vault across devices"))
	b.WriteString("\n\n")

	if s.deviceFingerprint != "" {
		b.WriteString(fmt.Sprintf("Device ID: %s\n", successStyle.Render(s.deviceFingerprint)))
		b.WriteString("\n")
	}
	if s.vaultID != "" {
		b.WriteString(fmt.Sprintf("Vault ID: %s\n", successStyle.Render(s.vaultID)))
		b.WriteString("\n")
	}

	if s.configured {
		statusIcon := "○"
		statusText := "Disconnected"
		statusRender := mutedStyle

		switch s.syncStatus {
		case syncStatusSyncing:
			statusIcon = "◐"
			statusText = "Syncing..."
			statusRender = mutedStyle
		case syncStatusSynced:
			statusIcon = "●"
			statusText = "Synced"
			statusRender = successStyle
		case syncStatusError:
			statusIcon = "✕"
			statusText = "Error"
			statusRender = errorStyle
		}

		b.WriteString(fmt.Sprintf("Status: %s %s\n", statusIcon, statusRender.Render(statusText)))

		if !s.lastSyncTime.IsZero() {
			b.WriteString(fmt.Sprintf("Last Sync: %s\n", mutedStyle.Render(s.lastSyncTime.Format("Jan 02 15:04"))))
		}

		if s.memberCount > 0 {
			b.WriteString(fmt.Sprintf("Vault Members: %s\n", mutedStyle.Render(fmt.Sprintf("%d devices", s.memberCount))))
		}

		b.WriteString("\n")

		options := s.listOptions()
		for i, opt := range options {
			cursor := "  "
			style := normalStyle
			if i == s.cursor {
				cursor = "▸ "
				style = selectedStyle
			}
			if opt == "Leave Vault" {
				if s.cursor == i {
					b.WriteString(fmt.Sprintf("%s%s\n", cursor, errorStyle.Render(opt)))
				} else {
					b.WriteString(fmt.Sprintf("%s%s\n", cursor, mutedStyle.Render(opt)))
				}
			} else {
				b.WriteString(fmt.Sprintf("%s%s\n", cursor, style.Render(opt)))
			}
		}
	} else {
		b.WriteString(mutedStyle.Render("Sync is not configured."))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("Set up cloud sync to access your vault from multiple devices."))
		b.WriteString("\n\n")

		cursor := "▸ "
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, selectedStyle.Render("Setup Sync")))
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate • enter select • y copy device id"))

	return b.String()
}

func (s SyncScreen) viewSetup() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Setup Cloud Sync"))
	b.WriteString("\n\n")

	if s.deviceFingerprint != "" {
		b.WriteString(fmt.Sprintf("Device ID: %s\n\n", successStyle.Render(s.deviceFingerprint)))
	}

	b.WriteString("Server URL:\n")
	b.WriteString(focusedInputStyle.Render(s.serverURL.View()))
	b.WriteString("\n\n")

	b.WriteString("Options:\n")
	b.WriteString(normalStyle.Render("  c - Create New Vault"))
	b.WriteString("\n")
	b.WriteString(normalStyle.Render("  j - Join Existing Vault"))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("c create • j join • esc cancel"))

	return boxStyle.Render(b.String())
}

func (s SyncScreen) viewInvite() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Invite Device"))
	b.WriteString("\n\n")

	if s.deviceFingerprint != "" {
		b.WriteString("Your Device ID:\n")
		b.WriteString(successStyle.Render(s.deviceFingerprint))
		b.WriteString("\n\n")
	}

	b.WriteString("Target Device ID:\n")
	b.WriteString(focusedInputStyle.Render(s.targetFingerprint.View()))
	b.WriteString("\n\n")
	b.WriteString(errorStyle.Render("WARNING: This will allow anyone connected to your coordination server to connect to your vault. Be careful who you give this to."))
	b.WriteString("\n\n")

	if s.generatedInvite != "" {
		b.WriteString("Share this invite code with the target device:\n\n")
		b.WriteString(boxStyle.Render(successStyle.Render(s.generatedInvite)))
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render("The recipient should use this code to join the vault."))
	} else {
		b.WriteString(mutedStyle.Render("Generate an invite code to share with another device."))
		b.WriteString("\n\n")
		b.WriteString(normalStyle.Render("Press 'g' to generate an invite code."))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("g generate invite • i copy invite • y copy device id • esc back"))

	return boxStyle.Render(b.String())
}

func (s SyncScreen) viewAcceptInvite() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Join Vault"))
	b.WriteString("\n\n")

	if s.deviceFingerprint != "" {
		b.WriteString("Your Device ID:\n")
		b.WriteString(successStyle.Render(s.deviceFingerprint))
		b.WriteString("\n\n")
	}

	b.WriteString("Enter the invite code you received:\n")
	b.WriteString(focusedInputStyle.Render(s.inviteCode.View()))
	b.WriteString("\n\n")

	b.WriteString(mutedStyle.Render("The invite code was provided by another vault member."))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("enter submit • y copy device id • esc back"))

	return boxStyle.Render(b.String())
}

func (s SyncScreen) viewManageDevices() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Manage Devices"))
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("Only the vault owner can remove devices."))
	b.WriteString("\n\n")

	if len(s.members) == 0 {
		b.WriteString(mutedStyle.Render("No devices found for this vault."))
	} else {
		for i, deviceID := range s.members {
			cursor := "  "
			style := normalStyle
			if i == s.manageCursor {
				cursor = ""
				style = selectedStyle
			}

			label := formatDeviceID(deviceID)
			if deviceID == s.deviceFingerprint {
				if s.isOwner {
					label += " (you, owner)"
				} else {
					label += " (you)"
				}
			}

			b.WriteString(fmt.Sprintf("%s%s\n", cursor, style.Render(label)))
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("enter remove • y copy device id • esc back"))

	return boxStyle.Render(b.String())
}

func (s SyncScreen) viewConfirmRemove() string {
	var b strings.Builder

	b.WriteString(errorStyle.Render("Remove Device?"))
	b.WriteString("\n\n")

	if s.confirmRemoveID != "" {
		b.WriteString(fmt.Sprintf("Remove device %s from this vault?\n", formatDeviceID(s.confirmRemoveID)))
	} else {
		b.WriteString("Remove this device from the vault?\n")
	}
	b.WriteString("This will revoke its access and cannot be undone.\n\n")

	b.WriteString(helpStyle.Render("y confirm • n cancel"))

	return boxStyle.Render(b.String())
}

func (s *SyncScreen) SetDeviceFingerprint(fingerprint string) {
	s.deviceFingerprint = fingerprint
}

func (s *SyncScreen) SetMembers(members []string) {
	s.members = members
	s.memberCount = len(members)
}

func (s *SyncScreen) SetIsOwner(isOwner bool) {
	s.isOwner = isOwner
}

func (s *SyncScreen) SetConfigured(configured bool) {
	s.configured = configured
}

func (s *SyncScreen) SetServerURL(url string) {
	s.serverURL.SetValue(url)
}

func (s *SyncScreen) SetVaultID(vaultID string) {
	s.vaultID = vaultID
}

func (s SyncScreen) IsInputActive() bool {
	return s.mode == syncModeSetup || s.mode == syncModeInvite || s.mode == syncModeAcceptInvite
}

func formatDeviceID(deviceID string) string {
	if len(deviceID) <= 12 {
		return deviceID
	}
	return fmt.Sprintf("%s...%s", deviceID[:6], deviceID[len(deviceID)-6:])
}

type SetupSyncMsg struct {
	ServerURL string
	Action    string
}

type RegisterDeviceMsg struct {
	ServerURL string
}

type SyncNowMsg struct{}

type InviteDeviceMsg struct {
	TargetDeviceID string
}

type AcceptInviteMsg struct {
	InviteCode string
	ServerURL  string
}

type SyncStatusUpdateMsg struct {
	Status   string
	LastSync time.Time
	Members  int
}

type SyncSetupCompleteMsg struct{}

type SyncSetupFailMsg struct {
	Err error
}

type SyncRegisterCompleteMsg struct{}

type SyncRegisterFailMsg struct {
	Err error
}

type InviteCreatedMsg struct {
	InviteCode string
}

type InviteAcceptedMsg struct{}

type InviteFailMsg struct {
	Err error
}

type LeaveSyncVaultMsg struct{}

type LeaveVaultCompleteMsg struct{}

type LeaveVaultFailMsg struct {
	Err error
}
