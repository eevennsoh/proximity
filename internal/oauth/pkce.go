package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// PkcePair contains an OAuth PKCE verifier and S256 challenge.
type PkcePair struct {
	Verifier  string
	Challenge string
}

// GeneratePkce returns a random PKCE verifier and matching S256 challenge.
func GeneratePkce(length int) (PkcePair, error) {
	verifier, err := RandomBase64Url(length)
	if err != nil {
		return PkcePair{}, err
	}

	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])

	return PkcePair{
		Verifier:  verifier,
		Challenge: challenge,
	}, nil
}

// RandomBase64Url returns a URL-safe random string of the requested length.
func RandomBase64Url(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	encoded := base64.RawURLEncoding.EncodeToString(bytes)
	if len(encoded) > length {
		return encoded[:length], nil
	}

	return encoded, nil
}
