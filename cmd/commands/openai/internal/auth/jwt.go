package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	chatgptAccountIdClaim           = "chatgpt_account_id"
	namespacedChatgptAccountIdClaim = "https://api.openai.com/auth.chatgpt_account_id"
	organizationsClaim              = "organizations"
	idClaim                         = "id"
)

// extractAccountId returns the ChatGPT account ID from unsigned JWT claims.
func extractAccountId(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid jwt token")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("failed to decode jwt payload: %w", err)
	}

	var claims map[string]any

	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("failed to parse jwt payload: %w", err)
	}

	if accountId, ok := claims[chatgptAccountIdClaim].(string); ok {
		return accountId, nil
	}

	if accountId, ok := claims[namespacedChatgptAccountIdClaim].(string); ok {
		return accountId, nil
	}

	organizations, ok := claims[organizationsClaim].([]any)
	if !ok || len(organizations) == 0 {
		return "", nil
	}

	organization, ok := organizations[0].(map[string]any)
	if !ok {
		return "", nil
	}

	accountId, ok := organization[idClaim].(string)
	if !ok {
		return "", nil
	}

	return accountId, nil
}
