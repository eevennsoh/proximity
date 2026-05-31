package proxy

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"bitbucket.org/atlassian-developers/proximity/internal/config"
	"bitbucket.org/atlassian-developers/proximity/internal/template"
)

func TestClaudeRoutesStripContextManagement(t *testing.T) {
	configPaths := []string{
		filepath.Join("..", "..", "config.yaml"),
		filepath.Join("..", "..", "cmd", "commands", "ai-gateway", "config.yaml"),
	}
	routes := []string{
		"/bedrock/claude/v1/messages",
		"/vertex/claude/v1/messages",
	}

	for _, configPath := range configPaths {
		cfg, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("load %s: %v", configPath, err)
		}

		for _, route := range routes {
			t.Run(configPath+" "+route, func(t *testing.T) {
				bodyExpr := cfg.Overrides.Uris[route]["POST"].Request.Body.Expr
				rendered, err := template.NewRenderer(nil).RenderExpr(bodyExpr, map[string]any{
					"body": map[string]any{
						"model":              "claude-opus-4-5-20251101",
						"stream":             false,
						"max_tokens":         32,
						"context_management": map[string]any{"enabled": true},
						"messages": []any{
							map[string]any{
								"role":    "user",
								"content": "hello",
							},
						},
					},
				}, nil)
				if err != nil {
					t.Fatalf("render body expr: %v", err)
				}

				var upstreamBody map[string]any
				if err := json.Unmarshal(rendered, &upstreamBody); err != nil {
					t.Fatalf("unmarshal rendered body: %v\nbody: %s", err, rendered)
				}

				if _, ok := upstreamBody["context_management"]; ok {
					t.Fatalf("context_management leaked into upstream body: %s", rendered)
				}
				if _, ok := upstreamBody["anthropic_version"]; !ok {
					t.Fatalf("expected anthropic_version in upstream body: %s", rendered)
				}
			})
		}
	}
}
