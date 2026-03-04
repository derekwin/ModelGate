package adapters

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

func generateID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "fallbackid"
	}
	return hex.EncodeToString(buf)
}

func getTimestamp() int64 {
	return time.Now().Unix()
}
