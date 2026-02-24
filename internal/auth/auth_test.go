package auth

import (
	"testing"
)

func TestHashAPIKey(t *testing.T) {
	key := "test-api-key-12345"
	hashed := HashAPIKey(key)

	if hashed == "" {
		t.Error("HashAPIKey() returned empty string")
	}

	expected := "2688f4e126ca5efd4a60022073e6cd90017626e56c3f30b194d53e6299edfe3c"
	if hashed != expected {
		t.Errorf("HashAPIKey() = %s, want %s", hashed, expected)
	}
}

func TestHashAPIKeyConsistency(t *testing.T) {
	key := "consistent-key-test"
	hash1 := HashAPIKey(key)
	hash2 := HashAPIKey(key)

	if hash1 != hash2 {
		t.Error("HashAPIKey() is not deterministic")
	}
}

func TestHashAPIKeyEmpty(t *testing.T) {
	key := ""
	hashed := HashAPIKey(key)
	
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if hashed != expected {
		t.Errorf("HashAPIKey(empty) = %s, want %s", hashed, expected)
	}
}

func TestHashAPIKey_SHA256(t *testing.T) {
	key := "test"
	hashed := HashAPIKey(key)
	
	expected := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	if hashed != expected {
		t.Errorf("HashAPIKey(test) = %s, want %s", hashed, expected)
	}
}
