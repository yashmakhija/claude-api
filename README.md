# Claude API Proxy

Personal Claude API endpoint at `claude.iyash.me`.

Uses OAuth token (setup-token) from Claude Max subscription for **direct API access** to all models including Opus.

## Available Models

- `claude-opus-4-20250514` - Most capable (~1.5s latency)
- `claude-sonnet-4-20250514` - Balanced
- `claude-3-haiku-20240307` - Fast

## Endpoints

### OpenAI-compatible (recommended)

```bash
# List models
curl https://claude.iyash.me/v1/models

# Chat completion
curl https://claude.iyash.me/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "claude-opus-4-20250514",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### Native endpoints

- `POST /chat` - Simple chat
- `POST /stream` - Streaming chat
- `POST /messages` - Pass-through to Anthropic API
- `GET /health` - Health check

## Auth

Requires `Authorization: Bearer <key>` or `X-API-Key: <key>` header.

## How it works

Uses OAuth token with Claude Code identity headers to access Anthropic API directly. Key headers:

- `anthropic-beta: claude-code-20250219,oauth-2025-04-20`
- `user-agent: claude-cli/2.1.2`
- System prompt includes "You are Claude Code" (required for OAuth tokens)

## Service

```bash
systemctl status claude-api
systemctl restart claude-api
journalctl -u claude-api -f
```
