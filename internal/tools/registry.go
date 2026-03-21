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
	// Platform
	r.register(newAdminPlatformStatsTool(db, cfg))
	// Users
	r.register(newAdminListUsersTool(db, cfg))
	r.register(newAdminGetUserTool(db, cfg))
	r.register(newAdminUpdateUserTool(db, cfg))
	r.register(newAdminUpdateUserRoleTool(db, cfg))
	r.register(newAdminSuspendUserTool(db, cfg))
	r.register(newAdminUnsuspendUserTool(db, cfg))
	r.register(newAdminVerifyUserTool(db, cfg))
	r.register(newAdminImpersonateTool(db, cfg))
	r.register(newAdminForceResetPasswordTool(db, cfg))
	r.register(newAdminGetLastOTPTool(db, cfg))
	r.register(newAdminUpsertSubscriptionTool(db, cfg))
	r.register(newAdminNotifyUserTool(db, cfg))
	// Badges
	r.register(newAdminListBadgesTool(db, cfg))
	r.register(newAdminCreateBadgeTool(db, cfg))
	r.register(newAdminUpdateBadgeTool(db, cfg))
	r.register(newAdminDeleteBadgeTool(db, cfg))
	r.register(newAdminAwardBadgeTool(db, cfg))
	r.register(newAdminRevokeBadgeTool(db, cfg))
	// Wallet / Points
	r.register(newAdminGrantPointsTool(db, cfg))
	r.register(newAdminGetPointsTool(db, cfg))
	// Teams
	r.register(newAdminListTeamsTool(db, cfg))
	r.register(newAdminGetTeamTool(db, cfg))
	r.register(newAdminUpdateTeamTool(db, cfg))
	r.register(newAdminDeleteTeamTool(db, cfg))
	r.register(newAdminAddTeamMemberTool(db, cfg))
	r.register(newAdminRemoveTeamMemberTool(db, cfg))
	// Marketplace approval
	r.register(newAdminListPendingMarketplaceTool(db, cfg))
	r.register(newAdminApproveMarketplaceTool(db, cfg))
	r.register(newAdminRejectMarketplaceTool(db, cfg))
	// Blog posts
	r.register(newAdminListPostsTool(db, cfg))
	r.register(newAdminCreatePostTool(db, cfg))
	r.register(newAdminUpdatePostTool(db, cfg))
	r.register(newAdminDeletePostTool(db, cfg))
	// CMS pages
	r.register(newAdminListPagesTool(db, cfg))
	r.register(newAdminCreatePageTool(db, cfg))
	r.register(newAdminUpdatePageTool(db, cfg))
	r.register(newAdminDeletePageTool(db, cfg))
	// Categories
	r.register(newAdminListCategoriesTool(db, cfg))
	r.register(newAdminCreateCategoryTool(db, cfg))
	r.register(newAdminDeleteCategoryTool(db, cfg))
	// Sponsors
	r.register(newAdminListSponsorsTool(db, cfg))
	r.register(newAdminCreateSponsorTool(db, cfg))
	r.register(newAdminUpdateSponsorTool(db, cfg))
	r.register(newAdminDeleteSponsorTool(db, cfg))
	// Community
	r.register(newAdminListCommunityPostsTool(db, cfg))
	r.register(newAdminUpdateCommunityPostTool(db, cfg))
	r.register(newAdminDeleteCommunityPostTool(db, cfg))
	// Issues
	r.register(newAdminListIssuesTool(db, cfg))
	r.register(newAdminUpdateIssueTool(db, cfg))
	// Contact
	r.register(newAdminListContactTool(db, cfg))
	r.register(newAdminDeleteContactTool(db, cfg))
	// Notifications
	r.register(newAdminSendNotificationTool(db, cfg))
	r.register(newAdminListNotificationsTool(db, cfg))
	// Content moderation
	r.register(newAdminListContentTool(db, cfg))
	r.register(newAdminUpdateContentVisibilityTool(db, cfg))
	r.register(newAdminDeleteContentTool(db, cfg))
	// Settings (all keys)
	r.register(newAdminGetSettingTool(db, cfg))
	r.register(newAdminUpdateSettingTool(db, cfg))
	r.register(newAdminSeedSettingsTool(db, cfg))
	r.register(newAdminTestEmailTool(db, cfg))
	r.register(newAdminGenerateSitemapTool(db, cfg))
	// GitHub sync
	r.register(newAdminListGitHubReposTool(db, cfg))
	r.register(newAdminSyncGitHubIssuesTool(db, cfg))
	// Framework docs (/docs page)
	r.register(newAdminListDocsTool(db, cfg))
	r.register(newAdminGetDocTool(db, cfg))
	r.register(newAdminCreateDocTool(db, cfg))
	r.register(newAdminUpdateDocTool(db, cfg))
	r.register(newAdminPinDocTool(db, cfg))
	r.register(newAdminDeleteDocTool(db, cfg))

	// User content tools (skills, agents, workflows, notes, API collections, presentations, community posts, shares).
	r.register(newListSkillsTool(cfg))
	r.register(newCreateSkillTool(cfg))
	r.register(newGetSkillTool(cfg))
	r.register(newUpdateSkillTool(cfg))
	r.register(newDeleteSkillTool(cfg))
	r.register(newListAgentsTool(cfg))
	r.register(newCreateAgentTool(cfg))
	r.register(newGetAgentTool(cfg))
	r.register(newUpdateAgentTool(cfg))
	r.register(newDeleteAgentTool(cfg))
	r.register(newListWorkflowsTool(cfg))
	r.register(newCreateWorkflowTool(cfg))
	r.register(newGetWorkflowTool(cfg))
	r.register(newUpdateWorkflowTool(cfg))
	r.register(newDeleteWorkflowTool(cfg))
	r.register(newListNotesTool(cfg))
	r.register(newCreateNoteTool(cfg))
	r.register(newUpdateNoteTool(cfg))
	r.register(newDeleteNoteTool(cfg))
	r.register(newListAPICollectionsTool(cfg))
	r.register(newCreateAPICollectionTool(cfg))
	r.register(newGetAPICollectionTool(cfg))
	r.register(newAddAPIEndpointTool(cfg))
	r.register(newDeleteAPICollectionTool(cfg))
	r.register(newListPresentationsTool(cfg))
	r.register(newCreatePresentationTool(cfg))
	r.register(newAddSlideTool(cfg))
	r.register(newDeletePresentationTool(cfg))
	r.register(newListCommunityPostsTool(cfg))
	r.register(newCreateCommunityPostTool(cfg))
	r.register(newUpdateCommunityPostTool(cfg))
	r.register(newDeleteCommunityPostTool(cfg))
	r.register(newListSharesTool(cfg))
	r.register(newCreateShareTool(cfg))

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
