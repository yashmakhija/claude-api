# Claude API Proxy

Personal Claude API endpoint at `claude.iyash.me`.

## Current Setup (v2)

Uses [claude-max-api-proxy](https://www.npmjs.com/package/claude-max-api-proxy) to expose Claude Max subscription as an OpenAI-compatible API.

### Available Models

- `claude-opus-4` - Most capable
- `claude-sonnet-4` - Balanced
- `claude-haiku-4` - Fast

### Usage

```bash
curl https://claude.iyash.me/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-opus-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Service

```bash
# Status
systemctl status claude-max-proxy

# Logs
journalctl -u claude-max-proxy -f

# Restart
systemctl restart claude-max-proxy
```

### Requirements

- Claude CLI authenticated (`claude auth status`)
- Node.js 22+
- nginx with SSL (Let's Encrypt)

---

## Legacy (v1 - Archived)

The `main.go` file is the old Go implementation that used OAuth tokens directly. It was limited to Haiku only. Kept for reference.
