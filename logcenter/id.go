package logcenter

import (
	"crypto/rand"
	"encoding/base64"
)

func newID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + fallbackID()
	}
	return prefix + base64.RawURLEncoding.EncodeToString(bytes[:])
}
