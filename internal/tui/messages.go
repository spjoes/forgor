package tui

import "forgor/internal/models"


type UnlockSuccessMsg struct {
	Entries []models.Entry
}

type UnlockFailMsg struct {
	Err error
}

type InitSuccessMsg struct{}

type InitFailMsg struct {
	Err error
}

type SaveSuccessMsg struct{}

type SaveFailMsg struct {
	Err error
}

type PeerFoundMsg struct {
	Peer models.Peer
}

type PeerLostMsg struct {
	Fingerprint string
}

type IncomingShareMsg struct {
	Share models.IncomingShare
}

type PairingCompleteMsg struct {
	Friend models.Friend
}

type PairingFailMsg struct {
	Err error
}

type ShareSentMsg struct{}

type ShareFailMsg struct {
	Err error
}

type CopyToClipboardMsg struct {
	Text    string
	Label   string
}

type StatusMsg struct {
	Message string
	IsError bool
}

type ClearStatusMsg struct{}

type RefreshFriendsMsg struct {
	Friends []models.Friend
}

type TickMsg struct{}
