package template

import (
	"context"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCustomExprFunctionUsesConfiguredFunction verifies Expr helpers can be supplied by callers.
func TestCustomExprFunctionUsesConfiguredFunction(t *testing.T) {
	renderer := NewRenderer(log.Default(), WithExprFunction("customHelper", func(_ context.Context, params ...any) (any, error) {
		require.Len(t, params, 1)
		assert.Equal(t, "hello", params[0])

		return "custom hello", nil
	}))

	got, err := renderer.Render(
		context.Background(),
		"",
		`customHelper(value)`,
		map[string]any{
			"value": "hello",
		},
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, "custom hello", string(got))
}

// TestCustomTemplateFunctionUsesConfiguredFunction verifies text/template helpers can be supplied by callers.
func TestCustomTemplateFunctionUsesConfiguredFunction(t *testing.T) {
	renderer := NewRenderer(log.Default(), WithTemplateFunction("customHelper", func(_ context.Context, params ...any) (any, error) {
		require.Len(t, params, 1)
		assert.Equal(t, "hello", params[0])

		return "custom hello", nil
	}))

	got, err := renderer.Render(
		context.Background(),
		`{{ customHelper .value }}`,
		"",
		map[string]any{
			"value": "hello",
		},
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, "custom hello", string(got))
}
