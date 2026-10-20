package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrInvalid = errors.New("credential cannot be decrypted with the configured key and binding")

type Envelope struct {
	Version    int    `json:"version"`
	KeyID      string `json:"key_id"`
	WrappedKey []byte `json:"wrapped_key"`
	Ciphertext []byte `json:"ciphertext"`
}

type Binding struct {
	OrgID        string
	ConnectionID string
	Version      int64
}
type Vault struct {
	keyID string
	keys  map[string][]byte
	kms   *kmsBackend
}

func New(keyID string, encodedKeys map[string]string) (*Vault, error) {
	if keyID == "" || len(encodedKeys) == 0 {
		return nil, errors.New("encryption key ID and keyring required")
	}
	keys := make(map[string][]byte, len(encodedKeys))
	for id, encoded := range encodedKeys {
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(key) != 32 {
			return nil, errors.New("encryption keys must be 32-byte base64 values")
		}
		keys[id] = key
	}
	if _, ok := keys[keyID]; !ok {
		return nil, errors.New("current encryption key missing from keyring")
	}
	return &Vault{keyID: keyID, keys: keys}, nil
}

func binding(b Binding, kind string) []byte {
	return []byte(fmt.Sprintf("reforge:v1:%s:%s:%d:%s", b.OrgID, b.ConnectionID, b.Version, kind))
}
func aead(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func seal(key, plain, aad []byte) ([]byte, error) {
	a, err := aead(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plain, aad), nil
}
func open(key, body, aad []byte) ([]byte, error) {
	a, err := aead(key)
	if err != nil || len(body) < a.NonceSize()+a.Overhead() {
		return nil, ErrInvalid
	}
	plain, err := a.Open(nil, body[:a.NonceSize()], body[a.NonceSize():], aad)
	if err != nil {
		return nil, ErrInvalid
	}
	return plain, nil
}

func validBinding(b Binding) bool {
	return b.OrgID != "" && b.ConnectionID != "" && len(b.OrgID) <= 128 && len(b.ConnectionID) <= 128 && !strings.ContainsAny(b.OrgID+b.ConnectionID, ":\x00") && b.Version > 0
}

func (v *Vault) Seal(b Binding, value []byte) (Envelope, error) {
	return v.SealContext(context.Background(), b, value)
}
func (v *Vault) Open(b Binding, e Envelope) ([]byte, error) {
	return v.OpenContext(context.Background(), b, e)
}
func (v *Vault) Rewrap(b Binding, e Envelope) (Envelope, error) {
	return v.RewrapContext(context.Background(), b, e)
}

const maxRecord = 4 << 20

func (v *Vault) SealContext(ctx context.Context, b Binding, value []byte) (Envelope, error) {
	return v.sealWithin(ctx, b, value, 65536)
}

func (v *Vault) SealRecord(ctx context.Context, b Binding, value []byte) (Envelope, error) {
	return v.sealWithin(ctx, b, value, maxRecord)
}

func (v *Vault) sealWithin(ctx context.Context, b Binding, value []byte, limit int) (Envelope, error) {
	if !validBinding(b) || len(value) == 0 || len(value) > limit {
		return Envelope{}, errors.New("invalid credential binding or size")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Envelope{}, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return Envelope{}, err
	}
	defer clear(key)
	ciphertext, err := seal(key, value, binding(b, "value"))
	if err != nil {
		return Envelope{}, err
	}
	wrapped, version, err := v.wrap(ctx, b, key)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{Version: version, KeyID: v.keyID, WrappedKey: wrapped, Ciphertext: ciphertext}, nil
}

func (v *Vault) OpenContext(ctx context.Context, b Binding, e Envelope) ([]byte, error) {
	return v.openWithin(ctx, b, e, 65536)
}

func (v *Vault) OpenRecord(ctx context.Context, b Binding, e Envelope) ([]byte, error) {
	return v.openWithin(ctx, b, e, maxRecord)
}

func (v *Vault) openWithin(ctx context.Context, b Binding, e Envelope, limit int) ([]byte, error) {
	if !validBinding(b) || len(e.Ciphertext) > limit+28 {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := v.unwrap(ctx, b, e)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	return open(key, e.Ciphertext, binding(b, "value"))
}

func (v *Vault) RewrapContext(ctx context.Context, b Binding, e Envelope) (Envelope, error) {
	if !validBinding(b) || len(e.Ciphertext) > 65564 {
		return Envelope{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Envelope{}, err
	}
	key, err := v.unwrap(ctx, b, e)
	if err != nil {
		return Envelope{}, err
	}
	defer clear(key)
	plain, err := open(key, e.Ciphertext, binding(b, "value"))
	if err != nil {
		return Envelope{}, err
	}
	clear(plain)
	wrapped, version, err := v.wrap(ctx, b, key)
	if err != nil {
		return Envelope{}, err
	}
	e.KeyID = v.keyID
	e.Version = version
	e.WrappedKey = wrapped
	return e, nil
}

func (v *Vault) wrap(ctx context.Context, b Binding, key []byte) ([]byte, int, error) {
	if v.kms != nil {
		wrapped, err := v.kms.wrap(ctx, b, v.keyID, key)
		return wrapped, 2, err
	}
	wrapped, err := seal(v.keys[v.keyID], key, binding(b, "key"))
	return wrapped, 1, err
}
func (v *Vault) unwrap(ctx context.Context, b Binding, e Envelope) ([]byte, error) {
	if v.kms != nil {
		if e.Version != 2 {
			return nil, ErrInvalid
		}
		return v.kms.unwrap(ctx, b, e)
	}
	key, ok := v.keys[e.KeyID]
	if !ok || e.Version != 1 {
		return nil, ErrInvalid
	}
	return open(key, e.WrappedKey, binding(b, "key"))
}

func (v *Vault) MarshalJSON() ([]byte, error) { return json.Marshal("[credential vault]") }
func (v *Vault) String() string               { return "[credential vault]" }
func (v *Vault) GoString() string             { return v.String() }
