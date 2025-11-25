package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"

	templatepkg "bitbucket.org/atlassian-developers/mini-proxy/internal/template"
)

type RenderRequest struct {
	JSONData     string `json:"jsonData"`
	TemplateText string `json:"templateText"`
}

type RenderResponse struct {
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

func main() {
	http.HandleFunc("/", serveHTML)
	http.HandleFunc("/render", handleRender)

	port := "8080"
	log.Printf("Server starting on http://localhost:%s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func serveHTML(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Go Template Renderer</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            height: 100vh;
            display: flex;
            flex-direction: column;
            background: #f5f5f5;
        }
        .header {
            background: #2c3e50;
            color: white;
            padding: 1rem 2rem;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .header h1 {
            font-size: 1.5rem;
            font-weight: 600;
        }
        .container {
            flex: 1;
            display: flex;
            gap: 1rem;
            padding: 1rem;
            overflow: hidden;
        }
        .pane {
            flex: 1;
            display: flex;
            flex-direction: column;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        .pane-header {
            background: #34495e;
            color: white;
            padding: 0.75rem 1rem;
            font-weight: 600;
            font-size: 0.9rem;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .pane-content {
            flex: 1;
            padding: 1rem;
            overflow: auto;
        }
        textarea {
            width: 100%;
            height: 100%;
            border: none;
            resize: none;
            font-family: 'Courier New', monospace;
            font-size: 14px;
            line-height: 1.5;
            outline: none;
            background: #fafafa;
            padding: 0.5rem;
            white-space: nowrap;
            overflow-x: auto;
            overflow-y: auto;
        }
        .output {
            font-family: 'Courier New', monospace;
            font-size: 14px;
            line-height: 1.5;
            white-space: pre;
            overflow-x: auto;
            overflow-y: auto;
            background: #fafafa;
            padding: 0.5rem;
            min-height: 100%;
        }
        .error {
            color: #e74c3c;
            background: #fce4e4;
            border: 1px solid #fcc2c3;
            padding: 1rem;
            border-radius: 4px;
            font-family: 'Courier New', monospace;
            font-size: 13px;
        }
        .controls {
            padding: 1rem 2rem;
            background: white;
            border-top: 1px solid #e0e0e0;
            display: flex;
            justify-content: center;
            gap: 1rem;
        }
        button {
            background: #3498db;
            color: white;
            border: none;
            padding: 0.75rem 2rem;
            border-radius: 4px;
            font-size: 1rem;
            font-weight: 600;
            cursor: pointer;
            transition: background 0.2s;
        }
        button:hover {
            background: #2980b9;
        }
        button:active {
            transform: translateY(1px);
        }
        button:disabled {
            background: #95a5a6;
            cursor: not-allowed;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>🎨 Go Template Renderer</h1>
    </div>
    <div class="container">
        <div class="pane">
            <div class="pane-header">Input JSON</div>
            <div class="pane-content">
                <textarea id="jsonInput" placeholder='{"name": "World", "count": 42}'>{
  "name": "World",
  "items": ["Apple", "Banana", "Cherry"],
  "count": 42,
  "enabled": true
}</textarea>
            </div>
        </div>
        <div class="pane">
            <div class="pane-header">Go Template</div>
            <div class="pane-content">
                <textarea id="templateInput" placeholder="Hello, {{.name}}!">Hello, {{.name}}!

{{range .items}}
- {{.}}
{{end}}

Count: {{.count}}
Enabled: {{.enabled}}</textarea>
            </div>
        </div>
        <div class="pane">
            <div class="pane-header">Rendered Output</div>
            <div class="pane-content">
                <div id="output" class="output">Click "Render" to see the output...</div>
            </div>
        </div>
    </div>
    <div class="controls">
        <button id="renderBtn" onclick="render()">🚀 Render</button>
    </div>

    <script>
        async function render() {
            const jsonInput = document.getElementById('jsonInput').value;
            const templateInput = document.getElementById('templateInput').value;
            const output = document.getElementById('output');
            const renderBtn = document.getElementById('renderBtn');

            // Validate inputs
            if (!jsonInput.trim()) {
                output.innerHTML = '<div class="error">Error: JSON input is empty</div>';
                return;
            }
            if (!templateInput.trim()) {
                output.innerHTML = '<div class="error">Error: Template is empty</div>';
                return;
            }

            // Disable button during request
            renderBtn.disabled = true;
            renderBtn.textContent = '⏳ Rendering...';
            output.textContent = 'Processing...';

            try {
                const response = await fetch('/render', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({
                        jsonData: jsonInput,
                        templateText: templateInput
                    })
                });

                const data = await response.json();

                if (data.error) {
                    output.innerHTML = '<div class="error">Error: ' + escapeHtml(data.error) + '</div>';
                } else {
                    output.textContent = data.result;
                }
            } catch (error) {
                output.innerHTML = '<div class="error">Error: ' + escapeHtml(error.message) + '</div>';
            } finally {
                renderBtn.disabled = false;
                renderBtn.textContent = '🚀 Render';
            }
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Allow Ctrl+Enter or Cmd+Enter to render
        document.addEventListener('keydown', function(e) {
            if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
                render();
            }
        });
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

func handleRender(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RenderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "Invalid request format: "+err.Error())
		return
	}

	// Parse JSON data
	var data any
	if err := json.Unmarshal([]byte(req.JSONData), &data); err != nil {
		sendError(w, "Invalid JSON: "+err.Error())
		return
	}

	pkg := templatepkg.NewTemplate(log.Default())

	// Parse and execute template
	tmpl, err := template.New("template").Funcs(pkg.FunctionsWithStorage(make(map[string]string))).Parse(req.TemplateText)
	if err != nil {
		sendError(w, "Template parse error: "+err.Error())
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		sendError(w, "Template execution error: "+err.Error())
		return
	}

	response := RenderResponse{
		Result: buf.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func sendError(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(RenderResponse{
		Error: errMsg,
	})
}
