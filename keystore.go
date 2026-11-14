package pagetree

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// APIKey — a stored key record (no raw key).
type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`   // "ptk_live_" or "ptk_test_"
	KeyHash    string     `json:"key_hash"` // SHA-256 hex
	Policy     KeyPolicy  `json:"policy"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// KeyPolicy — per-key access policy.
type KeyPolicy struct {
	Pages string `json:"pages"` // "r", "rw", "w", "none"
	News  string `json:"news"`
	Media string `json:"media"`
}

// KeyStore — in-memory canonical key state + file persistence.
type KeyStore struct {
	mu        sync.RWMutex
	keys      map[string]*APIKey // by ID
	path      string             // keys.json path
	master    []byte             // AES-256 key (nil = no encryption)
	prev      []byte             // previous master (for rotation)
	dirty     bool
	lastFlush time.Time
}

// NewKeyStore loads or creates a key store. masterKey may be nil (no encryption).
func NewKeyStore(dataDir string, masterKey, prevKey []byte) (*KeyStore, error) {
	if dataDir == "" {
		return nil, errors.New("pagetree: DataDir is required for KeyStore")
	}
	path := filepath.Join(dataDir, "keys.json")
	ks := &KeyStore{
		keys:   make(map[string]*APIKey),
		path:   path,
		master: masterKey,
		prev:   prevKey,
	}
	if err := ks.load(); err != nil {
		return nil, err
	}
	return ks, nil
}

// load reads keys.json (decrypting if master key present).
func (ks *KeyStore) load() error {
	data, err := os.ReadFile(ks.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // fresh start
		}
		return fmt.Errorf("read keys.json: %w", err)
	}
	if ks.master != nil {
		data, err = decryptAES(data, ks.master, ks.prev)
		if err != nil {
			return fmt.Errorf("decrypt keys.json: %w", err)
		}
	}
	var list struct {
		Keys []*APIKey `json:"keys"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parse keys.json: %w", err)
	}
	for _, k := range list.Keys {
		ks.keys[k.ID] = k
	}
	return nil
}

// flush writes keys.json atomically (encrypting if master key present).
func (ks *KeyStore) flush() error {
	list := struct {
		Keys []*APIKey `json:"keys"`
	}{}
	for _, k := range ks.keys {
		list.Keys = append(list.Keys, k)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if ks.master != nil {
		data, err = encryptAES(data, ks.master)
		if err != nil {
			return fmt.Errorf("encrypt keys.json: %w", err)
		}
	}
	return atomicWrite(ks.path, data, 0o600)
}

// GenerateKey creates a new API key and returns the raw key (shown once).
func (ks *KeyStore) GenerateKey(name string, policy KeyPolicy) (*APIKey, string, error) {
	if name == "" {
		return nil, "", errors.New("имя обязательно")
	}
	// Validate policy
	if err := validatePolicy(policy); err != nil {
		return nil, "", err
	}
	// Generate: 32 random bytes → hex
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", fmt.Errorf("rand: %w", err)
	}
	rawHex := hex.EncodeToString(raw)
	prefix := "ptk_live_"
	rawKey := prefix + rawHex

	hash := sha256.Sum256([]byte(rawKey))
	id := fmt.Sprintf("k_%s", rawHex[:8])
	now := time.Now().UTC()

	key := &APIKey{
		ID:        id,
		Name:      name,
		Prefix:    prefix,
		KeyHash:   hex.EncodeToString(hash[:]),
		Policy:    policy,
		CreatedAt: now,
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.keys[id] = key
	if err := ks.flush(); err != nil {
		delete(ks.keys, id)
		return nil, "", err
	}
	return key, rawKey, nil
}

// Authenticate validates a raw key and returns the matching record.
func (ks *KeyStore) Authenticate(rawKey string) (*APIKey, error) {
	if rawKey == "" {
		return nil, errors.New("пустой ключ")
	}
	hash := sha256.Sum256([]byte(rawKey))
	hashHex := hex.EncodeToString(hash[:])

	ks.mu.RLock()
	defer ks.mu.RUnlock()

	now := time.Now().UTC()
	for _, k := range ks.keys {
		if subtle.ConstantTimeCompare([]byte(k.KeyHash), []byte(hashHex)) != 1 {
			continue
		}
		if k.RevokedAt != nil {
			return nil, errors.New("ключ отозван")
		}
		if k.ExpiresAt != nil && now.After(*k.ExpiresAt) {
			return nil, errors.New("ключ истёк")
		}
		return k, nil
	}
	return nil, errors.New("невалидный ключ")
}

// TouchKey updates last_used_at (throttled, async flush).
func (ks *KeyStore) TouchKey(id string) {
	ks.mu.Lock()
	k, ok := ks.keys[id]
	if !ok {
		ks.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	// Throttle: only update if >60s since last flush
	if now.Sub(ks.lastFlush) > 60*time.Second {
		k.LastUsedAt = &now
		ks.dirty = true
		ks.lastFlush = now
		// Async flush
		go func() {
			ks.mu.Lock()
			if ks.dirty {
				_ = ks.flush()
				ks.dirty = false
			}
			ks.mu.Unlock()
		}()
	}
	ks.mu.Unlock()
}

// ListKeys returns all key records (without raw keys).
func (ks *KeyStore) ListKeys() []*APIKey {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	result := make([]*APIKey, 0, len(ks.keys))
	for _, k := range ks.keys {
		result = append(result, k)
	}
	return result
}

// GetKey returns a key by ID.
func (ks *KeyStore) GetKey(id string) (*APIKey, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	k, ok := ks.keys[id]
	if !ok {
		return nil, fmt.Errorf("ключ %q не найден", id)
	}
	return k, nil
}

// UpdateKey updates name and/or policy.
func (ks *KeyStore) UpdateKey(id string, name string, policy *KeyPolicy) error {
	if policy != nil {
		if err := validatePolicy(*policy); err != nil {
			return err
		}
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()
	k, ok := ks.keys[id]
	if !ok {
		return fmt.Errorf("ключ %q не найден", id)
	}
	if name != "" {
		k.Name = name
	}
	if policy != nil {
		k.Policy = *policy
	}
	return ks.flush()
}

// RevokeKey marks a key as revoked.
func (ks *KeyStore) RevokeKey(id string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	k, ok := ks.keys[id]
	if !ok {
		return fmt.Errorf("ключ %q не найден", id)
	}
	now := time.Now().UTC()
	k.RevokedAt = &now
	return ks.flush()
}

// DeleteKey removes a key permanently.
func (ks *KeyStore) DeleteKey(id string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if _, ok := ks.keys[id]; !ok {
		return fmt.Errorf("ключ %q не найден", id)
	}
	delete(ks.keys, id)
	return ks.flush()
}

// CheckPolicy checks if a key has the required scope for a resource.
func CheckPolicy(policy KeyPolicy, resource string, write bool) bool {
	var scope string
	switch resource {
	case "pages":
		scope = policy.Pages
	case "news":
		scope = policy.News
	case "media":
		scope = policy.Media
	default:
		return false
	}
	switch scope {
	case "rw":
		return true
	case "r":
		return !write
	case "w":
		return write
	default:
		return false
	}
}

// validatePolicy ensures scope values are valid.
func validatePolicy(p KeyPolicy) error {
	for name, scope := range map[string]string{
		"pages": p.Pages,
		"news":  p.News,
		"media": p.Media,
	} {
		switch scope {
		case "r", "rw", "w", "none":
		default:
			return fmt.Errorf("невалидный scope %q для %s (допустимы r/rw/w/none)", scope, name)
		}
	}
	return nil
}

// encryptAES encrypts data with AES-256-GCM (random nonce prefix).
func encryptAES(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, data, nil), nil
}

// decryptAES decrypts data with AES-256-GCM. Tries prev key as fallback.
func decryptAES(data, key, prevKey []byte) ([]byte, error) {
	for _, k := range [][]byte{key, prevKey} {
		if k == nil {
			continue
		}
		block, err := aes.NewCipher(k)
		if err != nil {
			continue
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			continue
		}
		ns := gcm.NonceSize()
		if len(data) < ns {
			continue
		}
		plain, err := gcm.Open(nil, data[:ns], data[ns:], nil)
		if err == nil {
			return plain, nil
		}
	}
	return nil, errors.New("расшифровка не удалась ни одним ключом")
}
