package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"forgor/internal/crypto"
	"forgor/internal/models"

	bolt "go.etcd.io/bbolt"
)

var (
	metaBucket    = []byte("meta")
	vaultBucket   = []byte("vault")
	friendsBucket = []byte("friends")

	keySchemaVersion    = []byte("schema_version")
	keyVaultSalt        = []byte("vault_salt")
	keyVaultBlob        = []byte("blob")
	keyDeviceName       = []byte("device_name")
	keyDevicePubKey     = []byte("device_pubkey")
	keyDevicePrivKeyEnc = []byte("device_privkey_enc")
)

const schemaVersion = "1"

type Store struct {
	db       *bolt.DB
	dbPath   string
	vaultKey []byte
	mu       sync.RWMutex
}

func Open(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	s := &Store{db: db, dbPath: dbPath}

	if err := s.initBuckets(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) initBuckets() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{metaBucket, vaultBucket, friendsBucket} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
		}

		meta := tx.Bucket(metaBucket)
		if meta.Get(keySchemaVersion) == nil {
			if err := meta.Put(keySchemaVersion, []byte(schemaVersion)); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Store) IsInitialized() bool {
	var initialized bool
	s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		initialized = meta.Get(keyVaultSalt) != nil
		return nil
	})
	return initialized
}

func (s *Store) Initialize(masterPassword, deviceName string) error {
	salt, err := crypto.GenerateSalt()
	if err != nil {
		return err
	}

	vaultKey := crypto.DeriveKey(masterPassword, salt)

	pub, priv, err := crypto.GenerateBoxKeyPair()
	if err != nil {
		return err
	}

	privKeyEnc, err := crypto.Encrypt(vaultKey, priv[:])
	if err != nil {
		return err
	}

	emptyVault, err := json.Marshal([]models.Entry{})
	if err != nil {
		return err
	}

	encryptedVault, err := crypto.Encrypt(vaultKey, emptyVault)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		vault := tx.Bucket(vaultBucket)

		if err := meta.Put(keyVaultSalt, salt); err != nil {
			return err
		}
		if err := meta.Put(keyDeviceName, []byte(deviceName)); err != nil {
			return err
		}
		if err := meta.Put(keyDevicePubKey, pub[:]); err != nil {
			return err
		}
		if err := meta.Put(keyDevicePrivKeyEnc, privKeyEnc); err != nil {
			return err
		}
		if err := vault.Put(keyVaultBlob, encryptedVault); err != nil {
			return err
		}

		return nil
	})
}

func (s *Store) Unlock(masterPassword string) ([]models.Entry, error) {
	var salt, encryptedVault []byte

	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		vault := tx.Bucket(vaultBucket)

		salt = copyBytes(meta.Get(keyVaultSalt))
		encryptedVault = copyBytes(vault.Get(keyVaultBlob))
		return nil
	})
	if err != nil {
		return nil, err
	}

	if salt == nil || encryptedVault == nil {
		return nil, fmt.Errorf("vault not initialized")
	}

	vaultKey := crypto.DeriveKey(masterPassword, salt)

	plaintext, err := crypto.Decrypt(vaultKey, encryptedVault)
	if err != nil {
		return nil, err
	}

	var entries []models.Entry
	if err := json.Unmarshal(plaintext, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse vault: %w", err)
	}

	s.mu.Lock()
	s.vaultKey = vaultKey
	s.mu.Unlock()
	return entries, nil
}

func (s *Store) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.vaultKey {
		s.vaultKey[i] = 0
	}
	s.vaultKey = nil
}

func (s *Store) IsUnlocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vaultKey != nil
}

func (s *Store) SaveEntries(entries []models.Entry) error {
	s.mu.RLock()
	if s.vaultKey == nil {
		s.mu.RUnlock()
		return fmt.Errorf("vault is locked")
	}
	// Copy the key bytes, not just the slice header
	vaultKey := make([]byte, len(s.vaultKey))
	copy(vaultKey, s.vaultKey)
	s.mu.RUnlock()

	plaintext, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("failed to serialize entries: %w", err)
	}

	ciphertext, err := crypto.Encrypt(vaultKey, plaintext)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		vault := tx.Bucket(vaultBucket)
		return vault.Put(keyVaultBlob, ciphertext)
	})
}

func (s *Store) GetDevice() (*models.Device, error) {
	s.mu.RLock()
	if s.vaultKey == nil {
		s.mu.RUnlock()
		return nil, fmt.Errorf("vault is locked")
	}
	vaultKey := make([]byte, len(s.vaultKey))
	copy(vaultKey, s.vaultKey)
	s.mu.RUnlock()

	var device models.Device
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)

		nameBytes := meta.Get(keyDeviceName)
		if nameBytes != nil {
			device.Name = string(nameBytes)
		}

		pubKeyBytes := meta.Get(keyDevicePubKey)
		if len(pubKeyBytes) == 32 {
			copy(device.PubKey[:], pubKeyBytes)
		}

		privKeyEnc := meta.Get(keyDevicePrivKeyEnc)
		if privKeyEnc != nil {
			privKeyPlain, err := crypto.Decrypt(vaultKey, privKeyEnc)
			if err != nil {
				return fmt.Errorf("failed to decrypt device private key: %w", err)
			}
			if len(privKeyPlain) == 32 {
				copy(device.PrivKey[:], privKeyPlain)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &device, nil
}

func (s *Store) SaveFriend(friend models.Friend) error {
	data, err := json.Marshal(friend)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		friends := tx.Bucket(friendsBucket)
		return friends.Put([]byte("friend:"+friend.Fingerprint), data)
	})
}

func (s *Store) GetFriend(fingerprint string) (*models.Friend, error) {
	var friend models.Friend
	err := s.db.View(func(tx *bolt.Tx) error {
		friends := tx.Bucket(friendsBucket)
		data := friends.Get([]byte("friend:" + fingerprint))
		if data == nil {
			return fmt.Errorf("friend not found")
		}
		return json.Unmarshal(data, &friend)
	})
	if err != nil {
		return nil, err
	}
	return &friend, nil
}

func (s *Store) GetAllFriends() ([]models.Friend, error) {
	var friends []models.Friend
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(friendsBucket)
		return bucket.ForEach(func(k, v []byte) error {
			var friend models.Friend
			if err := json.Unmarshal(v, &friend); err != nil {
				return err
			}
			friends = append(friends, friend)
			return nil
		})
	})
	return friends, err
}

func (s *Store) DeleteFriend(fingerprint string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		friends := tx.Bucket(friendsBucket)
		return friends.Delete([]byte("friend:" + fingerprint))
	})
}

func (s *Store) UpdateDeviceName(name string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		return meta.Put(keyDeviceName, []byte(name))
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
