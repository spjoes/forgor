package sync

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"
)

type UUID uuid.UUID

func (u UUID) String() string {
	return uuid.UUID(u).String()
}

func (u UUID) Bytes() []byte {
	b := uuid.UUID(u)
	return b[:]
}

func (u UUID) MarshalJSON() ([]byte, error) {
	return json.Marshal(uuid.UUID(u).String())
}

func (u *UUID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		return err
	}
	*u = UUID(parsed)
	return nil
}

func NewUUID() UUID {
	return UUID(uuid.New())
}

func ParseUUID(s string) (UUID, error) {
	parsed, err := uuid.Parse(s)
	if err != nil {
		return UUID{}, err
	}
	return UUID(parsed), nil
}

type Uint64String uint64

func (u Uint64String) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatUint(uint64(u), 10))
}

func (u *Uint64String) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	val, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return err
	}
	*u = Uint64String(val)
	return nil
}

type Base64Bytes []byte

func (b Base64Bytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.StdEncoding.EncodeToString(b))
}

func (b *Base64Bytes) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	*b = decoded
	return nil
}

type DeviceID string

func (d DeviceID) Bytes() ([]byte, error) {
	return hex.DecodeString(string(d))
}

func (d DeviceID) Validate() error {
	if len(d) != 64 {
		return fmt.Errorf("device_id must be 64 hex characters")
	}
	_, err := hex.DecodeString(string(d))
	return err
}

type DeviceBundle struct {
	DeviceID         DeviceID    `json:"device_id"`
	DevicePubkeySign Base64Bytes `json:"device_pubkey_sign"`
	DevicePubkeyBox  Base64Bytes `json:"device_pubkey_box"`
	DeviceBundleSig  Base64Bytes `json:"device_bundle_sig"`
}

type Event struct {
	MsgType    string       `json:"msg_type"`
	EventID    UUID         `json:"event_id"`
	VaultID    UUID         `json:"vault_id"`
	DeviceID   DeviceID     `json:"device_id"`
	Counter    Uint64String `json:"counter"`
	Lamport    Uint64String `json:"lamport"`
	KeyEpoch   Uint64String `json:"key_epoch"`
	PrevHash   Base64Bytes  `json:"prev_hash"`
	Nonce      Base64Bytes  `json:"nonce"`
	Ciphertext Base64Bytes  `json:"ciphertext"`
	Signature  Base64Bytes  `json:"signature"`
	Seq        Uint64String `json:"seq,omitempty"`
	CreatedAt  string       `json:"created_at,omitempty"`
}

type MemberEvent struct {
	MsgType           string       `json:"msg_type"`
	MemberEventID     UUID         `json:"member_event_id"`
	VaultID           UUID         `json:"vault_id"`
	MemberSeq         Uint64String `json:"member_seq"`
	PrevHash          Base64Bytes  `json:"prev_hash"`
	ActorDeviceID     DeviceID     `json:"actor_device_id"`
	SubjectDeviceID   DeviceID     `json:"subject_device_id"`
	SubjectPubkeySign Base64Bytes  `json:"subject_pubkey_sign,omitempty"`
	SubjectPubkeyBox  Base64Bytes  `json:"subject_pubkey_box,omitempty"`
	SubjectBundleSig  Base64Bytes  `json:"subject_bundle_sig,omitempty"`
	InviteID          UUID         `json:"invite_id,omitempty"`
	ClaimSig          Base64Bytes  `json:"claim_sig,omitempty"`
	Signature         Base64Bytes  `json:"signature"`
	CreatedAt         string       `json:"created_at,omitempty"`
}

type Invite struct {
	MsgType                string      `json:"msg_type"`
	InviteID               UUID        `json:"invite_id"`
	VaultID                UUID        `json:"vault_id"`
	TargetDeviceID         DeviceID    `json:"target_device_id"`
	TargetDevicePubkeySign Base64Bytes `json:"target_device_pubkey_sign"`
	TargetDevicePubkeyBox  Base64Bytes `json:"target_device_pubkey_box"`
	TargetDeviceBundleSig  Base64Bytes `json:"target_device_bundle_sig"`
	Nonce                  Base64Bytes `json:"nonce"`
	WrappedPayload         Base64Bytes `json:"wrapped_payload"`
	CreatedByDeviceID      DeviceID    `json:"created_by_device_id"`
	SingleUse              bool        `json:"single_use"`
	Signature              Base64Bytes `json:"signature"`
	CreatedAt              string      `json:"created_at,omitempty"`
}

type InviteClaim struct {
	MsgType   string      `json:"msg_type"`
	InviteID  UUID        `json:"invite_id"`
	VaultID   UUID        `json:"vault_id"`
	DeviceID  DeviceID    `json:"device_id"`
	Signature Base64Bytes `json:"signature"`
	CreatedAt string      `json:"created_at,omitempty"`
}

type KeyUpdate struct {
	MsgType           string       `json:"msg_type"`
	KeyUpdateID       UUID         `json:"key_update_id"`
	VaultID           UUID         `json:"vault_id"`
	MemberSeq         Uint64String `json:"member_seq"`
	MemberHeadHash    Base64Bytes  `json:"member_head_hash"`
	TargetDeviceID    DeviceID     `json:"target_device_id"`
	KeyEpoch          Uint64String `json:"key_epoch"`
	Nonce             Base64Bytes  `json:"nonce"`
	WrappedPayload    Base64Bytes  `json:"wrapped_payload"`
	CreatedByDeviceID DeviceID     `json:"created_by_device_id"`
	Signature         Base64Bytes  `json:"signature"`
	CreatedAt         string       `json:"created_at,omitempty"`
}

type KeyUpdateAck struct {
	MsgType        string       `json:"msg_type"`
	VaultID        UUID         `json:"vault_id"`
	DeviceID       DeviceID     `json:"device_id"`
	KeyEpoch       Uint64String `json:"key_epoch"`
	MemberSeq      Uint64String `json:"member_seq"`
	MemberHeadHash Base64Bytes  `json:"member_head_hash"`
	Signature      Base64Bytes  `json:"signature"`
	CreatedAt      string       `json:"created_at,omitempty"`
}

type Snapshot struct {
	MsgType           string       `json:"msg_type"`
	SnapshotID        UUID         `json:"snapshot_id"`
	VaultID           UUID         `json:"vault_id"`
	BaseSeq           Uint64String `json:"base_seq"`
	MemberSeq         Uint64String `json:"member_seq"`
	MemberHeadHash    Base64Bytes  `json:"member_head_hash"`
	BaseCounterMap    Base64Bytes  `json:"base_counter_map"`
	HeadHashMap       Base64Bytes  `json:"head_hash_map"`
	LamportAtSnapshot Uint64String `json:"lamport_at_snapshot"`
	KeyEpoch          Uint64String `json:"key_epoch"`
	Nonce             Base64Bytes  `json:"nonce"`
	Ciphertext        Base64Bytes  `json:"ciphertext"`
	Signature         Base64Bytes  `json:"signature"`
	CreatedByDeviceID DeviceID     `json:"created_by_device_id"`
	CreatedAt         string       `json:"created_at,omitempty"`
}

type VaultMember struct {
	DeviceID         DeviceID     `json:"device_id"`
	DevicePubkeySign Base64Bytes  `json:"device_pubkey_sign"`
	DevicePubkeyBox  Base64Bytes  `json:"device_pubkey_box"`
	KeyEpoch         Uint64String `json:"key_epoch"`
}

type VaultMembershipResponse struct {
	MemberSeq Uint64String  `json:"member_seq"`
	HeadHash  Base64Bytes   `json:"head_hash"`
	Members   []VaultMember `json:"members"`
}

type EventResponse struct {
	Seq Uint64String `json:"seq"`
}

const (
	MaxEventCiphertext    = 65536   // this is 64 KiB
	MaxSnapshotCiphertext = 8388608 // this is 8 MiB
	MaxWrappedPayload     = 1024    // this is 1 KiB
	MaxTags               = 128
	MaxTagLength          = 64
	MaxWebsiteLength      = 2048
	MaxUsernameLength     = 2048
	MaxPasswordLength     = 8192
	MaxNotesLength        = 65535
	MaxSnapshotEntries    = 5000
	MaxMapLength          = 1024

	NonceLength     = 24
	HashLength      = 32
	SignatureLength = 64
	PublicKeyLength = 32
	DeviceIDLength  = 64
)

var (
	ZeroUUID = UUID{}
	Zero32   = make([]byte, 32)
	Zero64   = make([]byte, 64)
)
