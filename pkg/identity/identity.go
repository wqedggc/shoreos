// Package identity contains provider-neutral primitives shared by ShoreOS modules.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	ProviderWechatMini = "wechat_mini"
	bcryptCost         = 10
)

// HashToken returns the database-safe digest of an opaque bearer token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewToken creates an opaque token suitable for a bearer session or a one-time binding code.
func NewToken() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}

// HashPassword creates a bcrypt password hash. Plain-text passwords are never persisted.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword supports the pre-existing SHA-256 format only long enough to upgrade it
// after a successful local login. New passwords always use bcrypt.
func CheckPassword(plain, stored string) (matched bool, needsUpgrade bool) {
	if strings.HasPrefix(stored, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(plain)) == nil, false
	}
	legacy := HashToken(plain)
	return subtle.ConstantTimeCompare([]byte(legacy), []byte(stored)) == 1, true
}
