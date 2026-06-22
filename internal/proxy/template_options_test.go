package proxy

import (
	"context"
	"log"
	"testing"

	proximitytemplate "bitbucket.org/atlassian-developers/proximity/internal/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewPassesTemplateOptionsToRenderer verifies proxy construction uses the generic template extension point.
func TestNewPassesTemplateOptionsToRenderer(t *testing.T) {
	proxyServer := New(Options{
		Logger: log.Default(),
		TemplateOptions: []proximitytemplate.Option{
			proximitytemplate.WithExprFunction("customToken", func(_ context.Context, params ...any) (any, error) {
				require.Empty(t, params)
				return "configured-token", nil
			}),
		},
	})

	serverImplementation, ok := proxyServer.(*server)
	require.True(t, ok)

	got, err := serverImplementation.renderer.Render(
		context.Background(),
		"",
		`customToken()`,
		make(map[string]any),
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, "configured-token", string(got))
}
