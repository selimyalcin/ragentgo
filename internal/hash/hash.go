package hash

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256 returns a hex-encoded SHA-256 digest of the given parts joined with a
// NUL separator. Used for cache keys so model name and text cannot collide.
func SHA256(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			_, _ = h.Write([]byte{0})
		}
		_, _ = h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}
