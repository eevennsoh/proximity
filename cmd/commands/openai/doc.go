package openai

import (
	"fmt"
	"io"
	"text/tabwriter"

	"bitbucket.org/atlassian-developers/proximity/internal/config"

	"github.com/urfave/cli/v2"
)

const (
	pathResponses     = "/v1/responses"
	contentTypeHeader = "Content-Type: application/json"
)

type endpointDocumentation struct {
	method      string
	path        string
	description string
	upstream    string
}

// docCommand returns the local endpoint reference subcommand.
func docCommand() *cli.Command {
	return &cli.Command{
		Name:   "doc",
		Usage:  "Show available OpenAI proxy endpoints",
		Action: doc,
	}
}

// doc writes endpoint documentation generated from the embedded proxy config.
func doc(c *cli.Context) error {
	cfg, err := config.LoadFromBytes(proxyConfig)
	if err != nil {
		return fmt.Errorf("failed to parse embedded config: %w", err)
	}

	return writeDocumentation(commandOutput(c), cfg, c.Int("port"))
}

// writeDocumentation renders a terminal-friendly endpoint reference.
func writeDocumentation(output io.Writer, cfg *config.Config, port int) error {
	baseUrl := fmt.Sprintf("http://localhost:%d", port)

	if _, err := fmt.Fprintf(output, "Proximity OpenAI Proxy\n\nBase URL: %s\n\n", baseUrl); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(output, "Authenticate once with:"); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(output, "  proximity openai login"); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(output, "  proximity openai login --device"); err != nil {
		return err
	}

	if _, err := fmt.Fprint(output, "\nClients do not need an OpenAI API key. Proximity injects ChatGPT OAuth credentials.\n\n"); err != nil {
		return err
	}

	if err := writeEndpointTable(output, endpointDocumentations(cfg)); err != nil {
		return err
	}

	return writeEndpointExamples(output, baseUrl)
}

// endpointDocumentations returns visible endpoint docs from proxy config routes.
func endpointDocumentations(cfg *config.Config) []endpointDocumentation {
	endpoints := make([]endpointDocumentation, 0)

	for _, uriGroup := range cfg.UriGroups {
		if uriGroup.Hidden {
			continue
		}

		for _, supportedUri := range uriGroup.SupportedUris {
			for _, outMethod := range supportedUri.Out {
				endpoints = append(endpoints, endpointDocumentation{
					method:      outMethod.Method,
					path:        supportedUri.In,
					description: supportedUri.Description,
					upstream:    endpointUpstreamPath(supportedUri, outMethod),
				})
			}
		}
	}

	return endpoints
}

// endpointUpstreamPath returns the configured upstream path for an endpoint.
func endpointUpstreamPath(supportedUri config.UriMap, outMethod config.OutMethod) string {
	if outMethod.Text != "" {
		return outMethod.Text
	}

	return supportedUri.In
}

// writeEndpointTable writes endpoint rows as aligned terminal columns.
func writeEndpointTable(output io.Writer, endpoints []endpointDocumentation) error {
	if _, err := fmt.Fprint(output, "Endpoints:\n\n"); err != nil {
		return err
	}

	table := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)

	if _, err := fmt.Fprintln(table, "METHOD\tPATH\tDESCRIPTION\tUPSTREAM"); err != nil {
		return err
	}

	for _, endpoint := range endpoints {
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", endpoint.method, endpoint.path, endpoint.description, endpoint.upstream); err != nil {
			return err
		}
	}

	return table.Flush()
}

// writeEndpointExamples writes curl examples for known OpenAI-compatible endpoints.
func writeEndpointExamples(output io.Writer, baseUrl string) error {
	if _, err := fmt.Fprint(output, "\nExamples:\n\n"); err != nil {
		return err
	}

	if err := writeGetExample(output, baseUrl, "/health"); err != nil {
		return err
	}

	if err := writeGetExample(output, baseUrl, "/v1/models"); err != nil {
		return err
	}

	return writePostExample(output, baseUrl, pathResponses, `{"model":"gpt-5.5","input":"Say hello"}`)
}

// writeGetExample writes a single curl example for a GET endpoint.
func writeGetExample(output io.Writer, baseUrl, path string) error {
	if _, err := fmt.Fprintf(output, "GET %s\n", path); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(output, "  curl %s%s\n\n", baseUrl, path); err != nil {
		return err
	}

	return nil
}

// writePostExample writes a single curl example for a JSON POST endpoint.
func writePostExample(output io.Writer, baseUrl, path, body string) error {
	if _, err := fmt.Fprintf(output, "POST %s\n", path); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(output, "  curl %s%s \\\n", baseUrl, path); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(output, "    -H '%s' \\\n", contentTypeHeader); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(output, "    -d '%s'\n\n", body); err != nil {
		return err
	}

	return nil
}
