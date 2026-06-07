package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeneratePkceReturnsVerifierAndS256Challenge verifies PKCE values are usable for OAuth.
func TestGeneratePkceReturnsVerifierAndS256Challenge(t *testing.T) {
	pair, err := GeneratePkce(43)

	require.NoError(t, err)
	assert.Len(t, pair.Verifier, 43)
	assert.NotEmpty(t, pair.Challenge)

	challengeBytes := sha256.Sum256([]byte(pair.Verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	assert.Equal(t, challenge, pair.Challenge)
}
