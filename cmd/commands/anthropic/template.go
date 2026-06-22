package anthropic

import (
	"context"
	"fmt"

	"bitbucket.org/atlassian-developers/proximity/cmd/commands/anthropic/internal/auth"
	proximitytemplate "bitbucket.org/atlassian-developers/proximity/internal/template"
)

const (
	anthropicAccessTokenFunctionName = "anthropicAccessToken"
	anthropicAuthNotConfiguredError  = "anthropic auth is not configured; run proximity anthropic login"
)

// anthropicTemplateOptions returns renderer options for Anthropic config helpers.
func anthropicTemplateOptions(authService auth.Interface) []proximitytemplate.Option {
	options := make([]proximitytemplate.Option, 0, 2)
	options = append(options, proximitytemplate.WithExprFunction(anthropicAccessTokenFunctionName, anthropicAccessTokenRenderFunction(authService)))
	options = append(options, proximitytemplate.WithTemplateFunction(anthropicAccessTokenFunctionName, anthropicAccessTokenRenderFunction(authService)))

	return options
}

// anthropicAccessTokenRenderFunction returns a helper that renders a fresh Claude access token.
func anthropicAccessTokenRenderFunction(authService auth.Interface) proximitytemplate.RenderFunction {
	return func(ctx context.Context, params ...any) (any, error) {
		if len(params) != 0 {
			return nil, fmt.Errorf("%s expects no arguments", anthropicAccessTokenFunctionName)
		}

		if authService == nil {
			return nil, fmt.Errorf(anthropicAuthNotConfiguredError)
		}

		return authService.AccessToken(ctx)
	}
}
