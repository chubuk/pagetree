package pagetree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestKeystore(t *testing.T) *KeyStore {
	t.Helper()
	dir := t.TempDir()
	ks, err := NewKeyStore(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ks
}

func TestGenerateKey(t *testing.T) {
	ks := newTestKeystore(t)
	policy := KeyPolicy{Pages: "rw", News: "r", Media: "w"}
	key, raw, err := ks.GenerateKey("test-agent", policy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "ptk_live_") {
		t.Errorf("raw key = %q, want prefix ptk_live_", raw)
	}
	if len(raw) != len("ptk_live_")+64 {
		t.Errorf("raw key length = %d, want %d", len(raw), len("ptk_live_")+64)
	}
	if key.KeyHash == "" {
		t.Error("KeyHash should not be empty")
	}
	if key.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", key.Name, "test-agent")
	}
	// Hash should NOT be the raw key
	if key.KeyHash == raw {
		t.Error("KeyHash should not equal raw key")
	}
}

func TestAuthenticateKey(t *testing.T) {
	ks := newTestKeystore(t)
	policy := KeyPolicy{Pages: "rw", News: "rw", Media: "rw"}
	_, raw, err := ks.GenerateKey("auth-test", policy)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ks.Authenticate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if key.Name != "auth-test" {
		t.Errorf("Name = %q, want %q", key.Name, "auth-test")
	}
}

func TestAuthenticateBadKey(t *testing.T) {
	ks := newTestKeystore(t)
	_, err := ks.Authenticate("ptk_live_deadbeef")
	if err == nil {
		t.Error("expected error for bad key")
	}
}

func TestAuthenticateEmpty(t *testing.T) {
	ks := newTestKeystore(t)
	_, err := ks.Authenticate("")
	if err == nil {
		t.Error("expected error for empty key")
	}
}

func TestRevokeKey(t *testing.T) {
	ks := newTestKeystore(t)
	policy := KeyPolicy{Pages: "rw", News: "rw", Media: "rw"}
	key, raw, err := ks.GenerateKey("revoke-test", policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.RevokeKey(key.ID); err != nil {
		t.Fatal(err)
	}
	_, err = ks.Authenticate(raw)
	if err == nil {
		t.Error("expected error for revoked key")
	}
}

func TestExpiredKey(t *testing.T) {
	ks := newTestKeystore(t)
	policy := KeyPolicy{Pages: "rw", News: "rw", Media: "rw"}
	key, raw, err := ks.GenerateKey("expire-test", policy)
	if err != nil {
		t.Fatal(err)
	}
	// Set expiry in the past
	past := key.CreatedAt.Add(-1 * 3600000000000) // -1 hour
	ks.mu.Lock()
	ks.keys[key.ID].ExpiresAt = &past
	ks.mu.Unlock()

	_, err = ks.Authenticate(raw)
	if err == nil {
		t.Error("expected error for expired key")
	}
}

func TestDeleteKey(t *testing.T) {
	ks := newTestKeystore(t)
	policy := KeyPolicy{Pages: "rw", News: "rw", Media: "rw"}
	key, _, err := ks.GenerateKey("delete-test", policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := ks.DeleteKey(key.ID); err != nil {
		t.Fatal(err)
	}
	_, err = ks.GetKey(key.ID)
	if err == nil {
		t.Error("expected error for deleted key")
	}
}

func TestUpdateKey(t *testing.T) {
	ks := newTestKeystore(t)
	policy := KeyPolicy{Pages: "rw", News: "r", Media: "none"}
	key, _, err := ks.GenerateKey("update-test", policy)
	if err != nil {
		t.Fatal(err)
	}
	newPolicy := KeyPolicy{Pages: "r", News: "r", Media: "r"}
	if err := ks.UpdateKey(key.ID, "renamed", &newPolicy); err != nil {
		t.Fatal(err)
	}
	updated, _ := ks.GetKey(key.ID)
	if updated.Name != "renamed" {
		t.Errorf("Name = %q, want renamed", updated.Name)
	}
	if updated.Policy.Pages != "r" {
		t.Errorf("Policy.Pages = %q, want r", updated.Policy.Pages)
	}
}

func TestCheckPolicy(t *testing.T) {
	policy := KeyPolicy{Pages: "rw", News: "r", Media: "w"}
	tests := []struct {
		resource string
		write    bool
		want     bool
	}{
		{"pages", false, true},
		{"pages", true, true},
		{"news", false, true},
		{"news", true, false},   // news is read-only
		{"media", false, false}, // media is write-only
		{"media", true, true},
		{"unknown", true, false},
	}
	for _, tt := range tests {
		got := CheckPolicy(policy, tt.resource, tt.write)
		if got != tt.want {
			t.Errorf("CheckPolicy(%q, write=%v) = %v, want %v", tt.resource, tt.write, got, tt.want)
		}
	}
}

func TestValidatePolicy(t *testing.T) {
	// Valid
	err := validatePolicy(KeyPolicy{Pages: "rw", News: "r", Media: "none"})
	if err != nil {
		t.Errorf("valid policy rejected: %v", err)
	}
	// Invalid
	err = validatePolicy(KeyPolicy{Pages: "x", News: "r", Media: "r"})
	if err == nil {
		t.Error("invalid policy accepted")
	}
}

func TestEncryptedKeystore(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	ks, err := NewKeyStore(dir, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := KeyPolicy{Pages: "rw", News: "rw", Media: "rw"}
	_, raw, err := ks.GenerateKey("encrypted", policy)
	if err != nil {
		t.Fatal(err)
	}
	// Reload with same key
	ks2, err := NewKeyStore(dir, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ks2.Authenticate(raw)
	if err != nil {
		t.Fatalf("authenticate after reload: %v", err)
	}
	// File should be encrypted (not plain JSON)
	data, _ := os.ReadFile(filepath.Join(dir, "keys.json"))
	if strings.Contains(string(data), "encrypted") {
		t.Error("keys.json should be encrypted (contains plaintext)")
	}
}

func TestEncryptedKeystoreWrongKey(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	wrongKey := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
		wrongKey[i] = byte(255 - i)
	}
	ks, err := NewKeyStore(dir, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := KeyPolicy{Pages: "rw", News: "rw", Media: "rw"}
	_, _, err = ks.GenerateKey("enc-test", policy)
	if err != nil {
		t.Fatal(err)
	}
	// Try with wrong key — should fail
	_, err = NewKeyStore(dir, wrongKey, nil)
	if err == nil {
		t.Error("expected error with wrong master key")
	}
}
