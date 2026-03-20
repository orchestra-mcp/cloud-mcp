# Orchestra Cloud MCP

Hosted personal MCP server for the [Orchestra](https://orchestra-mcp.com) platform, running at `orchestra-mcp.com/mcp`.

Implements the **MCP 2025-11-25 Streamable HTTP transport** — connect Claude Desktop or any MCP client in one click.

Part of the [Orchestra MCP](https://github.com/orchestra-mcp) framework.

## What is this?

Orchestra Cloud MCP lets you control Orchestra from any MCP-compatible AI agent without installing anything locally:

- **Check** if Orchestra is installed on your machine
- **Install** Orchestra, the desktop app, and IDE integrations via shell scripts
- **Browse** and install packs, plugins, skills, agents, and workflows from the marketplace
- **Read / update** your Orchestra profile (controlled by your permission toggles)

All tools are user-controlled — toggle permissions on/off from `orchestra-mcp.com/settings/mcp`.

## Quick Start

### Add to Claude Desktop (no account needed)

```json
{
  "mcpServers": {
    "orchestra": {
      "type": "sse",
      "url": "https://orchestra-mcp.com/mcp"
    }
  }
}
```

### Add with your account (full access)

1. Log in at [orchestra-mcp.com](https://orchestra-mcp.com)
2. Go to **Settings → MCP Access**
3. Click **Add to Claude Desktop** — pre-fills your token automatically

Or manually:

```json
{
  "mcpServers": {
    "orchestra": {
      "type": "sse",
      "url": "https://orchestra-mcp.com/mcp",
      "headers": {
        "Authorization": "Bearer <your-token>"
      }
    }
  }
}
```

## Available Tools

### Public (no account required)

| Tool | Description |
|------|-------------|
| `check_status` | Check if Orchestra is installed locally |
| `install_orchestra` | Install Orchestra CLI + configure your IDE |
| `install_desktop_app` | Install the Orchestra desktop app (macOS/Windows/Linux) |

### Authenticated (requires account token)

| Tool | Permission toggle | Description |
|------|-------------------|-------------|
| `get_profile` | `mcp.profile.read` | Read your Orchestra profile |
| `update_profile` | `mcp.profile.write` | Update name, timezone, bio |
| `list_packs` | `mcp.marketplace` | Browse available packs |
| `search_packs` | `mcp.marketplace` | Search the marketplace |
| `get_pack` | `mcp.marketplace` | Get details for a pack |
| `install_pack` | `mcp.marketplace` | Install a pack via CLI |

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8091` | HTTP listen port |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/orchestra_web?sslmode=disable` | PostgreSQL connection |
| `JWT_SECRET` | _(change in production)_ | JWT signing secret (same as apps/web) |
| `ALLOWED_ORIGINS` | `orchestra-mcp.com,localhost:3000,...` | Comma-separated CORS origins |
| `WEB_API_BASE_URL` | `https://orchestra-mcp.com/api` | Orchestra Web API base URL |
| `PUBLIC_RATE_LIMIT` | `10` | Requests/minute/IP for public tools |

## Project Structure

```
apps/cloud-mcp/
├── cmd/
│   └── main.go                  # Entry point, port 8091
├── internal/
│   ├── auth/
│   │   └── auth.go              # JWT + API key validation
│   ├── config/
│   │   └── config.go            # ENV-based configuration
│   ├── mcp/
│   │   ├── handler.go           # MCP Streamable HTTP transport
│   │   └── session.go           # SSE session management
│   ├── permissions/
│   │   └── checker.go           # Per-user permission toggle cache
│   ├── protocol/
│   │   └── mcp.go               # MCP 2025-11-25 types
│   └── tools/
│       ├── registry.go          # Tool registry + permission filtering
│       ├── status.go            # check_status
│       ├── install.go           # install_orchestra, install_desktop_app
│       ├── profile.go           # get_profile, update_profile
│       └── marketplace.go       # list_packs, search_packs, get_pack, install_pack
├── .github/
│   └── workflows/
│       ├── ci.yml               # Build + test on push
│       └── deploy.yml           # SSH deploy to VPS
├── Dockerfile                   # Multi-stage distroless image
├── go.mod                       # module: github.com/orchestra-mcp/cloud-mcp
└── README.md
```

## Development

```bash
# Build
go build -o orchestra-cloud-mcp ./cmd/

# Run locally (requires PostgreSQL)
DATABASE_URL="postgres://postgres:postgres@localhost:5432/orchestra_web?sslmode=disable" \
JWT_SECRET="dev-secret" \
./orchestra-cloud-mcp

# Vet
go vet ./...

# Test
go test ./...
```

### Health check

```bash
curl http://localhost:8091/health
# {"status":"ok","service":"orchestra-cloud-mcp","version":"1.0.0","protocol":"2025-11-25"}
```

### Try a tool call

```bash
# Initialize (anonymous)
curl -X POST http://localhost:8091/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"test","version":"1.0"}}}'

# List tools
curl -X POST http://localhost:8091/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'

# Call check_status
curl -X POST http://localhost:8091/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"check_status","arguments":{}}}'
```

## Docker

```bash
docker build -t orchestra-cloud-mcp .
docker run -p 8091:8091 \
  -e DATABASE_URL="postgres://..." \
  -e JWT_SECRET="your-secret" \
  orchestra-cloud-mcp
```

## Deployment

The server is deployed via GitHub Actions on every push to `master`. The workflow SSHs into the VPS, builds the Go binary, and restarts the `orchestra-cloud-mcp` systemd service.

See [`scripts/deploy/setup-server.sh`](https://github.com/orchestra-mcp/framework/blob/master/scripts/deploy/setup-server.sh) in the main framework repo for one-command server setup.

**GitHub secrets required** (Settings → Environments → production):

| Secret | Description |
|--------|-------------|
| `VPS_HOST` | Server IP or hostname |
| `VPS_USER` | SSH user (e.g. `deploy`) |
| `VPS_SSH_KEY` | Private key for SSH |
| `VPS_SSH_PORT` | SSH port (usually `22`) |
| `DISCORD_WEBHOOK` | _(optional)_ Discord deploy notifications |

## Protocol

Implements [MCP 2025-11-25](https://spec.modelcontextprotocol.io/) Streamable HTTP transport:

- `POST /mcp` — JSON-RPC request/response
- `GET /mcp` — SSE stream for server-initiated messages
- `GET /health` — Health check

## License

MIT License. See [LICENSE](LICENSE) for details.
