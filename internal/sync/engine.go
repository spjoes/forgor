package sync

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"forgor/internal/crypto"
	"forgor/internal/models"
	"forgor/internal/storage"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/nacl/secretbox"
)

type Engine struct {
	client *Client
	state  *SyncState
	store  *storage.Store
	mu     sync.Mutex
}

func NewEngine(client *Client, state *SyncState, store *storage.Store) *Engine {
	return &Engine{
		client: client,
		state:  state,
		store:  store,
	}
}

func (e *Engine) RegisterDevice() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	keys, err := e.state.GetDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to get device keys: %w", err)
	}

	deviceIDBytes, err := keys.DeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("failed to decode device_id: %w", err)
	}

	signBytes, err := SignBytesDeviceBundle(deviceIDBytes, keys.PubkeySign[:], keys.PubkeyBox[:])
	if err != nil {
		return fmt.Errorf("failed to compute sign bytes: %w", err)
	}

	signature := crypto.Sign(keys.PrivkeySign, signBytes)

	bundle := DeviceBundle{
		DeviceID:         keys.DeviceID,
		DevicePubkeySign: keys.PubkeySign[:],
		DevicePubkeyBox:  keys.PubkeyBox[:],
		DeviceBundleSig:  signature,
	}

	if err := e.client.RegisterDevice(bundle); err != nil {
		return fmt.Errorf("failed to register device: %w", err)
	}

	return nil
}

func (e *Engine) CreateVault() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	keys, err := e.state.GetDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to get device keys: %w", err)
	}

	vaultID := NewUUID()

	var vaultKey [32]byte
	if _, err := rand.Read(vaultKey[:]); err != nil {
		return fmt.Errorf("failed to generate vault key: %w", err)
	}

	memberEventID := NewUUID()
	memberSeq := uint64(1)
	prevHash := make([]byte, 32)

	deviceIDBytes, err := keys.DeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("failed to decode device_id: %w", err)
	}

	bundleSignBytes, err := SignBytesDeviceBundle(deviceIDBytes, keys.PubkeySign[:], keys.PubkeyBox[:])
	if err != nil {
		return fmt.Errorf("failed to compute bundle sign bytes: %w", err)
	}
	bundleSig := crypto.Sign(keys.PrivkeySign, bundleSignBytes)

	signBytes, err := SignBytesMemberAdd(
		memberEventID.Bytes(),
		vaultID.Bytes(),
		memberSeq,
		prevHash,
		deviceIDBytes,
		deviceIDBytes,
		ZeroUUID.Bytes(),
		Zero64,
		bundleSig,
		keys.PubkeySign[:],
		keys.PubkeyBox[:],
	)
	if err != nil {
		return fmt.Errorf("failed to compute sign bytes: %w", err)
	}

	signature := crypto.Sign(keys.PrivkeySign, signBytes)

	memberEvent := MemberEvent{
		MsgType:           "member_add",
		MemberEventID:     memberEventID,
		VaultID:           vaultID,
		MemberSeq:         Uint64String(memberSeq),
		PrevHash:          prevHash,
		ActorDeviceID:     keys.DeviceID,
		SubjectDeviceID:   keys.DeviceID,
		SubjectPubkeySign: keys.PubkeySign[:],
		SubjectPubkeyBox:  keys.PubkeyBox[:],
		SubjectBundleSig:  bundleSig,
		InviteID:          ZeroUUID,
		ClaimSig:          Zero64,
		Signature:         signature,
	}

	if err := e.client.CreateMemberEvent(vaultID, memberEvent); err != nil {
		return fmt.Errorf("failed to create vault: %w", err)
	}

	if err := e.state.SetVaultID(vaultID); err != nil {
		return fmt.Errorf("failed to save vault_id: %w", err)
	}
	if err := e.state.SetVaultKey(vaultKey); err != nil {
		return fmt.Errorf("failed to save vault_key: %w", err)
	}
	if err := e.state.SetKeyEpoch(1); err != nil {
		return fmt.Errorf("failed to save key_epoch: %w", err)
	}
	if err := e.state.SetOwnerDeviceID(keys.DeviceID); err != nil {
		return fmt.Errorf("failed to save owner_device_id: %w", err)
	}

	eventHash := sha256.Sum256(signBytes)
	if err := e.state.SetMembershipHead(&MembershipHead{
		MemberSeq:      memberSeq,
		MemberHeadHash: eventHash,
	}); err != nil {
		return fmt.Errorf("failed to save membership head: %w", err)
	}

	if err := e.state.SetVerifiedMember(&VerifiedMember{
		DeviceID:   keys.DeviceID,
		PubkeySign: keys.PubkeySign[:],
		PubkeyBox:  keys.PubkeyBox[:],
		KeyEpoch:   1,
	}); err != nil {
		return fmt.Errorf("failed to save verified member: %w", err)
	}

	return nil
}

func (e *Engine) JoinVault(inviteID UUID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	keys, err := e.state.GetDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to get device keys: %w", err)
	}

	invites, err := e.client.GetInvites(string(keys.DeviceID))
	if err != nil {
		return fmt.Errorf("failed to get invites: %w", err)
	}

	var invite *Invite
	for i := range invites {
		if invites[i].InviteID == inviteID {
			invite = &invites[i]
			break
		}
	}
	if invite == nil {
		return fmt.Errorf("invite not found: %s", inviteID.String())
	}

	creatorBundle, err := e.client.GetDevice(string(invite.CreatedByDeviceID))
	if err != nil {
		return fmt.Errorf("failed to get invite creator bundle: %w", err)
	}

	creatorPubBox := [32]byte{}
	copy(creatorPubBox[:], creatorBundle.DevicePubkeyBox)

	decrypted, err := crypto.BoxOpen(
		append(invite.Nonce, invite.WrappedPayload...),
		&creatorPubBox,
		&keys.PrivkeyBox,
	)
	if err != nil {
		return fmt.Errorf("failed to decrypt invite payload: %w", err)
	}

	if len(decrypted) < 32 {
		return fmt.Errorf("invalid invite payload length")
	}

	var vaultKey [32]byte
	copy(vaultKey[:], decrypted[:32])

	deviceIDBytes, err := keys.DeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("failed to decode device_id: %w", err)
	}

	claimSignBytes, err := SignBytesInviteClaim(inviteID.Bytes(), invite.VaultID.Bytes(), deviceIDBytes)
	if err != nil {
		return fmt.Errorf("failed to compute claim sign bytes: %w", err)
	}

	claimSig := crypto.Sign(keys.PrivkeySign, claimSignBytes)

	claim := InviteClaim{
		MsgType:   "invite_claim",
		InviteID:  inviteID,
		VaultID:   invite.VaultID,
		DeviceID:  keys.DeviceID,
		Signature: claimSig,
	}

	if err := e.client.ClaimInvite(inviteID, claim); err != nil {
		return fmt.Errorf("failed to claim invite: %w", err)
	}

	if err := e.state.SetVaultID(invite.VaultID); err != nil {
		return fmt.Errorf("failed to save vault_id: %w", err)
	}
	if err := e.state.SetVaultKey(vaultKey); err != nil {
		return fmt.Errorf("failed to save vault_key: %w", err)
	}
	if err := e.state.SetKeyEpoch(1); err != nil {
		return fmt.Errorf("failed to save key_epoch: %w", err)
	}

	members, err := e.client.GetVaultMembers(invite.VaultID)
	if err != nil {
		return fmt.Errorf("failed to get vault members: %w", err)
	}

	if len(members.HeadHash) != 32 {
		return fmt.Errorf("invalid membership head hash length")
	}
	var memberHeadHash [32]byte
	copy(memberHeadHash[:], members.HeadHash)
	if err := e.state.SetMembershipHead(&MembershipHead{
		MemberSeq:      uint64(members.MemberSeq),
		MemberHeadHash: memberHeadHash,
	}); err != nil {
		return fmt.Errorf("failed to save membership head: %w", err)
	}

	if err := e.state.ClearVerifiedMembers(); err != nil {
		return fmt.Errorf("failed to reset verified members: %w", err)
	}

	for _, member := range members.Members {
		if err := e.state.SetVerifiedMember(&VerifiedMember{
			DeviceID:   member.DeviceID,
			PubkeySign: member.DevicePubkeySign,
			PubkeyBox:  member.DevicePubkeyBox,
			KeyEpoch:   uint64(member.KeyEpoch),
		}); err != nil {
			return fmt.Errorf("failed to save verified member: %w", err)
		}
	}

	return nil
}

func (e *Engine) InviteDevice(targetBundle DeviceBundle) (*Invite, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	keys, err := e.state.GetDeviceKeys()
	if err != nil {
		return nil, fmt.Errorf("failed to get device keys: %w", err)
	}

	vaultID, err := e.state.GetVaultID()
	if err != nil {
		return nil, fmt.Errorf("failed to get vault_id: %w", err)
	}

	vaultKey, err := e.state.GetVaultKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get vault_key: %w", err)
	}

	targetPubBox := [32]byte{}
	copy(targetPubBox[:], targetBundle.DevicePubkeyBox)

	sealed, err := crypto.BoxSeal(vaultKey[:], &targetPubBox, &keys.PrivkeyBox)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt vault key: %w", err)
	}

	nonce := sealed[:24]
	wrappedPayload := sealed[24:]

	inviteID := NewUUID()

	deviceIDBytes, err := keys.DeviceID.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to decode device_id: %w", err)
	}

	targetDeviceIDBytes, err := targetBundle.DeviceID.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to decode target device_id: %w", err)
	}

	signBytes, err := SignBytesInvite(
		inviteID.Bytes(),
		vaultID.Bytes(),
		targetDeviceIDBytes,
		targetBundle.DevicePubkeySign,
		targetBundle.DevicePubkeyBox,
		targetBundle.DeviceBundleSig,
		nonce,
		wrappedPayload,
		deviceIDBytes,
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to compute sign bytes: %w", err)
	}

	signature := crypto.Sign(keys.PrivkeySign, signBytes)

	invite := Invite{
		MsgType:                "invite",
		InviteID:               inviteID,
		VaultID:                vaultID,
		TargetDeviceID:         targetBundle.DeviceID,
		TargetDevicePubkeySign: targetBundle.DevicePubkeySign,
		TargetDevicePubkeyBox:  targetBundle.DevicePubkeyBox,
		TargetDeviceBundleSig:  targetBundle.DeviceBundleSig,
		Nonce:                  nonce,
		WrappedPayload:         wrappedPayload,
		CreatedByDeviceID:      keys.DeviceID,
		SingleUse:              true,
		Signature:              signature,
	}

	if err := e.client.CreateInvite(vaultID, invite); err != nil {
		return nil, fmt.Errorf("failed to create invite: %w", err)
	}

	return &invite, nil
}

func (e *Engine) InviteDeviceByID(targetDeviceID string) (*Invite, error) {
	targetBundle, err := e.client.GetDevice(targetDeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device bundle: %w", err)
	}

	return e.InviteDevice(*targetBundle)
}

func (e *Engine) AcceptInviteClaim(claim InviteClaim) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	keys, err := e.state.GetDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to get device keys: %w", err)
	}

	vaultID, err := e.state.GetVaultID()
	if err != nil {
		return fmt.Errorf("failed to get vault_id: %w", err)
	}
	if claim.VaultID != vaultID {
		return fmt.Errorf("invite claim vault mismatch")
	}

	targetBundle, err := e.client.GetDevice(string(claim.DeviceID))
	if err != nil {
		return fmt.Errorf("failed to get device bundle: %w", err)
	}

	memberHead, err := e.state.GetMembershipHead()
	if err != nil {
		return fmt.Errorf("failed to get membership head: %w", err)
	}

	memberEventID := NewUUID()
	newMemberSeq := memberHead.MemberSeq + 1

	deviceIDBytes, err := keys.DeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("failed to decode device_id: %w", err)
	}

	targetDeviceIDBytes, err := targetBundle.DeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("failed to decode target device_id: %w", err)
	}

	claimSig := claim.Signature
	signBytes, err := SignBytesMemberAdd(
		memberEventID.Bytes(),
		vaultID.Bytes(),
		newMemberSeq,
		memberHead.MemberHeadHash[:],
		deviceIDBytes,
		targetDeviceIDBytes,
		claim.InviteID.Bytes(),
		claimSig,
		targetBundle.DeviceBundleSig,
		targetBundle.DevicePubkeySign,
		targetBundle.DevicePubkeyBox,
	)
	if err != nil {
		return fmt.Errorf("failed to compute sign bytes: %w", err)
	}

	signature := crypto.Sign(keys.PrivkeySign, signBytes)

	memberEvent := MemberEvent{
		MsgType:           "member_add",
		MemberEventID:     memberEventID,
		VaultID:           vaultID,
		MemberSeq:         Uint64String(newMemberSeq),
		PrevHash:          memberHead.MemberHeadHash[:],
		ActorDeviceID:     keys.DeviceID,
		SubjectDeviceID:   targetBundle.DeviceID,
		SubjectPubkeySign: targetBundle.DevicePubkeySign,
		SubjectPubkeyBox:  targetBundle.DevicePubkeyBox,
		SubjectBundleSig:  targetBundle.DeviceBundleSig,
		InviteID:          claim.InviteID,
		ClaimSig:          claimSig,
		Signature:         signature,
	}

	if err := e.client.CreateMemberEvent(vaultID, memberEvent); err != nil {
		return fmt.Errorf("failed to create member event: %w", err)
	}

	eventHash := sha256.Sum256(signBytes)
	if err := e.state.SetMembershipHead(&MembershipHead{
		MemberSeq:      newMemberSeq,
		MemberHeadHash: eventHash,
	}); err != nil {
		return fmt.Errorf("failed to save membership head: %w", err)
	}

	if err := e.state.SetVerifiedMember(&VerifiedMember{
		DeviceID:   targetBundle.DeviceID,
		PubkeySign: targetBundle.DevicePubkeySign,
		PubkeyBox:  targetBundle.DevicePubkeyBox,
		KeyEpoch:   1,
	}); err != nil {
		return fmt.Errorf("failed to save verified member: %w", err)
	}

	return nil
}

func (e *Engine) AcceptPendingInviteClaims() error {
	keys, err := e.state.GetDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to get device keys: %w", err)
	}

	claims, err := e.client.GetInviteClaims(string(keys.DeviceID))
	if err != nil {
		return fmt.Errorf("failed to get invite claims: %w", err)
	}

	if len(claims) == 0 {
		return nil
	}

	vaultID, err := e.state.GetVaultID()
	if err != nil {
		return fmt.Errorf("failed to get vault_id: %w", err)
	}

	for _, claim := range claims {
		if claim.VaultID != vaultID {
			continue
		}
		if err := e.AcceptInviteClaim(claim); err != nil {
			if isInviteAlreadyUsed(err) {
				continue
			}
			return err
		}
	}

	return nil
}

func (e *Engine) RefreshMembership() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	vaultID, err := e.state.GetVaultID()
	if err != nil {
		return fmt.Errorf("failed to get vault_id: %w", err)
	}

	members, err := e.client.GetVaultMembers(vaultID)
	if err != nil {
		return fmt.Errorf("failed to get vault members: %w", err)
	}

	if len(members.HeadHash) != 32 {
		return fmt.Errorf("invalid membership head hash length")
	}
	var memberHeadHash [32]byte
	copy(memberHeadHash[:], members.HeadHash)
	if err := e.state.SetMembershipHead(&MembershipHead{
		MemberSeq:      uint64(members.MemberSeq),
		MemberHeadHash: memberHeadHash,
	}); err != nil {
		return fmt.Errorf("failed to save membership head: %w", err)
	}

	if err := e.state.ClearVerifiedMembers(); err != nil {
		return fmt.Errorf("failed to reset verified members: %w", err)
	}

	for _, member := range members.Members {
		if err := e.state.SetVerifiedMember(&VerifiedMember{
			DeviceID:   member.DeviceID,
			PubkeySign: member.DevicePubkeySign,
			PubkeyBox:  member.DevicePubkeyBox,
			KeyEpoch:   uint64(member.KeyEpoch),
		}); err != nil {
			return fmt.Errorf("failed to save verified member: %w", err)
		}
	}

	return nil
}

func (e *Engine) FlushPendingEntries() error {
	pending, err := e.state.GetPendingEntries()
	if err != nil {
		return fmt.Errorf("failed to load pending entries: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}

	var firstErr error
	for _, item := range pending {
		if item.Op != "upsert" && item.Op != "delete" {
			if firstErr == nil {
				firstErr = fmt.Errorf("invalid pending operation: %s", item.Op)
			}
			continue
		}

		if err := e.PushEntry(item.Entry, item.Op); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		_ = e.state.RemovePendingEntry(item.Entry.ID)
	}

	return firstErr
}

func isInviteAlreadyUsed(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.Code == "invite_already_used" {
			return true
		}
		if apiErr.StatusCode == http.StatusConflict &&
			strings.Contains(strings.ToLower(apiErr.Message), "invite has already been used") {
			return true
		}
	}
	return false
}

func (e *Engine) PushEntry(entry models.Entry, op string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if op != "upsert" && op != "delete" {
		return fmt.Errorf("invalid operation: %s", op)
	}

	keys, err := e.state.GetDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to get device keys: %w", err)
	}

	vaultID, err := e.state.GetVaultID()
	if err != nil {
		return fmt.Errorf("failed to get vault_id: %w", err)
	}

	keyEpoch, err := e.state.GetKeyEpoch()
	if err != nil {
		return fmt.Errorf("failed to get key_epoch: %w", err)
	}

	eventHead, err := e.state.GetEventHead(keys.DeviceID)
	if err != nil {
		return fmt.Errorf("failed to get event head: %w", err)
	}

	lamport, err := e.state.IncrementLamport()
	if err != nil {
		return fmt.Errorf("failed to increment lamport: %w", err)
	}

	ciphertext, nonce, err := e.encryptEventPayload(op, entry)
	if err != nil {
		return fmt.Errorf("failed to encrypt event: %w", err)
	}

	eventID := NewUUID()
	counter := eventHead.LastCounter + 1

	deviceIDBytes, err := keys.DeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("failed to decode device_id: %w", err)
	}

	signBytes, err := SignBytesEvent(
		eventID.Bytes(),
		vaultID.Bytes(),
		deviceIDBytes,
		counter,
		lamport,
		keyEpoch,
		eventHead.LastHash[:],
		nonce,
		ciphertext,
	)
	if err != nil {
		return fmt.Errorf("failed to compute sign bytes: %w", err)
	}

	signature := crypto.Sign(keys.PrivkeySign, signBytes)

	event := Event{
		MsgType:    "event",
		EventID:    eventID,
		VaultID:    vaultID,
		DeviceID:   keys.DeviceID,
		Counter:    Uint64String(counter),
		Lamport:    Uint64String(lamport),
		KeyEpoch:   Uint64String(keyEpoch),
		PrevHash:   eventHead.LastHash[:],
		Nonce:      nonce,
		Ciphertext: ciphertext,
		Signature:  signature,
	}

	if _, err := e.client.PushEvent(vaultID, event); err != nil {
		return fmt.Errorf("failed to push event: %w", err)
	}

	eventHash := sha256.Sum256(signBytes)
	if err := e.state.SetEventHead(keys.DeviceID, &EventHead{
		LastCounter: counter,
		LastHash:    eventHash,
	}); err != nil {
		return fmt.Errorf("failed to update event head: %w", err)
	}

	if op == "upsert" {
		_ = e.state.SetEntryScheme(entry.ID, "v2")
	} else {
		_ = e.state.RemoveEntryScheme(entry.ID)
	}

	return nil
}

func (e *Engine) PullEvents() ([]models.Entry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	vaultID, err := e.state.GetVaultID()
	if err != nil {
		return nil, fmt.Errorf("failed to get vault_id: %w", err)
	}

	cursor, err := e.state.GetSyncCursor()
	if err != nil {
		return nil, fmt.Errorf("failed to get sync cursor: %w", err)
	}

	events, err := e.client.PullEvents(vaultID, cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to pull events: %w", err)
	}

	if len(events) == 0 {
		return nil, nil
	}

	var updatedEntries []models.Entry
	var maxSeq uint64 = cursor
	var maxLamport uint64

	currentLamport, err := e.state.GetLamport()
	if err != nil {
		return nil, fmt.Errorf("failed to get lamport: %w", err)
	}
	maxLamport = currentLamport

	for _, event := range events {
		member, err := e.state.GetVerifiedMember(event.DeviceID)
		if err != nil {
			continue
		}

		deviceIDBytes, err := event.DeviceID.Bytes()
		if err != nil {
			continue
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
			continue
		}

		pubkeySign := [32]byte{}
		copy(pubkeySign[:], member.PubkeySign)
		if !crypto.Verify(pubkeySign, signBytes, event.Signature) {
			continue
		}

		op, entry, scheme, err := e.decryptEventPayload(event.Ciphertext, event.Nonce, uint64(event.KeyEpoch))
		if err != nil {
			continue
		}

		if op == "upsert" || op == "delete" {
			updatedEntries = append(updatedEntries, entry)
			if op == "upsert" {
				_ = e.state.SetEntryScheme(entry.ID, scheme)
			} else if op == "delete" {
				_ = e.state.RemoveEntryScheme(entry.ID)
			}
		}

		if uint64(event.Seq) > maxSeq {
			maxSeq = uint64(event.Seq)
		}
		if uint64(event.Lamport) > maxLamport {
			maxLamport = uint64(event.Lamport)
		}
	}

	if maxSeq > cursor {
		if err := e.state.SetSyncCursor(maxSeq); err != nil {
			return nil, fmt.Errorf("failed to update sync cursor: %w", err)
		}
	}

	if maxLamport > currentLamport {
		if _, err := e.state.UpdateLamport(maxLamport); err != nil {
			return nil, fmt.Errorf("failed to update lamport: %w", err)
		}
	}

	return updatedEntries, nil
}

func (e *Engine) SyncEntries(localEntries []models.Entry) ([]models.Entry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	vaultID, err := e.state.GetVaultID()
	if err != nil {
		return localEntries, fmt.Errorf("failed to get vault_id: %w", err)
	}

	cursor, err := e.state.GetSyncCursor()
	if err != nil {
		return localEntries, fmt.Errorf("failed to get sync cursor: %w", err)
	}

	remoteEvents, err := e.client.PullEvents(vaultID, cursor)
	if err != nil {
		return localEntries, fmt.Errorf("failed to pull events: %w", err)
	}

	entryMap := make(map[string]models.Entry)
	entryLamport := make(map[string]uint64)
	entryDeviceID := make(map[string]string)
	deletedIDs := make(map[string]bool)

	for _, entry := range localEntries {
		entryMap[entry.ID] = entry
	}

	var maxSeq uint64 = cursor
	var maxLamport uint64

	currentLamport, err := e.state.GetLamport()
	if err != nil {
		return localEntries, fmt.Errorf("failed to get lamport: %w", err)
	}
	maxLamport = currentLamport

	for _, event := range remoteEvents {
		member, err := e.state.GetVerifiedMember(event.DeviceID)
		if err != nil {
			continue
		}

		deviceIDBytes, err := event.DeviceID.Bytes()
		if err != nil {
			continue
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
			continue
		}

		pubkeySign := [32]byte{}
		copy(pubkeySign[:], member.PubkeySign)
		if !crypto.Verify(pubkeySign, signBytes, event.Signature) {
			continue
		}

		op, entry, scheme, err := e.decryptEventPayload(event.Ciphertext, event.Nonce, uint64(event.KeyEpoch))
		if err != nil {
			continue
		}

		eventLamport := uint64(event.Lamport)
		eventDeviceID := string(event.DeviceID)

		if op == "delete" {
			existingLamport, exists := entryLamport[entry.ID]
			if !exists || eventLamport > existingLamport ||
				(eventLamport == existingLamport && eventDeviceID > entryDeviceID[entry.ID]) {
				deletedIDs[entry.ID] = true
				delete(entryMap, entry.ID)
				entryLamport[entry.ID] = eventLamport
				entryDeviceID[entry.ID] = eventDeviceID
				_ = e.state.RemoveEntryScheme(entry.ID)
			}
		} else if op == "upsert" {
			existingLamport, exists := entryLamport[entry.ID]
			if !exists || eventLamport > existingLamport ||
				(eventLamport == existingLamport && eventDeviceID > entryDeviceID[entry.ID]) {
				if !deletedIDs[entry.ID] {
					entryMap[entry.ID] = entry
					entryLamport[entry.ID] = eventLamport
					entryDeviceID[entry.ID] = eventDeviceID
					_ = e.state.SetEntryScheme(entry.ID, scheme)
				}
			}
		}

		if uint64(event.Seq) > maxSeq {
			maxSeq = uint64(event.Seq)
		}
		if eventLamport > maxLamport {
			maxLamport = eventLamport
		}
	}

	if maxSeq > cursor {
		if err := e.state.SetSyncCursor(maxSeq); err != nil {
			return localEntries, fmt.Errorf("failed to update sync cursor: %w", err)
		}
	}

	if maxLamport > currentLamport {
		if _, err := e.state.UpdateLamport(maxLamport); err != nil {
			return localEntries, fmt.Errorf("failed to update lamport: %w", err)
		}
	}

	result := make([]models.Entry, 0, len(entryMap))
	for _, entry := range entryMap {
		result = append(result, entry)
	}

	return result, nil
}

func (e *Engine) encryptEventPayload(op string, entry models.Entry) (ciphertext, nonce []byte, err error) {
	vaultKey, err := e.state.GetVaultKey()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get vault_key: %w", err)
	}

	keyEpoch, err := e.state.GetKeyEpoch()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get key_epoch: %w", err)
	}

	eventKey, err := deriveEventKey(vaultKey[:], keyEpoch)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to derive event key: %w", err)
	}

	payload := struct {
		Op    string       `json:"op"`
		Entry models.Entry `json:"entry"`
	}{
		Op:    op,
		Entry: entry,
	}

	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	nonce = make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	aead, err := chacha20poly1305.NewX(eventKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	ciphertext = aead.Seal(nil, nonce, plaintext, nil)

	return ciphertext, nonce, nil
}

func (e *Engine) decryptEventPayload(ciphertext, nonce []byte, keyEpoch uint64) (op string, entry models.Entry, scheme string, err error) {
	vaultKey, err := e.state.GetVaultKey()
	if err != nil {
		return "", entry, "", fmt.Errorf("failed to get vault_key: %w", err)
	}

	plaintext, err := decryptEventPayloadXChaCha(vaultKey[:], keyEpoch, nonce, ciphertext)
	if err != nil {
		plaintext, err = decryptEventPayloadLegacy(vaultKey[:], keyEpoch, nonce, ciphertext)
		if err != nil {
			return "", entry, "", fmt.Errorf("failed to decrypt: %w", err)
		}
		scheme = "legacy"
	} else {
		scheme = "v2"
	}

	var payload struct {
		Op    string       `json:"op"`
		Entry models.Entry `json:"entry"`
	}

	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return "", entry, "", fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return payload.Op, payload.Entry, scheme, nil
}

func decryptEventPayloadXChaCha(vaultKey []byte, keyEpoch uint64, nonce, ciphertext []byte) ([]byte, error) {
	eventKey, err := deriveEventKey(vaultKey, keyEpoch)
	if err != nil {
		return nil, fmt.Errorf("failed to derive event key: %w", err)
	}

	aead, err := chacha20poly1305.NewX(eventKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func decryptEventPayloadLegacy(vaultKey []byte, keyEpoch uint64, nonce, ciphertext []byte) ([]byte, error) {
	if len(nonce) != NonceLength {
		return nil, fmt.Errorf("invalid nonce length")
	}

	legacyKey := deriveLegacyEventKey(vaultKey, keyEpoch)
	var nonceArr [24]byte
	copy(nonceArr[:], nonce)

	plaintext, ok := secretbox.Open(nil, ciphertext, &nonceArr, &legacyKey)
	if !ok {
		return nil, fmt.Errorf("legacy decrypt failed")
	}

	return plaintext, nil
}

func (e *Engine) signEvent(event *Event) error {
	keys, err := e.state.GetDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to get device keys: %w", err)
	}

	deviceIDBytes, err := keys.DeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("failed to decode device_id: %w", err)
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

	event.Signature = crypto.Sign(keys.PrivkeySign, signBytes)
	return nil
}

func (e *Engine) signMemberEvent(event *MemberEvent) error {
	keys, err := e.state.GetDeviceKeys()
	if err != nil {
		return fmt.Errorf("failed to get device keys: %w", err)
	}

	deviceIDBytes, err := keys.DeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("failed to decode actor device_id: %w", err)
	}

	subjectDeviceIDBytes, err := event.SubjectDeviceID.Bytes()
	if err != nil {
		return fmt.Errorf("failed to decode subject device_id: %w", err)
	}

	var signBytes []byte
	if event.MsgType == "member_add" {
		signBytes, err = SignBytesMemberAdd(
			event.MemberEventID.Bytes(),
			event.VaultID.Bytes(),
			uint64(event.MemberSeq),
			event.PrevHash,
			deviceIDBytes,
			subjectDeviceIDBytes,
			event.InviteID.Bytes(),
			event.ClaimSig,
			event.SubjectBundleSig,
			event.SubjectPubkeySign,
			event.SubjectPubkeyBox,
		)
	} else if event.MsgType == "member_remove" {
		signBytes, err = SignBytesMemberRemove(
			event.MemberEventID.Bytes(),
			event.VaultID.Bytes(),
			uint64(event.MemberSeq),
			event.PrevHash,
			deviceIDBytes,
			subjectDeviceIDBytes,
		)
	} else {
		return fmt.Errorf("unknown member event type: %s", event.MsgType)
	}

	if err != nil {
		return fmt.Errorf("failed to compute sign bytes: %w", err)
	}

	event.Signature = crypto.Sign(keys.PrivkeySign, signBytes)
	return nil
}

func deriveEventKey(vaultKey []byte, keyEpoch uint64) ([]byte, error) {
	info := fmt.Sprintf("forgor-event-key-epoch-%d", keyEpoch)
	hkdfReader := hkdf.New(sha256.New, vaultKey, nil, []byte(info))

	eventKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, eventKey); err != nil {
		return nil, fmt.Errorf("failed to derive event key: %w", err)
	}

	return eventKey, nil
}

func deriveLegacyEventKey(vaultKey []byte, keyEpoch uint64) [32]byte {
	info := fmt.Sprintf("forgor-event-key-epoch-%d", keyEpoch)
	combined := make([]byte, 0, len(vaultKey)+len(info))
	combined = append(combined, vaultKey...)
	combined = append(combined, []byte(info)...)
	return sha256.Sum256(combined)
}
