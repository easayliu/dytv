package crypto

import (
	"crypto/md5"
	"encoding/hex"
	"time"
)

const (
	// DefaultDID is the default device ID for Douyu API
	DefaultDID = "10000000000000000000000000001501"
	// DefaultVer is a fallback version
	DefaultVer = "220120240705"
)

// SignParams holds the parameters needed for signing
type SignParams struct {
	RoomID string
	DID    string
	Time   int64
	Ver    string
}

// NewSignParams creates a new SignParams with default values
func NewSignParams(roomID string) *SignParams {
	return &SignParams{
		RoomID: roomID,
		DID:    DefaultDID,
		Time:   time.Now().Unix(),
		Ver:    DefaultVer,
	}
}

// MD5Hash computes MD5 hash of a string
func MD5Hash(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
