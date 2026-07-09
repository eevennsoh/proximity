package aigateway

import (
	_ "embed"
	"fmt"
	"strings"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/server"

	"github.com/urfave/cli/v2"
)

//go:embed config.yaml
var proxyConfig []byte

const (
	defaultPort          = 29574
	gatewayTargetMain    = "main"
	defaultGatewayTarget = gatewayTargetMain
	gatewayTargetEval    = "eval"
)

// Command returns the ai-gateway subcommand
func Command() *cli.Command {
	return &cli.Command{
		Name:  "ai-gateway",
		Usage: "Run Proximity with AI-Gateway configuration",
		Description: `Run Proximity to provide pre-configured endpoints for OpenAI, Claude, and Gemini models translated to be compatible with the enterprise APIs
through AI-Gateway.`,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   defaultPort,
				Usage:   "Port to run the server on",
			},
			&cli.StringFlag{
				Name:    "env",
				Aliases: []string{"e"},
				Value:   "staging",
				Usage:   "AI-Gateway environment (staging, prod)",
			},
			&cli.StringFlag{
				Name:  "gateway-target",
				Value: defaultGatewayTarget,
				Usage: "AI-Gateway target (main, eval)",
			},
			&cli.StringSliceFlag{
				Name:  "profile",
				Usage: "Profile definition: name=W;useCaseId=X;adGroup=Y;atlassianCloudId=Z (adGroup and atlassianCloudId are optional)",
			},
			&cli.StringFlag{
				Name:  "default-profile",
				Usage: "Name of the profile to use by default (if not defined then uses the first profile)",
			},
			&cli.BoolFlag{
				Name:  "use-slauth-command",
				Usage: "Get Slauth tokens by executing atlas slauth token instead of using atlas-cli plugin auth",
			},
		},
		Action: run,
	}
}

func parseProfile(s string) (map[string]string, error) {
	profile := make(map[string]string)
	parts := strings.Split(s, ";")

	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)

		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid profile part: %s", part)
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "name", "useCaseId", "adGroup", "atlassianCloudId":
			profile[key] = value
		default:
			return nil, fmt.Errorf("unknown profile key: %s", key)
		}
	}

	return profile, nil
}

func validateGatewayTarget(target string) error {
	switch target {
	case gatewayTargetMain, gatewayTargetEval:
		return nil
	default:
		return fmt.Errorf("invalid --gateway-target %q: expected %q or %q", target, gatewayTargetMain, gatewayTargetEval)
	}
}

func buildGlobalVars(env, gatewayTarget, defaultProfile string, useSlauthCommand bool, profiles []any) (map[string]any, error) {
	if err := validateGatewayTarget(gatewayTarget); err != nil {
		return nil, err
	}

	vars := make(map[string]any)
	vars["aiGatewayEnv"] = env
	vars["aiGatewayTarget"] = gatewayTarget
	vars["profiles"] = profiles

	if defaultProfile != "" {
		vars["defaultProfile"] = defaultProfile
	}

	vars["useSlauthCommand"] = useSlauthCommand
	return vars, nil
}

func run(c *cli.Context) error {
	port := c.Int("port")
	env := c.String("env")
	gatewayTarget := c.String("gateway-target")
	defaultProfile := c.String("default-profile")
	useSlauthCommand := c.Bool("use-slauth-command")

	profileStrings := c.StringSlice("profile")

	if len(profileStrings) == 0 {
		return fmt.Errorf("at least one --profile must be defined")
	}

	profiles := make([]any, 0, len(profileStrings))

	for i, s := range profileStrings {
		profile, err := parseProfile(s)
		if err != nil {
			return fmt.Errorf("failed to parse profile %d: %w", i, err)
		}

		profiles = append(profiles, profile)
	}

	vars, err := buildGlobalVars(env, gatewayTarget, defaultProfile, useSlauthCommand, profiles)
	if err != nil {
		return err
	}

	cfg, err := config.LoadFromBytes(proxyConfig)
	if err != nil {
		return fmt.Errorf("failed to parse embedded config: %w", err)
	}

	return server.RunServer(cfg, port, vars)
}
