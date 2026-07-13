package rovo

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"

	"bitbucket.org/atlassian-developers/proximity/cmd/commands/rovo/internal/auth"
	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/server"

	"github.com/pkg/browser"
	"github.com/urfave/cli/v2"
)

const (
	defaultPort   = 29575
	portFlagName  = "port"
	portFlagAlias = "p"

	defaultCloudId = "a436116f-02ce-4520-8fbb-7301462a1674"
)

//go:embed config.yaml
var proxyConfig []byte

// Command returns the rovo subcommand.
func Command() *cli.Command {
	return &cli.Command{
		Name:  "rovo",
		Usage: "Run Proximity with Rovo Dev configuration",
		Description: `Run Proximity to provide pre-configured endpoints for Claude models routed
through the Rovo Dev proxy (api.atlassian.com/rovodev/v2). Authentication uses
the Atlassian OAuth device-code flow, so no API token is required. Authenticate
once with "proximity rovo login".`,
		Flags: []cli.Flag{
			portFlag(),
			envFlag(),
			cloudIdFlag(),
		},
		Action: run,
		Subcommands: []*cli.Command{
			serveCommand(),
			loginCommand(),
			statusCommand(),
			logoutCommand(),
		},
	}
}

// portFlag returns the proxy server port flag.
func portFlag() cli.Flag {
	return &cli.IntFlag{
		Name:    portFlagName,
		Aliases: []string{portFlagAlias},
		Value:   defaultPort,
		Usage:   "Port to run the server on",
	}
}

// envFlag returns the Rovo Dev environment selection flag.
func envFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "env",
		Aliases: []string{"e"},
		Value:   "prod",
		Usage:   "Rovo Dev environment (prod, staging)",
	}
}

// cloudIdFlag returns the Atlassian billing cloud ID flag.
func cloudIdFlag() cli.Flag {
	return &cli.StringFlag{
		Name:    "cloud-id",
		Aliases: []string{"c"},
		Value:   defaultCloudId,
		Usage:   "Atlassian cloud ID of the billing site",
		EnvVars: []string{"ROVO_CLOUD_ID", "CLOUD_ID"},
	}
}

// serveCommand returns an explicit serve subcommand for users who prefer verbs.
func serveCommand() *cli.Command {
	return &cli.Command{
		Name:   "serve",
		Usage:  "Start the Rovo Dev proxy",
		Flags:  []cli.Flag{portFlag(), envFlag(), cloudIdFlag()},
		Action: run,
	}
}

// loginCommand returns the credential login subcommand.
func loginCommand() *cli.Command {
	return &cli.Command{
		Name:   "login",
		Usage:  "Authenticate with Atlassian OAuth",
		Flags:  []cli.Flag{envFlag()},
		Action: login,
	}
}

// statusCommand returns the credential status subcommand.
func statusCommand() *cli.Command {
	return &cli.Command{
		Name:   "status",
		Usage:  "Show Atlassian OAuth credential status",
		Flags:  []cli.Flag{envFlag()},
		Action: status,
	}
}

// logoutCommand returns the credential removal subcommand.
func logoutCommand() *cli.Command {
	return &cli.Command{
		Name:   "logout",
		Usage:  "Remove stored Atlassian OAuth credentials",
		Flags:  []cli.Flag{envFlag()},
		Action: logout,
	}
}

// newAuth creates the Rovo Dev auth service configured by CLI flags.
func newAuth(c *cli.Context) (auth.Interface, error) {
	env := commandEnvironment(c)
	if err := validateEnvironment(env); err != nil {
		return nil, err
	}

	options := make([]auth.Option, 0, 2)
	options = append(options, auth.WithBrowserOpener(browser.OpenURL))
	options = append(options, auth.WithEnvironment(env))

	return auth.New(options...)
}

// run starts the Rovo Dev proxy using Atlassian OAuth credentials.
func run(c *cli.Context) error {
	if err := rejectUnexpectedArgs(c); err != nil {
		return err
	}

	env := commandEnvironment(c)
	if err := validateEnvironment(env); err != nil {
		return err
	}

	authService, err := newAuth(c)
	if err != nil {
		return err
	}

	cfg, err := config.LoadFromBytes(proxyConfig)
	if err != nil {
		return fmt.Errorf("failed to parse embedded config: %w", err)
	}

	vars := map[string]any{
		"rovoEnv": env,
		"cloudID": c.String("cloud-id"),
	}

	return server.RunServerWithOptions(server.Options{
		Config:          cfg,
		Port:            commandPort(c),
		Vars:            vars,
		TemplateOptions: rovoTemplateOptions(authService),
	})
}

// login stores Atlassian OAuth credentials using the device-code flow.
func login(c *cli.Context) error {
	authService, err := newAuth(c)
	if err != nil {
		return err
	}

	return authService.LoginWithDevice(commandContext(c), commandOutput(c))
}

// status writes credential status without exposing token values.
func status(c *cli.Context) error {
	authService, err := newAuth(c)
	if err != nil {
		return err
	}

	credentialStatus, err := authService.Status(commandContext(c))
	if err != nil {
		return err
	}

	output := commandOutput(c)
	if !credentialStatus.Authenticated {
		_, err = fmt.Fprintln(output, "not authenticated")
		return err
	}

	if _, err := fmt.Fprintln(output, "authenticated"); err != nil {
		return err
	}

	if !credentialStatus.ExpiresAt.IsZero() {
		if _, err := fmt.Fprintf(output, "expires: %s\n", credentialStatus.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")); err != nil {
			return err
		}
	}

	if credentialStatus.Expired {
		_, err = fmt.Fprintln(output, "expired: true")
		return err
	}

	_, err = fmt.Fprintln(output, "expired: false")
	return err
}

// logout removes stored Atlassian OAuth credentials.
func logout(c *cli.Context) error {
	authService, err := newAuth(c)
	if err != nil {
		return err
	}

	if err := authService.Logout(); err != nil {
		return err
	}

	_, err = fmt.Fprintln(commandOutput(c), "credentials removed")
	return err
}

// rejectUnexpectedArgs fails when positional arguments are passed, since a stray
// argument such as a mistyped subcommand causes the standard flag parser to stop
// reading flags, silently dropping anything after it (for example --port).
func rejectUnexpectedArgs(c *cli.Context) error {
	if c.NArg() == 0 {
		return nil
	}

	return fmt.Errorf("unexpected argument %q\n\nRun 'proximity rovo --help' for available subcommands (serve, login, status, logout)", c.Args().First())
}

// commandEnvironment returns the selected Rovo Dev environment from the CLI lineage.
func commandEnvironment(c *cli.Context) string {
	for _, commandContext := range c.Lineage() {
		if commandContext.IsSet("env") {
			return commandContext.String("env")
		}
	}

	return "prod"
}

// validateEnvironment ensures the selected environment is supported.
func validateEnvironment(env string) error {
	if !strings.Contains(env, "prod") && env != "staging" {
		return fmt.Errorf("--env must be prod or staging (got %q)", env)
	}

	return nil
}

// commandPort returns the explicitly selected proxy port or the default.
func commandPort(c *cli.Context) int {
	for _, commandContext := range c.Lineage() {
		if contextHasLocalPortFlag(commandContext) {
			return commandContext.Int(portFlagName)
		}
	}

	return defaultPort
}

// contextHasLocalPortFlag reports whether this CLI context set the port flag.
func contextHasLocalPortFlag(c *cli.Context) bool {
	for _, name := range c.LocalFlagNames() {
		if name == portFlagName || name == portFlagAlias {
			return true
		}
	}

	return false
}

// commandContext returns the CLI action context.
func commandContext(c *cli.Context) context.Context {
	if c.Context != nil {
		return c.Context
	}

	return context.Background()
}

// commandOutput returns the CLI output writer.
func commandOutput(c *cli.Context) io.Writer {
	if c.App != nil && c.App.Writer != nil {
		return c.App.Writer
	}

	return os.Stdout
}
