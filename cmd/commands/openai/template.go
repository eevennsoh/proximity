package openai

import (
	"context"
	"fmt"

	"bitbucket.org/atlassian-developers/proximity/cmd/commands/openai/internal/auth"
	proximitytemplate "bitbucket.org/atlassian-developers/proximity/internal/template"
)

const (
	openaiAccessTokenFunctionName = "openaiAccessToken"
	openaiAccountIdFunctionName   = "openaiAccountId"
	openaiAuthNotConfiguredError  = "openai auth is not configured; run proximity openai login or proximity openai login --device"
)

// openaiTemplateOptions returns renderer options for OpenAI config helpers.
func openaiTemplateOptions(authService auth.Interface) []proximitytemplate.Option {
	options := make([]proximitytemplate.Option, 0, 4)
	options = append(options, proximitytemplate.WithExprFunction(openaiAccessTokenFunctionName, openaiAccessTokenRenderFunction(authService)))
	options = append(options, proximitytemplate.WithExprFunction(openaiAccountIdFunctionName, openaiAccountIdRenderFunction(authService)))
	options = append(options, proximitytemplate.WithTemplateFunction(openaiAccessTokenFunctionName, openaiAccessTokenRenderFunction(authService)))
	options = append(options, proximitytemplate.WithTemplateFunction(openaiAccountIdFunctionName, openaiAccountIdRenderFunction(authService)))

	return options
}

// openaiAccessTokenRenderFunction returns a helper that renders a fresh ChatGPT access token.
func openaiAccessTokenRenderFunction(authService auth.Interface) proximitytemplate.RenderFunction {
	return func(ctx context.Context, params ...any) (any, error) {
		if len(params) != 0 {
			return nil, fmt.Errorf("%s expects no arguments", openaiAccessTokenFunctionName)
		}

		if authService == nil {
			return nil, fmt.Errorf(openaiAuthNotConfiguredError)
		}

		return authService.AccessToken(ctx)
	}
}

// openaiAccountIdRenderFunction returns a helper that renders the ChatGPT account ID.
func openaiAccountIdRenderFunction(authService auth.Interface) proximitytemplate.RenderFunction {
	return func(ctx context.Context, params ...any) (any, error) {
		if len(params) != 0 {
			return nil, fmt.Errorf("%s expects no arguments", openaiAccountIdFunctionName)
		}

		if authService == nil {
			return nil, fmt.Errorf(openaiAuthNotConfiguredError)
		}

		return authService.AccountId(ctx)
	}
}
