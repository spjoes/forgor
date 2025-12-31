package sync

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"forgor/internal/crypto"
	"forgor/internal/models"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/curve25519"
)

var (
	syncMetaBucket       = []byte("sync_meta")
	syncMembersBucket    = []byte("sync_members")
	syncEventHeadsBucket = []byte("sync_event_heads")
	syncPendingBucket    = []byte("sync_pending")

	keyVaultID        = []byte("vault_id")
	keyDeviceID       = []byte("device_id")
	keyPubkeySign     = []byte("pubkey_sign")
	keyPrivkeySignEnc = []byte("privkey_sign_enc")
	keyPubkeyBox      = []byte("pubkey_box")
	keyPrivkeyBoxEnc  = []byte("privkey_box_enc")
	keyVaultKeyEnc    = []byte("vault_key_enc")
	keyKeyEpoch       = []byte("key_epoch")
	keyOwnerDeviceID  = []byte("owner_device_id")
	keyMemberSeq      = []byte("member_seq")
	keyMemberHeadHash = []byte("member_head_hash")
	keySyncCursor     = []byte("sync_cursor")
	keyLamport        = []byte("lamport")
	keyServerURL      = []byte("server_url")
)

type DeviceKeys struct {
	DeviceID    DeviceID
	PubkeySign  [32]byte
	PrivkeySign [64]byte
	PubkeyBox   [32]byte
	PrivkeyBox  [32]byte
}

type MembershipHead struct {
	MemberSeq      uint64
	MemberHeadHash [32]byte
}

type EventHead struct {
	LastCounter uint64
	LastHash    [32]byte
}

type VerifiedMember struct {
	DeviceID   DeviceID    `json:"device_id"`
	PubkeySign Base64Bytes `json:"pubkey_sign"`
	PubkeyBox  Base64Bytes `json:"pubkey_box"`
	KeyEpoch   uint64      `json:"key_epoch"`
}

type PendingEntry struct {
	Op    string       `json:"op"`
	Entry models.Entry `json:"entry"`
}

type SyncState struct {
	db       *bolt.DB
	vaultKey []byte
	mu       sync.RWMutex
}

func NewSyncState(db *bolt.DB, vaultKey []byte) (*SyncState, error) {
	s := &SyncState{
		db:       db,
		vaultKey: make([]byte, len(vaultKey)),
	}
	copy(s.vaultKey, vaultKey)

	if err := s.initBuckets(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *SyncState) initBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{syncMetaBucket, syncMembersBucket, syncEventHeadsBucket, syncPendingBucket} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
		}
		return nil
	})
}

func (s *SyncState) getVaultKey() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.vaultKey == nil {
		return nil, fmt.Errorf("sync state not initialized")
	}
	key := make([]byte, len(s.vaultKey))
	copy(key, s.vaultKey)
	return key, nil
}

func (s *SyncState) IsConfigured() bool {
	var configured bool
	s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		if meta == nil {
			return nil
		}
		configured = meta.Get(keyVaultID) != nil && meta.Get(keyDeviceID) != nil
		return nil
	})
	return configured
}

func (s *SyncState) GetVaultID() (UUID, error) {
	var vaultID UUID
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		data := meta.Get(keyVaultID)
		if data == nil {
			return fmt.Errorf("vault_id not set")
		}
		if len(data) != 16 {
			return fmt.Errorf("invalid vault_id length")
		}
		copy(vaultID[:], data)
		return nil
	})
	return vaultID, err
}

func (s *SyncState) SetVaultID(vaultID UUID) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		return meta.Put(keyVaultID, vaultID.Bytes())
	})
}

func (s *SyncState) GetDeviceKeys() (*DeviceKeys, error) {
	vaultKey, err := s.getVaultKey()
	if err != nil {
		return nil, err
	}

	var keys DeviceKeys
	err = s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)

		deviceIDBytes := meta.Get(keyDeviceID)
		if deviceIDBytes == nil {
			return fmt.Errorf("device_id not set")
		}
		keys.DeviceID = DeviceID(deviceIDBytes)

		pubkeySign := meta.Get(keyPubkeySign)
		if len(pubkeySign) != 32 {
			return fmt.Errorf("invalid pubkey_sign length")
		}
		copy(keys.PubkeySign[:], pubkeySign)

		privkeySignEnc := meta.Get(keyPrivkeySignEnc)
		if privkeySignEnc == nil {
			return fmt.Errorf("privkey_sign not set")
		}
		privkeySign, err := crypto.Decrypt(vaultKey, privkeySignEnc)
		if err != nil {
			return fmt.Errorf("failed to decrypt privkey_sign: %w", err)
		}
		if len(privkeySign) != 64 {
			return fmt.Errorf("invalid privkey_sign length")
		}
		copy(keys.PrivkeySign[:], privkeySign)

		pubkeyBox := meta.Get(keyPubkeyBox)
		if len(pubkeyBox) != 32 {
			return fmt.Errorf("invalid pubkey_box length")
		}
		copy(keys.PubkeyBox[:], pubkeyBox)

		privkeyBoxEnc := meta.Get(keyPrivkeyBoxEnc)
		if privkeyBoxEnc == nil {
			return fmt.Errorf("privkey_box not set")
		}
		privkeyBox, err := crypto.Decrypt(vaultKey, privkeyBoxEnc)
		if err != nil {
			return fmt.Errorf("failed to decrypt privkey_box: %w", err)
		}
		if len(privkeyBox) != 32 {
			return fmt.Errorf("invalid privkey_box length")
		}
		copy(keys.PrivkeyBox[:], privkeyBox)

		return nil
	})

	return &keys, err
}

func (s *SyncState) SetDeviceKeys(keys *DeviceKeys) error {
	vaultKey, err := s.getVaultKey()
	if err != nil {
		return err
	}

	privkeySignEnc, err := crypto.Encrypt(vaultKey, keys.PrivkeySign[:])
	if err != nil {
		return fmt.Errorf("failed to encrypt privkey_sign: %w", err)
	}

	privkeyBoxEnc, err := crypto.Encrypt(vaultKey, keys.PrivkeyBox[:])
	if err != nil {
		return fmt.Errorf("failed to encrypt privkey_box: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)

		if err := meta.Put(keyDeviceID, []byte(keys.DeviceID)); err != nil {
			return err
		}
		if err := meta.Put(keyPubkeySign, keys.PubkeySign[:]); err != nil {
			return err
		}
		if err := meta.Put(keyPrivkeySignEnc, privkeySignEnc); err != nil {
			return err
		}
		if err := meta.Put(keyPubkeyBox, keys.PubkeyBox[:]); err != nil {
			return err
		}
		if err := meta.Put(keyPrivkeyBoxEnc, privkeyBoxEnc); err != nil {
			return err
		}

		return nil
	})
}

func (s *SyncState) GetVaultKey() ([32]byte, error) {
	var key [32]byte
	vaultKey, err := s.getVaultKey()
	if err != nil {
		return key, err
	}

	err = s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		encKey := meta.Get(keyVaultKeyEnc)
		if encKey == nil {
			return fmt.Errorf("vault_key not set")
		}
		decKey, err := crypto.Decrypt(vaultKey, encKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt vault_key: %w", err)
		}
		if len(decKey) != 32 {
			return fmt.Errorf("invalid vault_key length")
		}
		copy(key[:], decKey)
		return nil
	})

	return key, err
}

func (s *SyncState) SetVaultKey(key [32]byte) error {
	vaultKey, err := s.getVaultKey()
	if err != nil {
		return err
	}

	encKey, err := crypto.Encrypt(vaultKey, key[:])
	if err != nil {
		return fmt.Errorf("failed to encrypt vault_key: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		return meta.Put(keyVaultKeyEnc, encKey)
	})
}

func (s *SyncState) GetKeyEpoch() (uint64, error) {
	var epoch uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		data := meta.Get(keyKeyEpoch)
		if data == nil {
			return nil
		}
		epoch = binary.BigEndian.Uint64(data)
		return nil
	})
	return epoch, err
}

func (s *SyncState) SetKeyEpoch(epoch uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, epoch)
		return meta.Put(keyKeyEpoch, buf)
	})
}

func (s *SyncState) GetOwnerDeviceID() (DeviceID, error) {
	var deviceID DeviceID
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		data := meta.Get(keyOwnerDeviceID)
		if data == nil {
			return fmt.Errorf("owner_device_id not set")
		}
		deviceID = DeviceID(data)
		return nil
	})
	return deviceID, err
}

func (s *SyncState) SetOwnerDeviceID(deviceID DeviceID) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		return meta.Put(keyOwnerDeviceID, []byte(deviceID))
	})
}

func (s *SyncState) GetMembershipHead() (*MembershipHead, error) {
	var head MembershipHead
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)

		seqData := meta.Get(keyMemberSeq)
		if seqData == nil {
			return nil
		}
		head.MemberSeq = binary.BigEndian.Uint64(seqData)

		hashData := meta.Get(keyMemberHeadHash)
		if len(hashData) == 32 {
			copy(head.MemberHeadHash[:], hashData)
		}

		return nil
	})
	return &head, err
}

func (s *SyncState) SetMembershipHead(head *MembershipHead) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)

		seqBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(seqBuf, head.MemberSeq)
		if err := meta.Put(keyMemberSeq, seqBuf); err != nil {
			return err
		}

		return meta.Put(keyMemberHeadHash, head.MemberHeadHash[:])
	})
}

func (s *SyncState) GetEventHead(deviceID DeviceID) (*EventHead, error) {
	var head EventHead
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncEventHeadsBucket)
		data := bucket.Get([]byte(deviceID))
		if data == nil {
			return nil
		}
		if len(data) != 40 {
			return fmt.Errorf("invalid event head data length")
		}
		head.LastCounter = binary.BigEndian.Uint64(data[:8])
		copy(head.LastHash[:], data[8:40])
		return nil
	})
	return &head, err
}

func (s *SyncState) SetEventHead(deviceID DeviceID, head *EventHead) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncEventHeadsBucket)
		data := make([]byte, 40)
		binary.BigEndian.PutUint64(data[:8], head.LastCounter)
		copy(data[8:40], head.LastHash[:])
		return bucket.Put([]byte(deviceID), data)
	})
}

func (s *SyncState) GetSyncCursor() (uint64, error) {
	var cursor uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		data := meta.Get(keySyncCursor)
		if data == nil {
			return nil
		}
		cursor = binary.BigEndian.Uint64(data)
		return nil
	})
	return cursor, err
}

func (s *SyncState) SetSyncCursor(cursor uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, cursor)
		return meta.Put(keySyncCursor, buf)
	})
}

func (s *SyncState) GetLamport() (uint64, error) {
	var lamport uint64
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		data := meta.Get(keyLamport)
		if data == nil {
			return nil
		}
		lamport = binary.BigEndian.Uint64(data)
		return nil
	})
	return lamport, err
}

func (s *SyncState) IncrementLamport() (uint64, error) {
	var newLamport uint64
	err := s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)

		data := meta.Get(keyLamport)
		var current uint64
		if data != nil {
			current = binary.BigEndian.Uint64(data)
		}

		newLamport = current + 1
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, newLamport)
		return meta.Put(keyLamport, buf)
	})
	return newLamport, err
}

func (s *SyncState) UpdateLamport(observed uint64) (uint64, error) {
	var newLamport uint64
	err := s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)

		data := meta.Get(keyLamport)
		var current uint64
		if data != nil {
			current = binary.BigEndian.Uint64(data)
		}

		if observed > current {
			newLamport = observed + 1
		} else {
			newLamport = current + 1
		}

		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, newLamport)
		return meta.Put(keyLamport, buf)
	})
	return newLamport, err
}

func (s *SyncState) GetServerURL() (string, error) {
	var url string
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		data := meta.Get(keyServerURL)
		if data != nil {
			url = string(data)
		}
		return nil
	})
	return url, err
}

func (s *SyncState) SetServerURL(url string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		return meta.Put(keyServerURL, []byte(url))
	})
}

func (s *SyncState) GetVerifiedMembers() ([]VerifiedMember, error) {
	var members []VerifiedMember
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncMembersBucket)
		return bucket.ForEach(func(k, v []byte) error {
			var member VerifiedMember
			if err := json.Unmarshal(v, &member); err != nil {
				return fmt.Errorf("failed to unmarshal member %s: %w", k, err)
			}
			members = append(members, member)
			return nil
		})
	})
	return members, err
}

func (s *SyncState) GetVerifiedMember(deviceID DeviceID) (*VerifiedMember, error) {
	var member VerifiedMember
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncMembersBucket)
		data := bucket.Get([]byte(deviceID))
		if data == nil {
			return fmt.Errorf("member not found: %s", deviceID)
		}
		return json.Unmarshal(data, &member)
	})
	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (s *SyncState) SetVerifiedMember(member *VerifiedMember) error {
	data, err := json.Marshal(member)
	if err != nil {
		return fmt.Errorf("failed to marshal member: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncMembersBucket)
		return bucket.Put([]byte(member.DeviceID), data)
	})
}

func (s *SyncState) RemoveVerifiedMember(deviceID DeviceID) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncMembersBucket)
		return bucket.Delete([]byte(deviceID))
	})
}

func (s *SyncState) ClearVerifiedMembers() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(syncMembersBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucket(syncMembersBucket)
		return err
	})
}

func (s *SyncState) AddPendingEntry(op string, entry models.Entry) error {
	if entry.ID == "" {
		return fmt.Errorf("entry id is required")
	}
	vaultKey, err := s.getVaultKey()
	if err != nil {
		return err
	}
	data, err := json.Marshal(PendingEntry{Op: op, Entry: entry})
	if err != nil {
		return fmt.Errorf("failed to marshal pending entry: %w", err)
	}
	enc, err := crypto.Encrypt(vaultKey, data)
	if err != nil {
		return fmt.Errorf("failed to encrypt pending entry: %w", err)
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncPendingBucket)
		if bucket == nil {
			return fmt.Errorf("pending bucket not initialized")
		}
		return bucket.Put([]byte(entry.ID), enc)
	})
}

func (s *SyncState) RemovePendingEntry(entryID string) error {
	if entryID == "" {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncPendingBucket)
		if bucket == nil {
			return nil
		}
		return bucket.Delete([]byte(entryID))
	})
}

func (s *SyncState) GetPendingEntries() ([]PendingEntry, error) {
	var pending []PendingEntry
	vaultKey, err := s.getVaultKey()
	if err != nil {
		return nil, err
	}
	err = s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncPendingBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, v []byte) error {
			var entry PendingEntry
			dec, err := crypto.Decrypt(vaultKey, v)
			if err != nil {
				return fmt.Errorf("failed to decrypt pending entry: %w", err)
			}
			if err := json.Unmarshal(dec, &entry); err != nil {
				return fmt.Errorf("failed to unmarshal pending entry: %w", err)
			}
			pending = append(pending, entry)
			return nil
		})
	})
	return pending, err
}

func (s *SyncState) ClearPendingEntries() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(syncPendingBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucket(syncPendingBucket)
		return err
	})
}

func (s *SyncState) ClearEventHeads() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(syncEventHeadsBucket)
		if bucket == nil {
			return nil
		}

		var keys [][]byte
		if err := bucket.ForEach(func(k, _ []byte) error {
			keyCopy := make([]byte, len(k))
			copy(keyCopy, k)
			keys = append(keys, keyCopy)
			return nil
		}); err != nil {
			return err
		}

		for _, key := range keys {
			if err := bucket.Delete(key); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *SyncState) ClearVaultState() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(syncMetaBucket)
		if meta == nil {
			return nil
		}

		keys := [][]byte{
			keyVaultID,
			keyVaultKeyEnc,
			keyKeyEpoch,
			keyOwnerDeviceID,
			keyMemberSeq,
			keyMemberHeadHash,
			keySyncCursor,
			keyLamport,
		}
		for _, key := range keys {
			if err := meta.Delete(key); err != nil {
				return err
			}
		}

		return nil
	})
}

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

func GenerateDeviceKeys() (*DeviceKeys, error) {
	pubSign, privSign, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate sign keypair: %w", err)
	}

	var privBox [32]byte
	if _, err := rand.Read(privBox[:]); err != nil {
		return nil, fmt.Errorf("failed to generate box private key: %w", err)
	}

	var pubBox [32]byte
	curve25519.ScalarBaseMult(&pubBox, &privBox)

	deviceIDHash := sha256.Sum256(pubSign)
	deviceID := DeviceID(hex.EncodeToString(deviceIDHash[:]))

	keys := &DeviceKeys{
		DeviceID:   deviceID,
		PrivkeyBox: privBox,
		PubkeyBox:  pubBox,
	}
	copy(keys.PubkeySign[:], pubSign)
	copy(keys.PrivkeySign[:], privSign)

	return keys, nil
}
