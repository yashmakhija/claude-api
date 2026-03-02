# Claude API

A lightweight Go wrapper for the Anthropic Claude API with streaming support.

## Features

- Simple REST API for Claude
- Streaming (SSE) support
- API key authentication
- Conversation history support
- Configurable models

## Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | No | Health check |
| `/chat` | POST | Yes | Regular chat response |
| `/stream` | POST | Yes | Streaming SSE response |

## Usage

```bash
# Build
go build -o claude-api main.go

# Run
export ANTHROPIC_API_KEY="your-key"
export CLIENT_API_KEY="your-client-key"  # optional, defaults to env
./claude-api
```

## API Example

```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-client-key" \
  -d '{"message": "Hello!"}'
```

## Environment Variables

- `ANTHROPIC_API_KEY` - Anthropic API key
- `CLIENT_API_KEY` - API key for clients to access this server
- `PORT` - Server port (default: 8080)
- `DEFAULT_MODEL` - Default Claude model (default: claude-sonnet-4-20250514)
