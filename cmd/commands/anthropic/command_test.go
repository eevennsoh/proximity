package anthropic

import (
	"context"
	"io"
	"log"
	"net/http"
	"testing"

	"bitbucket.org/atlassian-developers/proximity/cmd/commands/anthropic/internal/auth"
	"bitbucket.org/atlassian-developers/proximity/internal/config"
	proximitytemplate "bitbucket.org/atlassian-developers/proximity/internal/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.uber.org/mock/gomock"
)

// TestCommandIncludesServeAndAuthActions verifies the Anthropic command exposes runtime actions.
func TestCommandIncludesServeAndAuthActions(t *testing.T) {
	cmd := Command()

	assert.Equal(t, "anthropic", cmd.Name)
	require.Len(t, cmd.Subcommands, 5)

	names := make([]string, 0, len(cmd.Subcommands))

	for _, subcommand := range cmd.Subcommands {
		names = append(names, subcommand.Name)
	}

	assert.Contains(t, names, "serve")
	assert.Contains(t, names, "login")
	assert.Contains(t, names, "status")
	assert.Contains(t, names, "logout")
	assert.Contains(t, names, "doc")
}

// TestPortFlagDefaultsToAnthropicPort verifies the command has its own default port.
func TestPortFlagDefaultsToAnthropicPort(t *testing.T) {
	cmd := Command()

	flag, ok := cmd.Flags[0].(*cli.IntFlag)
	require.True(t, ok)
	assert.Equal(t, "port", flag.Name)
	assert.Equal(t, defaultPort, flag.Value)
}

// TestServePortCanBeSetBeforeOrAfterServe verifies both CLI port flag positions.
func TestServePortCanBeSetBeforeOrAfterServe(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "before serve",
			args: []string{"proximity", "anthropic", "--port", "29573", "serve"},
		},
		{
			name: "after serve",
			args: []string{"proximity", "anthropic", "serve", "--port", "29573"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var got int

			cmd := Command()
			for _, subcommand := range cmd.Subcommands {
				if subcommand.Name == "serve" {
					subcommand.Action = func(c *cli.Context) error {
						got = commandPort(c)
						return nil
					}
				}
			}

			app := cli.NewApp()
			app.Writer = io.Discard
			app.ErrWriter = io.Discard
			app.Commands = []*cli.Command{cmd}

			err := app.Run(testCase.args)

			require.NoError(t, err)
			assert.Equal(t, 29573, got)
		})
	}
}

// TestUnexpectedArgumentIsRejected verifies a mistyped subcommand fails loudly
// instead of silently running the parent action and dropping later flags.
func TestUnexpectedArgumentIsRejected(t *testing.T) {
	cmd := Command()

	app := cli.NewApp()
	app.Writer = io.Discard
	app.ErrWriter = io.Discard
	app.Commands = []*cli.Command{cmd}

	err := app.Run([]string{"proximity", "anthropic", "server", "--port", "8092"})

	require.Error(t, err)
	assert.ErrorContains(t, err, `unexpected argument "server"`)
}

// TestConfigDefinesAnthropicMessagesEndpoint verifies the embedded config targets Anthropic APIs.
func TestConfigDefinesAnthropicMessagesEndpoint(t *testing.T) {
	cfg, err := config.LoadFromBytes(proxyConfig)
	require.NoError(t, err)

	messagesOverride := cfg.Overrides.Uris["/v1/messages"]["POST"]
	healthOverride := cfg.Overrides.Uris["/health"]["GET"]
	messagesUri := cfg.UriGroups[1].SupportedUris[0]

	assert.Contains(t, cfg.BaseEndpoint, "api.anthropic.com")
	assert.NotNil(t, messagesOverride)
	require.Len(t, messagesUri.Out, 1)
	assert.Equal(t, http.MethodPost, messagesUri.Out[0].Method)
	assert.Equal(t, "/v1/messages", messagesUri.Out[0].Text)
	assert.Empty(t, cfg.Overrides.Global.Response.Headers)
	assert.Contains(t, healthOverride.Response.Headers, config.Header{
		Operation: config.AddOperation,
		Name:      "Content-Type",
		Input: config.Input{
			Text: "application/json",
		},
	})
	assert.Len(t, cfg.UriGroups, 2)
}

// TestAnthropicTemplateOptionsRegisterHelpers verifies templates can render access tokens.
func TestAnthropicTemplateOptionsRegisterHelpers(t *testing.T) {
	controller := gomock.NewController(t)
	authService := auth.NewMockInterface(controller)
	authService.EXPECT().AccessToken(gomock.Any()).Return("access-token", nil).Times(2)

	renderer := proximitytemplate.NewRenderer(log.New(io.Discard, "", 0), anthropicTemplateOptions(authService)...)

	exprToken, err := renderer.Render(context.Background(), "", `anthropicAccessToken()`, make(map[string]any), nil)
	require.NoError(t, err)
	templateToken, err := renderer.Render(context.Background(), `{{ anthropicAccessToken }}`, "", make(map[string]any), nil)

	require.NoError(t, err)
	assert.Equal(t, "access-token", string(exprToken))
	assert.Equal(t, "access-token", string(templateToken))
}
