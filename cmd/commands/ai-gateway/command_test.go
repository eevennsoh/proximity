package aigateway

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	proximitytemplate "bitbucket.org/atlassian-developers/proximity/internal/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestCommandIncludesGatewayTargetFlag(t *testing.T) {
	cmd := Command()

	var gatewayTargetFlag *cli.StringFlag

	for _, flag := range cmd.Flags {
		if typedFlag, ok := flag.(*cli.StringFlag); ok && typedFlag.Name == "gateway-target" {
			gatewayTargetFlag = typedFlag
			break
		}
	}

	require.NotNil(t, gatewayTargetFlag)
	assert.Equal(t, defaultGatewayTarget, gatewayTargetFlag.Value)
	assert.Contains(t, gatewayTargetFlag.Usage, "main")
	assert.Contains(t, gatewayTargetFlag.Usage, "eval")
}

func TestParseProfileAcceptsExistingKeys(t *testing.T) {
	got, err := parseProfile("name=default;useCaseId=use-case;adGroup=ad-group;atlassianCloudId=cloud-id")

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"name":             "default",
		"useCaseId":        "use-case",
		"adGroup":          "ad-group",
		"atlassianCloudId": "cloud-id",
	}, got)
}

func TestBuildGlobalVarsIncludesGatewayTarget(t *testing.T) {
	profiles := []any{map[string]string{"name": "default", "useCaseId": "use-case"}}

	got, err := buildGlobalVars("prod", gatewayTargetEval, "default", true, profiles)

	require.NoError(t, err)
	assert.Equal(t, "prod", got["aiGatewayEnv"])
	assert.Equal(t, gatewayTargetEval, got["aiGatewayTarget"])
	assert.Equal(t, "default", got["defaultProfile"])
	assert.Equal(t, true, got["useSlauthCommand"])
	assert.Equal(t, profiles, got["profiles"])
}

func TestInvalidGatewayTargetIsRejected(t *testing.T) {
	err := validateGatewayTarget("offline")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid --gateway-target "offline"`)
	assert.Contains(t, err.Error(), gatewayTargetMain)
	assert.Contains(t, err.Error(), gatewayTargetEval)
}

func TestCommandRejectsInvalidGatewayTargetBeforeStartingServer(t *testing.T) {
	app := cli.NewApp()
	app.Writer = io.Discard
	app.ErrWriter = io.Discard
	app.Commands = []*cli.Command{Command()}

	err := app.Run([]string{
		"proximity",
		"ai-gateway",
		"--gateway-target",
		"offline",
		"--profile",
		"name=default;useCaseId=use-case",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid --gateway-target "offline"`)
}

func TestConfigsRenderGatewayTargetEndpoints(t *testing.T) {
	configs := map[string]*config.Config{
		"embedded": loadEmbeddedConfig(t),
		"root":     loadRootConfig(t),
	}

	tests := []struct {
		name string
		vars map[string]any
		want string
	}{
		{
			name: "default target uses main gateway",
			vars: map[string]any{
				"aiGatewayEnv": "staging",
			},
			want: "https://ai-gateway.us-east-1.staging.atl-paas.net",
		},
		{
			name: "main target uses main gateway",
			vars: map[string]any{
				"aiGatewayEnv":    "prod",
				"aiGatewayTarget": gatewayTargetMain,
			},
			want: "https://ai-gateway.us-east-1.prod.atl-paas.net",
		},
		{
			name: "eval target uses eval gateway",
			vars: map[string]any{
				"aiGatewayEnv":    "prod",
				"aiGatewayTarget": gatewayTargetEval,
			},
			want: "https://ai-eval-gateway.sgw.prod.atl-paas.net",
		},
	}

	for configName, cfg := range configs {
		for _, testCase := range tests {
			t.Run(configName+"/"+testCase.name, func(t *testing.T) {
				got := renderBaseEndpoint(t, cfg, testCase.vars)

				assert.Equal(t, testCase.want, got)
			})
		}
	}
}

func TestConfigsDeriveSlauthAudienceFromGatewayTarget(t *testing.T) {
	configs := map[string]*config.Config{
		"embedded": loadEmbeddedConfig(t),
		"root":     loadRootConfig(t),
	}

	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			authorizationHeader := findGlobalRequestHeader(t, cfg, "Authorization")

			assert.Contains(t, authorizationHeader.Expr, `let aiGatewayTarget = get(globalVars, "aiGatewayTarget") ?? "main";`)
			assert.Contains(t, authorizationHeader.Expr, `let aiGatewayAudience = aiGatewayTarget == "eval" ? "ai-eval-gateway" : "ai-gateway";`)
			assert.Contains(t, authorizationHeader.Expr, "aiGatewayAudience")
		})
	}
}

func loadEmbeddedConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, err := config.LoadFromBytes(proxyConfig)
	require.NoError(t, err)

	return cfg
}

func loadRootConfig(t *testing.T) *config.Config {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "config.yaml"))
	require.NoError(t, err)

	cfg, err := config.LoadFromBytes(data)
	require.NoError(t, err)

	return cfg
}

func renderBaseEndpoint(t *testing.T, cfg *config.Config, vars map[string]any) string {
	t.Helper()

	renderer := proximitytemplate.NewRenderer(log.New(io.Discard, "", 0))
	got, err := renderer.RenderExpr(context.Background(), cfg.BaseEndpoint, map[string]any{
		"globalVars": vars,
	}, nil)
	require.NoError(t, err)

	return string(got)
}

func findGlobalRequestHeader(t *testing.T, cfg *config.Config, name string) config.Header {
	t.Helper()

	for _, header := range cfg.Overrides.Global.Request.Headers {
		if header.Name == name {
			return header
		}
	}

	require.Failf(t, "missing global request header", "header %q not found", name)
	return config.Header{}
}
