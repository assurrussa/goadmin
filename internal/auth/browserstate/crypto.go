package browserstate

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/assurrussa/goauth"
)

type sealedRecord struct {
	Version    int64  `json:"version"`
	Owner      string `json:"owner"`
	KeyID      string `json:"keyId"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}
type codec struct{ keys goauth.KeyRing }

func (c codec) aead(keyID string) (cipher.AEAD, error) {
	key, err := c.keys.Get(keyID)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key.Material)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func associatedData(id string, v int64) []byte {
	return []byte("goadmin-browser-auth-v1\x00" + id + "\x00" + strconv.FormatInt(v, 10))
}

func (c codec) encode(id string, r Record) ([]byte, error) {
	aead, err := c.aead(c.keys.ActiveKeyID())
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	// Mutable ownership is a server concurrency marker; credentials and their
	// version/opaque-session binding are authenticated independently of it.
	plain := r
	plain.Owner = ""
	raw, err := json.Marshal(plain)
	if err != nil {
		return nil, err
	}
	sealed := sealedRecord{
		Version: r.Version, Owner: r.Owner, KeyID: c.keys.ActiveKeyID(), Nonce: nonce,
		Ciphertext: aead.Seal(nil, nonce, raw, associatedData(id, r.Version)),
	}
	return json.Marshal(sealed)
}

func (c codec) decode(id string, raw []byte) (Record, error) {
	if len(raw) > 64*1024 {
		return Record{}, errors.New("oversized admin auth state")
	}
	var sealed sealedRecord
	if err := json.Unmarshal(raw, &sealed); err != nil {
		return Record{}, err
	}
	aead, err := c.aead(sealed.KeyID)
	if err != nil {
		return Record{}, err
	}
	if len(sealed.Nonce) != aead.NonceSize() {
		return Record{}, errors.New("invalid admin auth nonce")
	}
	plain, err := aead.Open(nil, sealed.Nonce, sealed.Ciphertext, associatedData(id, sealed.Version))
	if err != nil {
		return Record{}, errors.New("invalid admin auth ciphertext")
	}
	var r Record
	if err := json.Unmarshal(plain, &r); err != nil {
		return Record{}, err
	}
	if r.Version != sealed.Version {
		return Record{}, errors.New("invalid admin auth version")
	}
	r.Owner = sealed.Owner
	return r, nil
}
