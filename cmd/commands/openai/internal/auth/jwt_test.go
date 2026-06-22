package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractAccountId verifies all OpenCode-compatible account ID claim locations.
func TestExtractAccountId(t *testing.T) {
	tests := []struct {
		name   string
		claims map[string]any
		want   string
	}{
		{
			name: "chatgpt account claim",
			claims: map[string]any{
				"chatgpt_account_id": "account-direct",
			},
			want: "account-direct",
		},
		{
			name: "namespaced account claim",
			claims: map[string]any{
				"https://api.openai.com/auth.chatgpt_account_id": "account-namespaced",
			},
			want: "account-namespaced",
		},
		{
			name: "first organization id",
			claims: map[string]any{
				"organizations": []any{
					map[string]any{
						"id": "org-123",
					},
				},
			},
			want: "org-123",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			token := unsignedToken(t, testCase.claims)

			got, err := extractAccountId(token)

			require.NoError(t, err)
			assert.Equal(t, testCase.want, got)
		})
	}
}

// TestExtractAccountIdReturnsEmptyWhenMissing verifies tokens without account claims are accepted.
func TestExtractAccountIdReturnsEmptyWhenMissing(t *testing.T) {
	got, err := extractAccountId(unsignedToken(t, map[string]any{
		"sub": "user",
	}))

	require.NoError(t, err)
	assert.Empty(t, got)
}

// unsignedToken returns an unsigned JWT string containing the provided claims.
func unsignedToken(t *testing.T, claims map[string]any) string {
	t.Helper()

	header, err := json.Marshal(map[string]any{
		"alg": "none",
		"typ": "JWT",
	})
	require.NoError(t, err)

	payload, err := json.Marshal(claims)
	require.NoError(t, err)

	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}
