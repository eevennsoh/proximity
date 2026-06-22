package main

import (
	"fmt"
	"log"
	"os"

	aigateway "bitbucket.org/atlassian-developers/proximity/cmd/commands/ai-gateway"
	anthropic "bitbucket.org/atlassian-developers/proximity/cmd/commands/anthropic"
	openai "bitbucket.org/atlassian-developers/proximity/cmd/commands/openai"
	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/server"

	"github.com/urfave/cli/v2"
)

var (
	Version string

	logFile *os.File
)

func main() {
	app := &cli.App{
		Name:    "proximity",
		Usage:   "A general purpose configurable HTTP proxy",
		Version: Version,
		Description: `Proximity is a configurable HTTP proxy that can transform requests and responses
based on a YAML configuration file.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to the config file",
			},
			&cli.IntFlag{
				Name:    "port",
				Aliases: []string{"p"},
				Value:   29574,
				Usage:   "Port to run the server on",
			},
			&cli.StringFlag{
				Name:    "log-file",
				Aliases: []string{"l"},
				Usage:   "Write logs to a file instead of stderr",
				EnvVars: []string{"PROXIMITY_LOG_FILE"},
			},
		},
		Before: func(c *cli.Context) error {
			path := c.String("log-file")
			if path == "" {
				return nil
			}

			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file %q: %w", path, err)
			}

			logFile = f
			log.SetOutput(f)
			return nil
		},
		After: func(c *cli.Context) error {
			if logFile != nil {
				return logFile.Close()
			}
			return nil
		},
		Action: runWithConfig,
		Commands: []*cli.Command{
			aigateway.Command(),
			anthropic.Command(),
			openai.Command(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runWithConfig(c *cli.Context) error {
	configPath := c.String("config")
	if configPath == "" {
		return fmt.Errorf("--config flag is required when not using a subcommand\n\nRun 'proximity --help' for usage")
	}

	port := c.Int("port")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// TODO: allow caller to provide a path to a file which provides vars
	return server.RunServer(cfg, port, make(map[string]any))
}
