package openai

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"

	"bitbucket.org/atlassian-developers/proximity/cmd/commands/openai/internal/auth"
	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/server"

	"github.com/pkg/browser"
	"github.com/urfave/cli/v2"
)

const defaultPort = 29574

//go:embed config.yaml
var proxyConfig []byte

// Command returns the openai subcommand.
func Command() *cli.Command {
	return &cli.Command{
		Name:        "openai",
		Usage:       "Run Proximity with ChatGPT subscription authentication",
		Description: "Run an OpenAI-compatible local proxy backed by ChatGPT OAuth credentials.",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   defaultPort,
				Usage:   "Port to run the server on",
			},
			&cli.StringFlag{
				Name:    "credentials-file",
				Usage:   "Path to the OpenAI credential file",
				EnvVars: []string{"PROXIMITY_OPENAI_CREDENTIALS_FILE"},
			},
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

// serveCommand returns an explicit serve subcommand for users who prefer verbs.
func serveCommand() *cli.Command {
	return &cli.Command{
		Name:   "serve",
		Usage:  "Start the OpenAI-compatible proxy",
		Action: run,
	}
}

// loginCommand returns the credential login subcommand.
func loginCommand() *cli.Command {
	return &cli.Command{
		Name:  "login",
		Usage: "Authenticate with ChatGPT OAuth",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "device",
				Usage: "Use device-code login instead of browser login",
			},
		},
		Action: login,
	}
}

// statusCommand returns the credential status subcommand.
func statusCommand() *cli.Command {
	return &cli.Command{
		Name:   "status",
		Usage:  "Show ChatGPT OAuth credential status",
		Action: status,
	}
}

// logoutCommand returns the credential removal subcommand.
func logoutCommand() *cli.Command {
	return &cli.Command{
		Name:   "logout",
		Usage:  "Remove stored ChatGPT OAuth credentials",
		Action: logout,
	}
}

// newAuth creates the OpenAI auth service configured by CLI flags.
func newAuth(c *cli.Context) (auth.Interface, error) {
	options := make([]auth.Option, 0, 2)
	options = append(options, auth.WithBrowserOpener(browser.OpenURL))

	if credentialPath := c.String("credentials-file"); credentialPath != "" {
		options = append(options, auth.WithCredentialPath(credentialPath))
	}

	return auth.New(options...)
}

// run starts the OpenAI-compatible proxy using ChatGPT OAuth credentials.
func run(c *cli.Context) error {
	authService, err := newAuth(c)
	if err != nil {
		return err
	}

	cfg, err := config.LoadFromBytes(proxyConfig)
	if err != nil {
		return fmt.Errorf("failed to parse embedded config: %w", err)
	}

	return server.RunServerWithOptions(server.Options{
		Config:          cfg,
		Port:            c.Int("port"),
		Vars:            make(map[string]any),
		TemplateOptions: openaiTemplateOptions(authService),
	})
}

// login stores ChatGPT OAuth credentials using browser or device login.
func login(c *cli.Context) error {
	authService, err := newAuth(c)
	if err != nil {
		return err
	}

	if c.Bool("device") {
		return authService.LoginWithDevice(commandContext(c), commandOutput(c))
	}

	return authService.LoginWithBrowser(commandContext(c), commandOutput(c))
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

	if credentialStatus.AccountId != "" {
		if _, err := fmt.Fprintf(output, "account: %s\n", credentialStatus.AccountId); err != nil {
			return err
		}
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

// logout removes stored ChatGPT OAuth credentials.
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
