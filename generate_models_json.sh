#!/bin/bash

# Run queries for each provider and capture the output
echo "Fetching OpenAI models..."
openai_models=$(./refresh_models.sh \
  "family=gpt-4.1-family&vendor=openai" \
  "family=gpt-4.1-mini-family&vendor=openai" \
  "family=gpt-4.1-nano-family&vendor=openai" \
  "family=gpt-4o-family&vendor=openai" \
  "family=gpt-4o-mini-family&vendor=openai" \
  "family=gpt-5-family&vendor=openai" \
  "family=gpt-5-nano-family&vendor=openai" \
  "family=gpt-5-mini-family&vendor=openai" \
  "family=gpt-5-codex-family&vendor=openai" \
  "family=gpt-5.1-family&vendor=openai" \
  "family=gpt-5.1-chat-family&vendor=openai" \
  "family=gpt-5.1-codex-family&vendor=openai"
)

echo "Fetching Anthropic (Bedrock) models..."
anthropic_bedrock_models=$(./refresh_models.sh "family=claude-family&vendor=bedrock")

echo "Fetching Anthropic (Vertex) models..."
anthropic_vertex_models=$(./refresh_models.sh "family=claude-family&vendor=google")

echo "Fetching Gemini models..."
gemini_models=$(./refresh_models.sh "family=gemini-pro-family&vendor=google" "family=gemini-flash-family&vendor=google")

# Use jq to construct the final JSON object
# The --argjson flag is used to pass a JSON-encoded string (the array)
jq -n \
  --argjson openai "$(echo "$openai_models" | jq -R . | jq -s .)" \
  --argjson bedrock "$(echo "$anthropic_bedrock_models" | jq -R . | jq -s .)" \
  --argjson vertex "$(echo "$anthropic_vertex_models" | jq -R . | jq -s .)" \
  --argjson gemini "$(echo "$gemini_models" | jq -R . | jq -s .)" \
  '{openai: $openai, bedrock: $bedrock, vertex: $vertex, gemini: $gemini}' > models.json

echo "JSON file 'models.json' created successfully."
