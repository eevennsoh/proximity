package anthropic

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"

	"bitbucket.org/atlassian-developers/proximity/cmd/commands/anthropic/internal/auth"
	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/server"

	"github.com/pkg/browser"
	"github.com/urfave/cli/v2"
)

const (
	defaultPort   = 29573
	portFlagName  = "port"
	portFlagAlias = "p"
)

//go:embed config.yaml
var proxyConfig []byte

// Command returns the anthropic subcommand.
func Command() *cli.Command {
	return &cli.Command{
		Name:        "anthropic",
		Usage:       "Run Proximity with Claude subscription authentication",
		Description: "Run an Anthropic-compatible local proxy backed by Claude OAuth credentials.",
		Flags: []cli.Flag{
			portFlag(),
		},
		Action: run,
		Subcommands: []*cli.Command{
			serveCommand(),
			loginCommand(),
			statusCommand(),
			logoutCommand(),
			docCommand(),
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

// serveCommand returns an explicit serve subcommand for users who prefer verbs.
func serveCommand() *cli.Command {
	return &cli.Command{
		Name:   "serve",
		Usage:  "Start the Anthropic-compatible proxy",
		Flags:  []cli.Flag{portFlag()},
		Action: run,
	}
}

// loginCommand returns the credential login subcommand.
func loginCommand() *cli.Command {
	return &cli.Command{
		Name:   "login",
		Usage:  "Authenticate with Claude OAuth",
		Action: login,
	}
}

// statusCommand returns the credential status subcommand.
func statusCommand() *cli.Command {
	return &cli.Command{
		Name:   "status",
		Usage:  "Show Claude OAuth credential status",
		Action: status,
	}
}

// logoutCommand returns the credential removal subcommand.
func logoutCommand() *cli.Command {
	return &cli.Command{
		Name:   "logout",
		Usage:  "Remove stored Claude OAuth credentials",
		Action: logout,
	}
}

// newAuth creates the Anthropic auth service.
func newAuth() (auth.Interface, error) {
	options := make([]auth.Option, 0, 1)
	options = append(options, auth.WithBrowserOpener(browser.OpenURL))

	return auth.New(options...)
}

// run starts the Anthropic-compatible proxy using Claude OAuth credentials.
func run(c *cli.Context) error {
	if err := rejectUnexpectedArgs(c); err != nil {
		return err
	}

	authService, err := newAuth()
	if err != nil {
		return err
	}

	cfg, err := config.LoadFromBytes(proxyConfig)
	if err != nil {
		return fmt.Errorf("failed to parse embedded config: %w", err)
	}

	return server.RunServerWithOptions(server.Options{
		Config:          cfg,
		Port:            commandPort(c),
		Vars:            make(map[string]any),
		TemplateOptions: anthropicTemplateOptions(authService),
	})
}

// rejectUnexpectedArgs fails when positional arguments are passed, since a stray
// argument such as a mistyped subcommand causes the standard flag parser to stop
// reading flags, silently dropping anything after it (for example --port).
func rejectUnexpectedArgs(c *cli.Context) error {
	if c.NArg() == 0 {
		return nil
	}

	return fmt.Errorf("unexpected argument %q\n\nRun 'proximity anthropic --help' for available subcommands (serve, login, status, logout)", c.Args().First())
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

// login stores Claude OAuth credentials using browser login.
func login(c *cli.Context) error {
	authService, err := newAuth()
	if err != nil {
		return err
	}

	return authService.LoginWithBrowser(commandContext(c), commandOutput(c))
}

// status writes credential status without exposing token values.
func status(c *cli.Context) error {
	authService, err := newAuth()
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

// logout removes stored Claude OAuth credentials.
func logout(c *cli.Context) error {
	authService, err := newAuth()
	if err != nil {
		return err
	}

	if err := authService.Logout(); err != nil {
		return err
	}

	_, err = fmt.Fprintln(commandOutput(c), "credentials removed")
	return err
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
