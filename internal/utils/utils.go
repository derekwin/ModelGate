package utils

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

func GenerateRandomString(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:n]
}

func GetCurrentTimestamp() int64 {
	return time.Now().Unix()
}

func GetCurrentTimestampMilli() int64 {
	return time.Now().UnixMilli()
}
