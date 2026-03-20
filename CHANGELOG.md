## v1.0.0

- Initial release: Orchestra Cloud MCP
- MCP 2025-11-25 Streamable HTTP transport (POST + GET /mcp)
- Two-tier auth: public tools (no token) + authenticated tools (JWT / API key)
- 9 tools: check_status, install_orchestra, install_desktop_app, get_profile, update_profile, list_packs, search_packs, get_pack, install_pack
- Per-user permission toggles with 30s in-process cache
- Rate limiting for public tools (10 req/min/IP)
- Multi-stage Dockerfile (distroless runtime)
- GitHub Actions CI + SSH deploy workflow
