# Proximity

A desktop application to proxy AI-Gateway built with [Wails](https://wails.io/). Proximity exposes standard LLM provider endpoints enabling you to use various enterprise AI models.

## Purpose

Proximity serves as a bridge between LLM clients and AI-Gateway which is the compliant API and provides the best models in an API which differs slightly from the upstream provider APIs.

There is no authentication required from clients, the proxy automatically handles SLAuth authentication with AI-Gateway for all endpoints. You don't need to provide any API keys or authentication tokens when making requests to the proxy.

## Installation

### Download Pre-built Application

Download the latest release for macOS (ARM64):

```bash
curl -O https://statlas.prod.atl-paas.net/vportella/proximity/proximity-arm64-latest.tar.gz
tar -xzf proximity-arm64-latest.tar.gz
```

Remove macOS quarantine attribute:

```bash
xattr -d com.apple.quarantine Proximity.app
```

### Build from Source

#### Prerequisites

- Go 1.21 or later
- Node.js 16+ and npm
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2

#### Build Steps

```bash
# Clone the repository
git clone git@bitbucket.org:atlassian-developers/proximity.git
cd proximity

# Install dependencies
npm install --prefix frontend

# Run in development mode
make run

# Build for production
make build

# Create distributable package
make package
```

## Usage

The proxy runs on **port 29576** or **29575** (for development) and provides multiple endpoints to mimic different LLM providers.

### OpenAI Chat Completions

Drop-in replacement for OpenAI's Chat Completions API.

**Endpoint:** `http://localhost:29576/openai/v1/chat/completions`

```bash
curl -X POST http://localhost:29576/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5-2025-08-07",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

**List available models:**

```bash
curl http://localhost:29576/openai/v1/models
```

### Anthropic Claude via Bedrock

Native Anthropic Messages API format proxied through AWS Bedrock.

**Endpoint:** `http://localhost:29576/bedrock/claude/v1/messages`

```bash
curl -X POST http://localhost:29576/bedrock/claude/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-sonnet-4-5-20250929-v1:0",
    "stream": false,
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, Claude!"}
    ]
  }'
```

**Features:**
- Streaming support: set `"stream": true` in request body
- Automatic injection of the `anthropic_version: "bedrock-2023-05-31"` header

**List available models:**

```bash
curl http://localhost:29576/bedrock/claude/v1/models
```

### Anthropic ↔ OpenAI Translation

Use Claude models with OpenAI-compatible clients without code changes.

**Endpoint:** `http://localhost:29576/provider/bedrock/format/openai/v1/chat/completions`

```bash
curl -X POST http://localhost:29576/provider/bedrock/format/openai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-sonnet-4-5-20250929-v1:0",
    "messages": [
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

**Features:**
- Accepts OpenAI Chat Completion format
- Translates to Bedrock/Anthropic under the hood
- Returns OpenAI-compatible responses
- Streaming supported: returns `chat.completion.chunk` frames

**List available models:**

```bash
curl http://localhost:29576/provider/bedrock/format/openai/v1/models
```

### Google Gemini

Content generation endpoints for Google's Gemini models.

**Endpoints:**
- Generate: `http://localhost:29576/google/gemini/v1beta/models/{model}:generateContent`
- Stream: `http://localhost:29576/google/gemini/v1beta/models/{model}:streamGenerateContent`

## Configuration

Proximity uses `config.yaml` for route definitions and request/response transformations. The configuration supports:

- **URI routing** with template-based path parameters
- **Header manipulation** (add, remove, modify)
- **Request/response body templating** using Go templates
- **Route-specific overrides** for authentication and formatting

### Configuration Structure

```yaml
baseEndpoint: https://ai-gateway.us-east-1.staging.atl-paas.net

supportedUris:
  - in: /openai/v1/chat/completions
    out: /v1/openai/v1/chat/completions
  # ... more routes

overrides:
  global:
    request:
      headers: [...]
  uris:
    /specific/endpoint:
      request:
        body:
          template: |
            # Go template for request transformation
```

### Model Configuration

Available models are stored in `models.json` and can be refreshed using:

```bash
make refresh-models
```

## Architecture

### Tech Stack

- **Backend:** Go with [Wails v2](https://wails.io/) framework
- **Frontend:** React with Vite and Tailwind CSS
- **Proxy Engine:** Custom HTTP proxy with template-based transformations
- **Build System:** Make + Wails build tools

### Project Structure

```
proximity/
├── main.go                # Application entry point
├── config.yaml            # Proxy configuration
├── models.json            # Available AI models
├── internal/
│   ├── app/              # Wails application logic
│   ├── config/           # Configuration parsing
│   └── proxy/            # Proxy handler and templating
├── frontend/             # React frontend application
│   ├── src/
│   │   ├── App.jsx       # Main UI component
│   │   └── assets/       # Images and fonts
│   └── wailsjs/          # Generated Wails bindings
└── build/                # Application icons and build config
```

## Development

### Running in Development Mode

```bash
make run
```

This starts the Wails application in development mode on port **29575** with hot reload enabled.

### Available Make Targets

- `make run` - Run in development mode
- `make build` - Build production binary
- `make package` - Create distributable .tar.gz
- `make refresh-models` - Update models.json from AI-Gateway
- `make upload` - Upload package to Statlas (requires Atlas CLI)

## API Documentation

For detailed information about the underlying AI-Gateway APIs, refer to:

- [AI-Gateway REST API Documentation](https://developer.atlassian.com/platform/ai-gateway/rest/)
- [AI-Gateway Supported Models](https://developer.atlassian.com/platform/ai-gateway/models/)

## Author

**vportella**
