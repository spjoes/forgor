package models

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Entry struct {
	ID        string    `json:"id"`
	Website   string    `json:"website"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	Notes     string    `json:"notes"`
	Tags      []string  `json:"tags,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewEntry(website, username, password, notes string, tags []string) Entry {
	return Entry{
		ID:        generateID(),
		Website:   website,
		Username:  username,
		Password:  password,
		Notes:     notes,
		Tags:      tags,
		UpdatedAt: time.Now(),
	}
}

type Friend struct {
	Fingerprint string    `json:"fingerprint"`
	Name        string    `json:"name"`
	PubKey      [32]byte  `json:"pubkey"`
	AddedAt     time.Time `json:"added_at"`
	LastAddr    string    `json:"last_addr,omitempty"` // this is stored for cases where peer was manually added and not found. we can stay in sync across restarts
}

type Device struct {
	Name    string   `json:"name"`
	PubKey  [32]byte `json:"pubkey"`
	PrivKey [32]byte `json:"privkey"`
}

func (d *Device) Fingerprint() string {
	return ComputeFingerprint(d.PubKey[:])
}

type Peer struct {
	Name        string
	Fingerprint string
	Host        string
	Port        int
	PubKey      [32]byte
	IsPaired    bool
}

type ShareMessage struct {
	FromFingerprint string `json:"from_fingerprint"`
	Ciphertext      []byte `json:"ciphertext"`
}

type WhoAmIResponse struct {
	DeviceName  string   `json:"device_name"`
	PubKey      [32]byte `json:"pubkey"`
	Fingerprint string   `json:"fingerprint"`
	Version     string   `json:"version"`
}

type IncomingShare struct {
	FromFingerprint string
	FromName        string
	Entry           Entry
}

func ComputeFingerprint(pubKey []byte) string {
	if len(pubKey) < 8 {
		return ""
	}
	return hex.EncodeToString(pubKey[:8])
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
