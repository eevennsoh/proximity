package rovo

import (
	"context"
	"fmt"

	"bitbucket.org/atlassian-developers/proximity/cmd/commands/rovo/internal/auth"
	proximitytemplate "bitbucket.org/atlassian-developers/proximity/internal/template"
)

const (
	rovoAccessTokenFunctionName = "rovoAccessToken"
	rovoAuthNotConfiguredError  = "rovo auth is not configured; run proximity rovo login"
)

// rovoTemplateOptions returns renderer options for Rovo Dev config helpers.
func rovoTemplateOptions(authService auth.Interface) []proximitytemplate.Option {
	options := make([]proximitytemplate.Option, 0, 2)
	options = append(options, proximitytemplate.WithExprFunction(rovoAccessTokenFunctionName, rovoAccessTokenRenderFunction(authService)))
	options = append(options, proximitytemplate.WithTemplateFunction(rovoAccessTokenFunctionName, rovoAccessTokenRenderFunction(authService)))

	return options
}

// rovoAccessTokenRenderFunction returns a helper that renders a fresh Atlassian access token.
func rovoAccessTokenRenderFunction(authService auth.Interface) proximitytemplate.RenderFunction {
	return func(ctx context.Context, params ...any) (any, error) {
		if len(params) != 0 {
			return nil, fmt.Errorf("%s expects no arguments", rovoAccessTokenFunctionName)
		}

		if authService == nil {
			return nil, fmt.Errorf(rovoAuthNotConfiguredError)
		}

		return authService.AccessToken(ctx)
	}
}
