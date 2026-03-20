// Package mcp implements MCP 2025-11-25 Streamable HTTP transport for the cloud-mcp service.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/orchestra-mcp/cloud-mcp/internal/auth"
	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"github.com/orchestra-mcp/cloud-mcp/internal/permissions"
	"github.com/orchestra-mcp/cloud-mcp/internal/protocol"
	"github.com/orchestra-mcp/cloud-mcp/internal/tools"
	"gorm.io/gorm"
)

// Handler implements MCP Streamable HTTP transport (MCP 2025-11-25).
// POST /mcp  — request/response
// GET  /mcp  — SSE stream for server-initiated messages
type Handler struct {
	db       *gorm.DB
	cfg      *config.Config
	perms    *permissions.Checker
	sessions *SessionStore
	registry *tools.Registry
}

// NewHandler creates a fully wired MCP handler.
func NewHandler(db *gorm.DB, cfg *config.Config) *Handler {
	perms := permissions.NewChecker(db)
	registry := tools.NewRegistry(db, cfg, perms)
	return &Handler{
		db:       db,
		cfg:      cfg,
		perms:    perms,
		sessions: NewSessionStore(),
		registry: registry,
	}
}

// HandlePost handles POST /mcp — the main MCP request/response endpoint.
func (h *Handler) HandlePost(c fiber.Ctx) error {
	// Resolve caller identity (0 = anonymous).
	userID := h.resolveUser(c)

	// Parse request body.
	var req protocol.Request
	if err := json.Unmarshal(c.Body(), &req); err != nil {
		return h.writeError(c, nil, protocol.CodeParseError, "parse error")
	}

	if req.JSONRPC != "2.0" {
		return h.writeError(c, req.ID, protocol.CodeInvalidRequest, "jsonrpc must be 2.0")
	}

	// Route to method handler.
	result, rpcErr := h.dispatch(req, userID, c)
	if rpcErr != nil {
		return h.writeError(c, req.ID, rpcErr.Code, rpcErr.Message)
	}

	// Notifications (no ID) get 202 Accepted, no body.
	if req.ID == nil {
		c.Status(fiber.StatusAccepted)
		return nil
	}

	resp := protocol.Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
	c.Set("Content-Type", "application/json")
	return c.JSON(resp)
}

// HandleGet handles GET /mcp — SSE stream for server push.
func (h *Handler) HandleGet(c fiber.Ctx) error {
	sessionID := c.Get("Mcp-Session-Id")
	if sessionID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Mcp-Session-Id header required for SSE stream",
		})
	}

	s, ok := h.sessions.Get(sessionID)
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "session not found",
		})
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	w := bufio.NewWriter(c.Response().BodyWriter())

	// Send initial "connected" event.
	fmt.Fprintf(w, "event: connected\ndata: {\"sessionId\":%q}\n\n", sessionID)
	w.Flush()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-s.send:
			if !ok {
				return nil
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			w.Flush()

		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			w.Flush()

		case <-s.Done():
			return nil

		case <-c.Context().Done():
			h.sessions.Remove(sessionID)
			return nil
		}
	}
}

// dispatch routes an RPC method to its handler.
func (h *Handler) dispatch(req protocol.Request, userID uint, c fiber.Ctx) (interface{}, *protocol.RPCError) {
	switch req.Method {
	case "initialize":
		return h.handleInitialize(req, userID, c)
	case "notifications/initialized":
		return nil, nil
	case "ping":
		return map[string]interface{}{}, nil
	case "tools/list":
		return h.handleToolsList(userID)
	case "tools/call":
		return h.handleToolCall(req, userID)
	default:
		return nil, &protocol.RPCError{
			Code:    protocol.CodeMethodNotFound,
			Message: fmt.Sprintf("method not found: %s", req.Method),
		}
	}
}

// handleInitialize responds to the MCP initialize handshake.
func (h *Handler) handleInitialize(_ protocol.Request, userID uint, c fiber.Ctx) (interface{}, *protocol.RPCError) {
	s := h.sessions.Create(userID)
	c.Set("Mcp-Session-Id", s.ID)

	result := protocol.InitializeResult{
		ProtocolVersion: protocol.ProtocolVersion,
		Capabilities: protocol.ServerCapabilities{
			Tools:   &protocol.ToolsCapability{ListChanged: false},
			Logging: &struct{}{},
		},
		ServerInfo: protocol.ServerInfo{
			Name:        "orchestra-cloud-mcp",
			Version:     "1.0.0",
			Title:       "Orchestra Cloud",
			Description: "Personal Orchestra MCP — manage your profile, install Orchestra, browse the marketplace, and control agent permissions from the web.",
			Icons: []protocol.Icon{
				{
					Src:      "https://orchestra-mcp.dev/favicon.ico",
					MimeType: "image/x-icon",
				},
				{
					Src:      "https://orchestra-mcp.dev/icon-192.png",
					MimeType: "image/png",
					Sizes:    []string{"192x192"},
				},
			},
			WebsiteURL: "https://orchestra-mcp.dev",
		},
		SessionID: s.ID,
	}
	return result, nil
}

// handleToolsList returns the filtered tool list based on user permissions.
func (h *Handler) handleToolsList(userID uint) (interface{}, *protocol.RPCError) {
	defs := h.registry.List(userID)
	return map[string]interface{}{
		"tools": defs,
	}, nil
}

// handleToolCall dispatches a tool call and returns its result.
func (h *Handler) handleToolCall(req protocol.Request, userID uint) (interface{}, *protocol.RPCError) {
	if req.Params == nil {
		return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "params required"}
	}

	name, _ := req.Params["name"].(string)
	if name == "" {
		return nil, &protocol.RPCError{Code: protocol.CodeInvalidParams, Message: "params.name required"}
	}

	args, _ := req.Params["arguments"].(map[string]interface{})
	result, err := h.registry.Call(name, args, userID)
	if err != nil {
		return nil, &protocol.RPCError{Code: protocol.CodeInternalError, Message: err.Error()}
	}

	return result, nil
}

// resolveUser extracts the user ID from the Authorization header or ?token= query param.
// Returns 0 for anonymous callers.
func (h *Handler) resolveUser(c fiber.Ctx) uint {
	// Prefer Authorization header.
	token := c.Get("Authorization")
	if token == "" {
		// Fall back to ?token= query param (used when pasting URL into Claude Desktop connectors dialog).
		token = c.Query("token")
	}
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return 0
	}
	userID, err := auth.ValidateToken(token, h.cfg, h.db)
	if err != nil {
		return 0
	}
	return userID
}

// writeError writes a JSON-RPC error response.
func (h *Handler) writeError(c fiber.Ctx, id interface{}, code int, message string) error {
	resp := protocol.Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &protocol.RPCError{Code: code, Message: message},
	}
	status := fiber.StatusOK
	if code == protocol.CodeParseError || code == protocol.CodeInvalidRequest {
		status = fiber.StatusBadRequest
	}
	c.Set("Content-Type", "application/json")
	c.Status(status)
	return c.JSON(resp)
}
