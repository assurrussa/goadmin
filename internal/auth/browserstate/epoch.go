package browserstate

import (
	"crypto/sha256"
	"encoding/hex"
)

// SessionEpoch identifies a canonical login without exposing its credential.
// Page bootstrap and maintenance must derive the same fence from that login.
func SessionEpoch(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("goadmin-maintenance-epoch\x00" + sessionID))
	return hex.EncodeToString(digest[:])
}
