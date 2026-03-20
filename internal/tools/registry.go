package tools

import (
	"fmt"
	"sync"

	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"github.com/orchestra-mcp/cloud-mcp/internal/protocol"
	"github.com/orchestra-mcp/cloud-mcp/internal/permissions"
	"gorm.io/gorm"
)

// tokenStore holds the raw Bearer token per authenticated userID for the duration
// of a tool call. Admin tools need to forward the token to the web API.
var (
	tokenStore = map[uint]string{}
	tokenMu    sync.RWMutex
)

// Tool is a callable MCP tool.
type Tool struct {
	Definition   protocol.ToolDefinition
	Permission   string // "" = public (no auth needed)
	VisibleToAll bool   // if true, always show in tools/list even for unauthenticated users
	Handler      func(args map[string]interface{}, userID uint) (protocol.ToolResult, error)
}

// Registry holds all registered tools and dispatches calls.
type Registry struct {
	tools []Tool
	perms *permissions.Checker
}

// NewRegistry creates a fully populated tool registry.
func NewRegistry(db *gorm.DB, cfg *config.Config, perms *permissions.Checker) *Registry {
	r := &Registry{perms: perms}

	// Register all tools.
	r.register(newCheckStatusTool())
	r.register(newInstallOrchestralTool())
	r.register(newInstallDesktopTool())
	r.register(newGetProfileTool(db, cfg))
	r.register(newUpdateProfileTool(db, cfg))

	// Marketplace tools.
	r.register(newListPacksTool(cfg))
	r.register(newSearchPacksTool(cfg))
	r.register(newInstallPackTool(cfg))
	r.register(newGetPackTool(cfg))

	// Admin tools (only visible to users with role=admin).
	r.register(newAdminPlatformStatsTool(db, cfg))
	r.register(newAdminListUsersTool(db, cfg))
	r.register(newAdminUpdateUserRoleTool(db, cfg))
	r.register(newAdminGetSettingTool(db, cfg))
	r.register(newAdminUpdateSettingTool(db, cfg))
	r.register(newAdminSeedSettingsTool(db, cfg))
	r.register(newAdminListPagesTool(db, cfg))
	r.register(newAdminCreatePageTool(db, cfg))
	r.register(newAdminUpdatePageTool(db, cfg))
	r.register(newAdminSendNotificationTool(db, cfg))

	return r
}

func (r *Registry) register(t Tool) {
	r.tools = append(r.tools, t)
}

// List returns tool definitions visible to the given user (filtered by permissions).
// Tools with VisibleToAll=true are always included regardless of auth status.
// Admin tools (mcp.admin permission) are only shown to admin users.
func (r *Registry) List(userID uint) []protocol.ToolDefinition {
	defs := make([]protocol.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		switch {
		case t.Permission == "":
			// Public tool — always visible.
			defs = append(defs, t.Definition)
		case t.VisibleToAll:
			// Visible to all authenticated and unauthenticated users.
			// Permission is enforced at call time, not list time.
			defs = append(defs, t.Definition)
		case r.perms.Can(userID, t.Permission):
			// User has this permission.
			defs = append(defs, t.Definition)
		}
	}
	return defs
}

// Call dispatches a tool call by name, forwarding the raw token for admin tools.
func (r *Registry) Call(name string, args map[string]interface{}, userID uint, rawToken string) (protocol.ToolResult, error) {
	// Store token so admin handlers can forward it to the web API.
	if rawToken != "" && userID != 0 {
		tokenMu.Lock()
		tokenStore[userID] = rawToken
		tokenMu.Unlock()
		defer func() {
			tokenMu.Lock()
			delete(tokenStore, userID)
			tokenMu.Unlock()
		}()
	}

	for _, t := range r.tools {
		if t.Definition.Name != name {
			continue
		}

		// Permission check.
		if t.Permission != "" && !r.perms.Can(userID, t.Permission) {
			if userID == 0 {
				return protocol.ToolResult{
					Content: []protocol.Content{{
						Type: "text",
						Text: "This tool requires authentication. Add your Orchestra token at orchestra-mcp.dev/settings/mcp and reconnect with authorization.",
					}},
					IsError: true,
				}, nil
			}
			return protocol.ToolResult{
				Content: []protocol.Content{{
					Type: "text",
					Text: fmt.Sprintf("Permission denied. Enable \"%s\" at https://orchestra-mcp.dev/settings/mcp", t.Permission),
				}},
				IsError: true,
			}, nil
		}

		if args == nil {
			args = map[string]interface{}{}
		}
		return t.Handler(args, userID)
	}
	return protocol.ToolResult{
		Content: []protocol.Content{{Type: "text", Text: fmt.Sprintf("tool not found: %s", name)}},
		IsError: true,
	}, nil
}
