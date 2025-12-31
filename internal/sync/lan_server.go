package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"forgor/internal/crypto"
	"forgor/internal/storage"
)

// Literally like the entire LAN syncing implementation is not done yet lol. I left notes where things definitely need to be changed.
// I focused majority on the cloud based coordination server since that's the main use case for now imo.

type LANServer struct {
	store  *storage.Store
	state  *SyncState
	port   int
	server *http.Server
	mu     gosync.RWMutex
}

func NewLANServer(store *storage.Store, state *SyncState, port int) *LANServer {
	return &LANServer{
		store: store,
		state: state,
		port:  port,
	}
}

func (s *LANServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/whoami", s.handleWhoami)
	mux.HandleFunc("/v1/vaults/", s.handleVaults)

	s.mu.Lock()
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	s.mu.Unlock()

	return s.server.ListenAndServe()
}

func (s *LANServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
		s.server = nil
	}
}

func (s *LANServer) Port() int {
	return s.port
}

type WhoamiResponse struct {
	DeviceID   DeviceID    `json:"device_id"`
	VaultID    UUID        `json:"vault_id"`
	PubkeySign Base64Bytes `json:"pubkey_sign"`
	PubkeyBox  Base64Bytes `json:"pubkey_box"`
}

func (s *LANServer) handleWhoami(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
		return
	}

	keys, err := s.state.GetDeviceKeys()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get device keys")
		return
	}

	vaultID, err := s.state.GetVaultID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get vault ID")
		return
	}

	resp := WhoamiResponse{
		DeviceID:   keys.DeviceID,
		VaultID:    vaultID,
		PubkeySign: keys.PubkeySign[:],
		PubkeyBox:  keys.PubkeyBox[:],
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *LANServer) handleVaults(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/vaults/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		s.writeError(w, http.StatusNotFound, "not_found", "Invalid path")
		return
	}

	vaultIDStr := parts[0]
	endpoint := parts[1]

	requestedVaultID, err := ParseUUID(vaultIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_vault_id", "Invalid vault ID format")
		return
	}

	myVaultID, err := s.state.GetVaultID()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get vault ID")
		return
	}

	if requestedVaultID != myVaultID {
		s.writeError(w, http.StatusForbidden, "wrong_vault", "This server only handles its own vault")
		return
	}

	switch {
	case endpoint == "events" && r.Method == http.MethodGet:
		s.handlePullEvents(w, r, myVaultID)
	case endpoint == "events" && r.Method == http.MethodPost:
		s.handlePushEvent(w, r, myVaultID)
	case endpoint == "member_events" && r.Method == http.MethodGet:
		s.handleGetMemberEvents(w, r, myVaultID)
	case endpoint == "members" && r.Method == http.MethodGet:
		s.handleGetMembers(w, r, myVaultID)
	default:
		s.writeError(w, http.StatusNotFound, "not_found", "Endpoint not found")
	}
}

func (s *LANServer) handlePullEvents(w http.ResponseWriter, r *http.Request, vaultID UUID) {
	sinceSeqStr := r.URL.Query().Get("since_seq")
	var sinceSeq uint64
	if sinceSeqStr != "" {
		var err error
		sinceSeq, err = strconv.ParseUint(sinceSeqStr, 10, 64)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_since_seq", "Invalid since_seq parameter")
			return
		}
	}

	events, err := s.getLocalEvents(vaultID, sinceSeq)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get events")
		return
	}

	s.writeJSON(w, http.StatusOK, events)
}

func (s *LANServer) handlePushEvent(w http.ResponseWriter, r *http.Request, vaultID UUID) {
	var event Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse event")
		return
	}

	if event.VaultID != vaultID {
		s.writeError(w, http.StatusBadRequest, "vault_mismatch", "Event vault_id does not match")
		return
	}

	if err := s.validateEventSignature(&event); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_signature", err.Error())
		return
	}

	if err := s.validateSenderIsMember(event.DeviceID); err != nil {
		s.writeError(w, http.StatusForbidden, "not_member", err.Error())
		return
	}

	seq, err := s.storeEvent(&event)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to store event")
		return
	}

	s.writeJSON(w, http.StatusOK, EventResponse{Seq: Uint64String(seq)})
}

func (s *LANServer) handleGetMemberEvents(w http.ResponseWriter, r *http.Request, vaultID UUID) {
	sinceSeqStr := r.URL.Query().Get("since_seq")
	var sinceSeq uint64
	if sinceSeqStr != "" {
		var err error
		sinceSeq, err = strconv.ParseUint(sinceSeqStr, 10, 64)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_since_seq", "Invalid since_seq parameter")
			return
		}
	}

	events, err := s.getLocalMemberEvents(vaultID, sinceSeq)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get member events")
		return
	}

	s.writeJSON(w, http.StatusOK, events)
}

func (s *LANServer) handleGetMembers(w http.ResponseWriter, r *http.Request, vaultID UUID) {
	members, err := s.state.GetVerifiedMembers()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get members")
		return
	}

	head, err := s.state.GetMembershipHead()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get membership head")
		return
	}

	vaultMembers := make([]VaultMember, len(members))
	for i, m := range members {
		vaultMembers[i] = VaultMember{
			DeviceID:         m.DeviceID,
			DevicePubkeySign: m.PubkeySign,
			DevicePubkeyBox:  m.PubkeyBox,
			KeyEpoch:         Uint64String(m.KeyEpoch),
		}
	}

	resp := VaultMembershipResponse{
		MemberSeq: Uint64String(head.MemberSeq),
		HeadHash:  head.MemberHeadHash[:],
		Members:   vaultMembers,
	}

	s.writeJSON(w, http.StatusOK, resp)
}

func (s *LANServer) validateEventSignature(event *Event) error {
	member, err := s.state.GetVerifiedMember(event.DeviceID)
	if err != nil {
		return fmt.Errorf("sender not a verified member: %w", err)
	}

	deviceIDBytes, err := event.DeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("invalid device_id: %w", err)
	}

	signBytes, err := SignBytesEvent(
		event.EventID.Bytes(),
		event.VaultID.Bytes(),
		deviceIDBytes,
		uint64(event.Counter),
		uint64(event.Lamport),
		uint64(event.KeyEpoch),
		event.PrevHash,
		event.Nonce,
		event.Ciphertext,
	)
	if err != nil {
		return fmt.Errorf("failed to compute sign bytes: %w", err)
	}

	pubkeySign := [32]byte{}
	copy(pubkeySign[:], member.PubkeySign)
	if !crypto.Verify(pubkeySign, signBytes, event.Signature) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

func (s *LANServer) validateSenderIsMember(deviceID DeviceID) error {
	_, err := s.state.GetVerifiedMember(deviceID)
	if err != nil {
		return fmt.Errorf("device is not a vault member")
	}
	return nil
}

func (s *LANServer) getLocalEvents(vaultID UUID, sinceSeq uint64) ([]Event, error) {
	// LAN server returns events from local sync state
	// For now, return empty. events are currently stored by the sync engine
	return []Event{}, nil
}

func (s *LANServer) getLocalMemberEvents(vaultID UUID, sinceSeq uint64) ([]MemberEvent, error) {
	// Similar to getLocalEvents, member events would need to be stored locally
	// For now, we're just gonna return empty
	return []MemberEvent{}, nil
}

func (s *LANServer) storeEvent(event *Event) (uint64, error) {
	// Store the event locally and return a sequence number
	// For LAN sync, we're just gonna use the lamport timestamp as a pseudo-sequence
	// the full implementation will store events in BoltDB (bbolt in GoLang)
	return uint64(event.Lamport), nil
}

func (s *LANServer) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *LANServer) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, APIError{Code: code, Message: message})
}
