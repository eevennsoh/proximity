# Mini-proxy

This is a small configurable proxy with the aim of being able to compile it to proxy anything. This was a small fun project with the aim of proxying AI-Gateway to use OSS AI tools with the models it provides.

Ai-Gateway REST documentation: https://developer.atlassian.com/platform/ai-gateway/rest/

It requires [postman-slauthtoken](https://bitbucket.org/atlassian-developers/postman-slauthtoken/src/master/README.md) to get slauth tokens to properly authenticate to AI-Gateway.

You can run it locally using `make` or run the image via the docker compose file provided.

```shell
docker compose up -d
```

## Routes available

- OpenAI Chat Completions (drop-in OpenAI compatibility)
  - Endpoint: http://localhost:3001/openai/v1/chat/completions
  - The models available can be found here: https://developer.atlassian.com/platform/ai-gateway/models/openai/
  - Models list (served from config): http://localhost:3001/openai/v1/models

- Anthropic Claude via Bedrock (Anthropic Messages format)
  - Endpoint: http://localhost:3001/bedrock/claude/v1/messages
  - Streaming: set `"stream": true` in the request body
  - The proxy injects `anthropic_version: "bedrock-2023-05-31"` and converts simple text content to Claude text blocks as required by Bedrock

- Anthropic → OpenAI translation (use Claude with OpenAI-style clients)
  - Endpoint: http://localhost:3001/provider/bedrock/format/openai/v1/chat/completions
  - Description: Accepts OpenAI Chat Completion requests and translates them to Bedrock/Anthropic under the hood. Responses are returned in OpenAI-compatible format.
  - Streaming: supported; returns `chat.completion.chunk` frames that mirror OpenAI’s streaming shape

## Notes

- The proxy now exposes endpoints for:
  - OpenAI Chat Completions (pass-through)
  - Anthropic Claude (Anthropic Messages format via Bedrock)
  - Anthropic → OpenAI translation (so OpenAI SDKs can use Claude with no code changes)
- Contributions are welcome to expand the number of providers/models and add more translation/streaming modes.
