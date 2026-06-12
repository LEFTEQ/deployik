package push

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// VAPIDKeys identifies this server to push services. Generated once on first
// boot and persisted — rotating the keys silently invalidates every existing
// subscription, so the file must survive restarts (it lives in DATA_DIR next
// to the SQLite database).
type VAPIDKeys struct {
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

// LoadOrCreateVAPIDKeys reads keys from dataDir/vapid.json, generating and
// persisting a fresh pair when the file doesn't exist yet.
func LoadOrCreateVAPIDKeys(dataDir string) (*VAPIDKeys, error) {
	path := filepath.Join(dataDir, "vapid.json")

	data, err := os.ReadFile(path)
	if err == nil {
		keys := &VAPIDKeys{}
		if err := json.Unmarshal(data, keys); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if keys.PublicKey == "" || keys.PrivateKey == "" {
			return nil, fmt.Errorf("%s is missing key material", path)
		}
		return keys, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return nil, fmt.Errorf("generate VAPID keys: %w", err)
	}
	keys := &VAPIDKeys{PublicKey: publicKey, PrivateKey: privateKey}

	data, err = json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal VAPID keys: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}
	return keys, nil
}
