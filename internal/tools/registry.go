package tools

import (
	"fmt"

	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"github.com/orchestra-mcp/cloud-mcp/internal/protocol"
	"github.com/orchestra-mcp/cloud-mcp/internal/permissions"
	"gorm.io/gorm"
)

// Tool is a callable MCP tool.
type Tool struct {
	Definition protocol.ToolDefinition
	Permission string // "" = public (no auth needed)
	Handler    func(args map[string]interface{}, userID uint) (protocol.ToolResult, error)
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

	return r
}

func (r *Registry) register(t Tool) {
	r.tools = append(r.tools, t)
}

// List returns tool definitions visible to the given user (filtered by permissions).
func (r *Registry) List(userID uint) []protocol.ToolDefinition {
	defs := make([]protocol.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		if t.Permission == "" || r.perms.Can(userID, t.Permission) {
			defs = append(defs, t.Definition)
		}
	}
	return defs
}

// Call dispatches a tool call by name.
func (r *Registry) Call(name string, args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
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
						Text: fmt.Sprintf("This tool requires authentication. Add your Orchestra token at orchestra-mcp.com/settings/mcp and reconnect with authorization."),
					}},
					IsError: true,
				}, nil
			}
			return protocol.ToolResult{
				Content: []protocol.Content{{
					Type: "text",
					Text: fmt.Sprintf("Permission denied. Enable \"%s\" at https://orchestra-mcp.com/settings/mcp", t.Permission),
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
