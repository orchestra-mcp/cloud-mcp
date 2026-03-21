package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/orchestra-mcp/cloud-mcp/internal/auth"
	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"github.com/orchestra-mcp/cloud-mcp/internal/permissions"
	"github.com/orchestra-mcp/cloud-mcp/internal/protocol"
	"gorm.io/gorm"
)

// adminAPIFetch calls the web API with the admin's Bearer token.
func adminAPIFetch(baseURL, method, path, token string, body interface{}) (interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var arrResult []interface{}
	if json.Unmarshal(respBody, &arrResult) == nil {
		return arrResult, nil
	}
	var result interface{}
	json.Unmarshal(respBody, &result)
	return result, nil
}

func adminFetchText(baseURL, method, path, token string, body interface{}) (protocol.ToolResult, error) {
	result, err := adminAPIFetch(baseURL, method, path, token, body)
	if err != nil {
		return errResult(err.Error()), nil
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	return protocol.ToolResult{Content: []protocol.Content{{Type: "text", Text: string(b)}}}, nil
}

func strProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": desc}
}

func numProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "number", "description": desc}
}

func boolProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "boolean", "description": desc}
}

func arrProp(desc string) map[string]interface{} {
	return map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "number"}, "description": desc}
}

func getAdminToken(userID uint, db *gorm.DB) (string, error) {
	tokenMu.RLock()
	token, ok := tokenStore[userID]
	tokenMu.RUnlock()
	if !ok || token == "" {
		return "", fmt.Errorf("no token available for user %d", userID)
	}
	return token, nil
}

func errResult(msg string) protocol.ToolResult {
	return protocol.ToolResult{
		Content: []protocol.Content{{Type: "text", Text: msg}},
		IsError: true,
	}
}

// adminTool is a helper to build admin-only tools with consistent structure.
func adminTool(db *gorm.DB, cfg *config.Config, name, title, desc string, schema map[string]interface{}, fn func(args map[string]interface{}, token string) (protocol.ToolResult, error)) Tool {
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:        name,
			Title:       title,
			Description: "Admin: " + desc + " Requires admin role.",
			InputSchema: schema,
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("No admin token available. Reconnect with your Orchestra token."), nil
			}
			return fn(args, token)
		},
	}
}

func objSchema(props map[string]interface{}, required ...string) map[string]interface{} {
	s := map[string]interface{}{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// ─── PLATFORM STATS ───────────────────────────────────────────────────────────

func newAdminPlatformStatsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_platform_stats", "Platform Statistics",
		"Get platform-wide statistics: total users, active users, admin count.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			var total, active, admins int64
			db.Model(&auth.User{}).Count(&total)
			db.Model(&auth.User{}).Where("status = ?", "active").Count(&active)
			db.Model(&auth.User{}).Where("role = ?", "admin").Count(&admins)
			text := fmt.Sprintf("## Platform Statistics\n\n**Total users:** %d\n**Active users:** %d\n**Admin users:** %d\n", total, active, admins)
			return protocol.ToolResult{Content: []protocol.Content{{Type: "text", Text: text}}}, nil
		})
}

// ─── USER MANAGEMENT ──────────────────────────────────────────────────────────

func newAdminListUsersTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_users", "List Users",
		"List all platform users. Filter by role, status, or search by name/email.",
		objSchema(map[string]interface{}{
			"page":   numProp("Page number (default 1)"),
			"limit":  numProp("Results per page (default 20, max 100)"),
			"role":   strProp("Filter by role: admin, member, guest"),
			"status": strProp("Filter by status: active, suspended, blocked"),
			"q":      strProp("Search by name or email"),
		}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			q := url.Values{}
			for _, k := range []string{"role", "status", "q"} {
				if v, _ := args[k].(string); v != "" {
					q.Set(k, v)
				}
			}
			for _, k := range []string{"page", "limit"} {
				if v, _ := args[k].(float64); v > 0 {
					q.Set(k, fmt.Sprintf("%.0f", v))
				}
			}
			path := "/api/admin/users"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, path, token, nil)
		})
}

func newAdminGetUserTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_get_user", "Get User Details",
		"Get full details for a user including project/note/session/team counts.",
		objSchema(map[string]interface{}{"user_id": numProp("User ID")}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, fmt.Sprintf("/api/admin/users/%.0f", id), token, nil)
		})
}

func newAdminUpdateUserTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_user", "Update User",
		"Update a user's name, email, role, or status.",
		objSchema(map[string]interface{}{
			"user_id": numProp("User ID"),
			"name":    strProp("Display name"),
			"email":   strProp("Email address"),
			"role":    strProp("Role: admin, team_owner, team_manager, user"),
			"status":  strProp("Status: active, suspended, blocked"),
		}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			body := map[string]interface{}{}
			for _, f := range []string{"name", "email", "role", "status"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/admin/users/%.0f", id), token, body)
		})
}

func newAdminUpdateUserRoleTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_user_role", "Update User Role",
		"Change a user's role.",
		objSchema(map[string]interface{}{
			"user_id": numProp("User ID"),
			"role":    strProp("New role: admin, team_owner, team_manager, user"),
		}, "user_id", "role"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			role, _ := args["role"].(string)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/admin/users/%.0f/role", id), token, map[string]interface{}{"role": role})
		})
}

func newAdminSuspendUserTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_suspend_user", "Suspend User",
		"Suspend a user account.",
		objSchema(map[string]interface{}{"user_id": numProp("User ID")}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/admin/users/%.0f/suspend", id), token, map[string]interface{}{})
		})
}

func newAdminUnsuspendUserTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_unsuspend_user", "Unsuspend User",
		"Reactivate a suspended user account.",
		objSchema(map[string]interface{}{"user_id": numProp("User ID")}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/admin/users/%.0f/unsuspend", id), token, map[string]interface{}{})
		})
}

func newAdminVerifyUserTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_verify_user", "Verify User",
		"Mark a user as verified.",
		objSchema(map[string]interface{}{"user_id": numProp("User ID")}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/admin/users/%.0f/verify", id), token, map[string]interface{}{})
		})
}

func newAdminImpersonateTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_impersonate_user", "Impersonate User",
		"Generate a JWT to log in as another user for debugging.",
		objSchema(map[string]interface{}{"user_id": numProp("User ID to impersonate")}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/admin/users/%.0f/impersonate", id), token, map[string]interface{}{})
		})
}

func newAdminForceResetPasswordTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_force_reset_password", "Force Reset Password",
		"Force-set a new password for a user.",
		objSchema(map[string]interface{}{
			"user_id":  numProp("User ID"),
			"password": strProp("New password (min 8 chars)"),
		}, "user_id", "password"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			pw, _ := args["password"].(string)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/admin/users/%.0f/password", id), token, map[string]interface{}{"password": pw})
		})
}

func newAdminGetLastOTPTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_get_last_otp", "Get Last OTP",
		"Get the last OTP code issued to a user (support debugging).",
		objSchema(map[string]interface{}{"user_id": numProp("User ID")}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, fmt.Sprintf("/api/admin/users/%.0f/otp", id), token, nil)
		})
}

func newAdminUpsertSubscriptionTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_upsert_subscription", "Set User Subscription",
		"Create or update a user's subscription plan.",
		objSchema(map[string]interface{}{
			"user_id": numProp("User ID"),
			"plan":    strProp("Plan name (e.g. pro, enterprise, free)"),
			"notes":   strProp("Admin notes"),
		}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			body := map[string]interface{}{}
			if v, _ := args["plan"].(string); v != "" {
				body["plan"] = v
			}
			if v, _ := args["notes"].(string); v != "" {
				body["notes"] = v
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/admin/users/%.0f/subscription", id), token, body)
		})
}

func newAdminNotifyUserTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_notify_user", "Notify User",
		"Send an in-app notification to a specific user.",
		objSchema(map[string]interface{}{
			"user_id": numProp("User ID"),
			"title":   strProp("Notification title"),
			"message": strProp("Notification body"),
			"type":    strProp("Type: info, success, warning, error (default: info)"),
		}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			body := map[string]interface{}{}
			for _, f := range []string{"title", "message", "type"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/admin/users/%.0f/notify", id), token, body)
		})
}

// ─── BADGES ───────────────────────────────────────────────────────────────────

func newAdminListBadgesTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_badges", "List Badge Definitions",
		"List all badge definitions sorted by order and name.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/badges", token, nil)
		})
}

func newAdminCreateBadgeTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_create_badge", "Create Badge",
		"Create a new badge definition.",
		objSchema(map[string]interface{}{
			"slug":             strProp("Unique slug"),
			"name":             strProp("Display name"),
			"icon":             strProp("Icon name or URL"),
			"color":            strProp("Hex color"),
			"category":         strProp("Category"),
			"description":      strProp("How to earn it"),
			"points_threshold": numProp("Points needed to auto-award"),
			"sort_order":       numProp("Display order"),
		}, "slug", "name"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			body := map[string]interface{}{}
			for _, f := range []string{"slug", "name", "icon", "color", "category", "description"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			for _, f := range []string{"points_threshold", "sort_order"} {
				if v, _ := args[f].(float64); v != 0 {
					body[f] = int(v)
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/badges", token, body)
		})
}

func newAdminUpdateBadgeTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_badge", "Update Badge",
		"Update a badge definition.",
		objSchema(map[string]interface{}{
			"badge_id":         numProp("Badge definition ID"),
			"name":             strProp("Display name"),
			"icon":             strProp("Icon"),
			"color":            strProp("Hex color"),
			"category":         strProp("Category"),
			"description":      strProp("Description"),
			"points_threshold": numProp("Points threshold"),
			"sort_order":       numProp("Display order"),
		}, "badge_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["badge_id"].(float64)
			body := map[string]interface{}{}
			for _, f := range []string{"name", "icon", "color", "category", "description"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			for _, f := range []string{"points_threshold", "sort_order"} {
				if v, _ := args[f].(float64); v != 0 {
					body[f] = int(v)
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPut, fmt.Sprintf("/api/admin/badges/%.0f", id), token, body)
		})
}

func newAdminDeleteBadgeTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_badge", "Delete Badge",
		"Delete a badge definition.",
		objSchema(map[string]interface{}{"badge_id": numProp("Badge definition ID")}, "badge_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["badge_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/badges/%.0f", id), token, nil)
		})
}

func newAdminAwardBadgeTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_award_badge", "Award Badge to User",
		"Award a badge to a specific user.",
		objSchema(map[string]interface{}{
			"user_id":             numProp("User ID"),
			"badge_definition_id": strProp("Badge definition ID or slug"),
			"note":                strProp("Why the badge was awarded"),
		}, "user_id", "badge_definition_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			body := map[string]interface{}{"badge_definition_id": args["badge_definition_id"]}
			if v, _ := args["note"].(string); v != "" {
				body["note"] = v
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/admin/users/%.0f/badges", id), token, body)
		})
}

func newAdminRevokeBadgeTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_revoke_badge", "Revoke Badge from User",
		"Remove a badge from a user.",
		objSchema(map[string]interface{}{
			"user_id":  numProp("User ID"),
			"badge_id": numProp("Badge award ID"),
		}, "user_id", "badge_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			uid := args["user_id"].(float64)
			bid := args["badge_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/users/%.0f/badges/%.0f", uid, bid), token, nil)
		})
}

// ─── WALLET / POINTS ──────────────────────────────────────────────────────────

func newAdminGrantPointsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_grant_points", "Grant/Deduct Points",
		"Add or deduct points from a user's wallet. Use negative amount to deduct.",
		objSchema(map[string]interface{}{
			"user_id": numProp("User ID"),
			"amount":  numProp("Points to add (positive) or deduct (negative)"),
			"reason":  strProp("Reason for the adjustment"),
		}, "user_id", "amount"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			amount := args["amount"].(float64)
			body := map[string]interface{}{"amount": int(amount)}
			if v, _ := args["reason"].(string); v != "" {
				body["reason"] = v
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/admin/users/%.0f/points", id), token, body)
		})
}

func newAdminGetPointsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_get_points", "Get User Points",
		"Get a user's current point balance.",
		objSchema(map[string]interface{}{"user_id": numProp("User ID")}, "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["user_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, fmt.Sprintf("/api/admin/users/%.0f/points", id), token, nil)
		})
}

// ─── TEAMS ────────────────────────────────────────────────────────────────────

func newAdminListTeamsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_teams", "List All Teams",
		"List all teams on the platform.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/teams", token, nil)
		})
}

func newAdminGetTeamTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_get_team", "Get Team Details",
		"Get details for a specific team.",
		objSchema(map[string]interface{}{"team_id": numProp("Team ID")}, "team_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["team_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, fmt.Sprintf("/api/admin/teams/%.0f", id), token, nil)
		})
}

func newAdminUpdateTeamTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_team", "Update Team",
		"Update a team's properties.",
		objSchema(map[string]interface{}{
			"team_id": numProp("Team ID"),
			"name":    strProp("Team name"),
			"status":  strProp("Status: active, suspended"),
		}, "team_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["team_id"].(float64)
			body := map[string]interface{}{}
			for _, f := range []string{"name", "status"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/admin/teams/%.0f", id), token, body)
		})
}

func newAdminDeleteTeamTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_team", "Delete Team",
		"Delete a team.",
		objSchema(map[string]interface{}{"team_id": numProp("Team ID")}, "team_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["team_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/teams/%.0f", id), token, nil)
		})
}

func newAdminAddTeamMemberTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_add_team_member", "Add Team Member",
		"Add a user to a team.",
		objSchema(map[string]interface{}{
			"team_id": numProp("Team ID"),
			"user_id": numProp("User ID to add"),
			"role":    strProp("Member role: owner, manager, member"),
		}, "team_id", "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			tid := args["team_id"].(float64)
			body := map[string]interface{}{"user_id": args["user_id"]}
			if v, _ := args["role"].(string); v != "" {
				body["role"] = v
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/admin/teams/%.0f/members", tid), token, body)
		})
}

func newAdminRemoveTeamMemberTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_remove_team_member", "Remove Team Member",
		"Remove a user from a team.",
		objSchema(map[string]interface{}{
			"team_id": numProp("Team ID"),
			"user_id": numProp("User ID to remove"),
		}, "team_id", "user_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			tid := args["team_id"].(float64)
			uid := args["user_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/teams/%.0f/members/%.0f", tid, uid), token, nil)
		})
}

// ─── MARKETPLACE APPROVAL ─────────────────────────────────────────────────────

func newAdminListPendingMarketplaceTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_pending_marketplace", "List Pending Marketplace",
		"List community posts pending marketplace approval.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/marketplace/pending", token, nil)
		})
}

func newAdminApproveMarketplaceTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_approve_marketplace", "Approve Marketplace Submission",
		"Approve a pending marketplace submission (publishes it).",
		objSchema(map[string]interface{}{"post_id": numProp("Community post ID")}, "post_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["post_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/admin/marketplace/%.0f/approve", id), token, map[string]interface{}{})
		})
}

func newAdminRejectMarketplaceTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_reject_marketplace", "Reject Marketplace Submission",
		"Reject a pending marketplace submission.",
		objSchema(map[string]interface{}{
			"post_id": numProp("Community post ID"),
			"reason":  strProp("Rejection reason shown to submitter"),
		}, "post_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["post_id"].(float64)
			body := map[string]interface{}{}
			if v, _ := args["reason"].(string); v != "" {
				body["reason"] = v
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/admin/marketplace/%.0f/reject", id), token, body)
		})
}

// ─── BLOG POSTS ───────────────────────────────────────────────────────────────

func newAdminListPostsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_posts", "List Blog Posts",
		"List all blog posts.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/posts", token, nil)
		})
}

func newAdminCreatePostTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_create_post", "Create Blog Post",
		"Create a new blog post.",
		objSchema(map[string]interface{}{
			"title":   strProp("Post title"),
			"slug":    strProp("URL slug (auto-generated if omitted)"),
			"excerpt": strProp("Short summary"),
			"content": strProp("Full content in Markdown or HTML"),
			"status":  strProp("Status: draft, published (default: draft)"),
		}, "title"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			body := map[string]interface{}{}
			for _, f := range []string{"title", "slug", "excerpt", "content", "status"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/posts", token, body)
		})
}

func newAdminUpdatePostTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_post", "Update Blog Post",
		"Update an existing blog post.",
		objSchema(map[string]interface{}{
			"post_id": numProp("Post ID"),
			"title":   strProp("Title"),
			"slug":    strProp("URL slug"),
			"excerpt": strProp("Short excerpt"),
			"content": strProp("Content in Markdown or HTML"),
			"status":  strProp("Status: draft, published"),
			"locale":  strProp("Locale for translation (default: en)"),
		}, "post_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["post_id"].(float64)
			body := map[string]interface{}{}
			for _, f := range []string{"title", "slug", "excerpt", "content", "status", "locale"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPut, fmt.Sprintf("/api/admin/posts/%.0f", id), token, body)
		})
}

func newAdminDeletePostTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_post", "Delete Blog Post",
		"Delete a blog post.",
		objSchema(map[string]interface{}{"post_id": numProp("Post ID")}, "post_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["post_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/posts/%.0f", id), token, nil)
		})
}

// ─── CMS PAGES ────────────────────────────────────────────────────────────────

func newAdminListPagesTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_pages", "List CMS Pages",
		"List all CMS/marketing pages.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/pages", token, nil)
		})
}

func newAdminCreatePageTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_create_page", "Create CMS Page",
		"Create a new CMS/marketing page.",
		objSchema(map[string]interface{}{
			"title":   strProp("Page title"),
			"slug":    strProp("URL slug (e.g. features-overview)"),
			"content": strProp("Content in Markdown or HTML"),
			"status":  strProp("Status: draft, published (default: draft)"),
		}, "title"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			body := map[string]interface{}{}
			for _, f := range []string{"title", "slug", "content", "status"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/pages", token, body)
		})
}

func newAdminUpdatePageTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_page", "Update CMS Page",
		"Update an existing CMS/marketing page.",
		objSchema(map[string]interface{}{
			"page_id": numProp("Page ID"),
			"title":   strProp("Title"),
			"slug":    strProp("URL slug"),
			"content": strProp("Content in Markdown or HTML"),
			"status":  strProp("Status: draft, published"),
			"locale":  strProp("Locale (default: en)"),
		}, "page_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["page_id"].(float64)
			body := map[string]interface{}{}
			for _, f := range []string{"title", "slug", "content", "status", "locale"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPut, fmt.Sprintf("/api/admin/pages/%.0f", id), token, body)
		})
}

func newAdminDeletePageTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_page", "Delete CMS Page",
		"Delete a CMS page.",
		objSchema(map[string]interface{}{"page_id": numProp("Page ID")}, "page_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["page_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/pages/%.0f", id), token, nil)
		})
}

// ─── CATEGORIES ───────────────────────────────────────────────────────────────

func newAdminListCategoriesTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_categories", "List Categories",
		"List all content categories.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/categories", token, nil)
		})
}

func newAdminCreateCategoryTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_create_category", "Create Category",
		"Create a new content category.",
		objSchema(map[string]interface{}{
			"name":        strProp("Category name"),
			"slug":        strProp("URL slug"),
			"description": strProp("Description"),
		}, "name"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			body := map[string]interface{}{}
			for _, f := range []string{"name", "slug", "description"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/categories", token, body)
		})
}

func newAdminDeleteCategoryTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_category", "Delete Category",
		"Delete a content category.",
		objSchema(map[string]interface{}{"category_id": numProp("Category ID")}, "category_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["category_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/categories/%.0f", id), token, nil)
		})
}

// ─── SPONSORS ─────────────────────────────────────────────────────────────────

func newAdminListSponsorsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_sponsors", "List Sponsors",
		"List all sponsors.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/sponsors", token, nil)
		})
}

func newAdminCreateSponsorTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_create_sponsor", "Create Sponsor",
		"Add a new sponsor.",
		objSchema(map[string]interface{}{
			"name":        strProp("Sponsor name"),
			"logo_url":    strProp("Logo image URL"),
			"website_url": strProp("Website URL"),
			"tier":        strProp("Tier: gold, silver, bronze (default: silver)"),
			"description": strProp("Sponsor description"),
			"order":       numProp("Display order"),
			"status":      strProp("Status: active, inactive (default: active)"),
		}, "name"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			body := map[string]interface{}{}
			for _, f := range []string{"name", "logo_url", "website_url", "tier", "description", "status"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			if v, _ := args["order"].(float64); v != 0 {
				body["order"] = int(v)
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/sponsors", token, body)
		})
}

func newAdminUpdateSponsorTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_sponsor", "Update Sponsor",
		"Update a sponsor's details.",
		objSchema(map[string]interface{}{
			"sponsor_id":  numProp("Sponsor ID"),
			"name":        strProp("Name"),
			"logo_url":    strProp("Logo URL"),
			"website_url": strProp("Website URL"),
			"tier":        strProp("Tier: gold, silver, bronze"),
			"description": strProp("Description"),
			"status":      strProp("Status: active, inactive"),
			"order":       numProp("Display order"),
		}, "sponsor_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["sponsor_id"].(float64)
			body := map[string]interface{}{}
			for _, f := range []string{"name", "logo_url", "website_url", "tier", "description", "status"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			if v, _ := args["order"].(float64); v != 0 {
				body["order"] = int(v)
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPut, fmt.Sprintf("/api/admin/sponsors/%.0f", id), token, body)
		})
}

func newAdminDeleteSponsorTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_sponsor", "Delete Sponsor",
		"Delete a sponsor.",
		objSchema(map[string]interface{}{"sponsor_id": numProp("Sponsor ID")}, "sponsor_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["sponsor_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/sponsors/%.0f", id), token, nil)
		})
}

// ─── COMMUNITY ────────────────────────────────────────────────────────────────

func newAdminListCommunityPostsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_community_posts", "List Community Posts",
		"List all community posts with metadata.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/community/posts", token, nil)
		})
}

func newAdminUpdateCommunityPostTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_community_post", "Update Community Post",
		"Update a community post's status or content.",
		objSchema(map[string]interface{}{
			"post_id": numProp("Post ID"),
			"status":  strProp("Status: published, pending, removed"),
			"title":   strProp("Title"),
			"content": strProp("Content"),
		}, "post_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["post_id"].(float64)
			body := map[string]interface{}{}
			for _, f := range []string{"status", "title", "content"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/admin/community/posts/%.0f", id), token, body)
		})
}

func newAdminDeleteCommunityPostTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_community_post", "Delete Community Post",
		"Delete a community post.",
		objSchema(map[string]interface{}{"post_id": numProp("Post ID")}, "post_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["post_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/community/posts/%.0f", id), token, nil)
		})
}

// ─── ISSUES ───────────────────────────────────────────────────────────────────

func newAdminListIssuesTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_issues", "List User Issues",
		"List user-reported issues.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/issues", token, nil)
		})
}

func newAdminUpdateIssueTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_issue", "Update Issue",
		"Update an issue's status or resolution.",
		objSchema(map[string]interface{}{
			"issue_id":   numProp("Issue ID"),
			"status":     strProp("Status: open, in_progress, resolved, closed"),
			"resolution": strProp("Resolution notes"),
		}, "issue_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["issue_id"].(float64)
			body := map[string]interface{}{}
			for _, f := range []string{"status", "resolution"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/admin/issues/%.0f", id), token, body)
		})
}

// ─── CONTACT ──────────────────────────────────────────────────────────────────

func newAdminListContactTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_contact", "List Contact Submissions",
		"List all contact form submissions.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/contact", token, nil)
		})
}

func newAdminDeleteContactTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_contact", "Delete Contact Submission",
		"Delete a contact form submission.",
		objSchema(map[string]interface{}{"contact_id": numProp("Contact submission ID")}, "contact_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["contact_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/contact/%.0f", id), token, nil)
		})
}

// ─── NOTIFICATIONS ────────────────────────────────────────────────────────────

func newAdminSendNotificationTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_send_notification", "Send Platform Notification",
		"Send an in-app notification to all users, a role group, or specific users.",
		objSchema(map[string]interface{}{
			"title":      strProp("Notification title"),
			"message":    strProp("Notification body"),
			"recipients": strProp("Target: 'all', 'role:admin', 'role:user', or 'user:{id}'"),
			"user_ids":   arrProp("Specific user IDs to notify"),
			"type":       strProp("Type: info, success, warning, error (default: info)"),
		}, "title", "message"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			body := map[string]interface{}{}
			for _, f := range []string{"title", "message", "recipients", "type"} {
				if v, _ := args[f].(string); v != "" {
					body[f] = v
				}
			}
			if v, _ := args["user_ids"].([]interface{}); len(v) > 0 {
				body["user_ids"] = v
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/notifications/send", token, body)
		})
}

func newAdminListNotificationsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_notifications", "List Sent Notifications",
		"List notifications sent by admins.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/notifications", token, nil)
		})
}

// ─── CONTENT MODERATION ───────────────────────────────────────────────────────

func newAdminListContentTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_content", "List All Content",
		"List all shared content with pagination and filtering.",
		objSchema(map[string]interface{}{
			"page":       numProp("Page number"),
			"limit":      numProp("Results per page"),
			"q":          strProp("Search query"),
			"visibility": strProp("Filter: public, unlisted, private"),
		}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			q := url.Values{}
			for _, k := range []string{"q", "visibility"} {
				if v, _ := args[k].(string); v != "" {
					q.Set(k, v)
				}
			}
			for _, k := range []string{"page", "limit"} {
				if v, _ := args[k].(float64); v > 0 {
					q.Set(k, fmt.Sprintf("%.0f", v))
				}
			}
			path := "/api/admin/content"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, path, token, nil)
		})
}

func newAdminUpdateContentVisibilityTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_content_visibility", "Update Content Visibility",
		"Change the visibility of a content item.",
		objSchema(map[string]interface{}{
			"content_id": numProp("Content ID"),
			"visibility": strProp("Visibility: public, unlisted, private"),
		}, "content_id", "visibility"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["content_id"].(float64)
			visibility, _ := args["visibility"].(string)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/admin/content/%.0f/visibility", id), token, map[string]interface{}{"visibility": visibility})
		})
}

func newAdminDeleteContentTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_content", "Delete Content",
		"Delete a content item.",
		objSchema(map[string]interface{}{"content_id": numProp("Content ID")}, "content_id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["content_id"].(float64)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/admin/content/%.0f", id), token, nil)
		})
}

// ─── SETTINGS ─────────────────────────────────────────────────────────────────

func newAdminGetSettingTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_get_setting", "Get Platform Setting",
		"Get a platform setting by key. Valid keys: general, features, homepage, agents, email, contact, pricing, download, integrations, seo, discord, slack, github, social, smart_prompts, coming_soon, marketplace, plugins, docs, blog, community, sponsors.",
		objSchema(map[string]interface{}{
			"key": strProp("Setting key (e.g. general, features, homepage, agents, email, contact, pricing, download, integrations, seo, discord, slack, github, social, smart_prompts, coming_soon)"),
		}, "key"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			key, _ := args["key"].(string)
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/settings/"+url.PathEscape(key), token, nil)
		})
}

func newAdminUpdateSettingTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_setting", "Update Platform Setting",
		"Update a platform setting. Pass the key and the full JSON value. "+
			"general: {site_name, tagline, url, support_email, maintenance_mode, version, github_url, discord_url}. "+
			"features: {registration_enabled, coming_soon, blog_enabled, community_enabled, marketplace_enabled, sponsors_enabled, teams_enabled, ai_agents_enabled, mcp_enabled, issues_enabled}. "+
			"homepage: {hero_title, hero_subtitle, hero_cta_label, hero_cta_url, features_title, features_subtitle, features:[]}. "+
			"agents: {system_prompt, model, temperature, max_tokens, enabled}. "+
			"email: {host, port, username, password, from_email, from_name, tls}. "+
			"contact: {email, phone, address, maps_url}. "+
			"pricing: {plans:[{name, price, features:[], cta}]}. "+
			"download: {platforms:[{name, version, url, checksum}]}. "+
			"integrations: {github_enabled, github_client_id, github_client_secret, google_enabled, discord_enabled, slack_enabled}. "+
			"seo: {meta_title, meta_description, og_image, robots_txt, google_analytics_id}. "+
			"discord: {webhook_url, client_id, client_secret, bot_token, guild_id}. "+
			"slack: {webhook_url, client_id, client_secret, bot_token}. "+
			"github: {token, org, repo, webhook_secret}. "+
			"social: {twitter, linkedin, instagram, youtube, tiktok, facebook}. "+
			"smart_prompts: [{title, content, category}]. "+
			"coming_soon: {enabled, title, subtitle, show_countdown, launch_date}.",
		objSchema(map[string]interface{}{
			"key":   strProp("Setting key"),
			"value": strProp("Full JSON value for this key (must be valid JSON string)"),
		}, "key", "value"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			key, _ := args["key"].(string)
			valueStr, _ := args["value"].(string)
			var parsed interface{}
			if err := json.Unmarshal([]byte(valueStr), &parsed); err != nil {
				return errResult(fmt.Sprintf("value must be valid JSON: %v", err)), nil
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, "/api/admin/settings/"+url.PathEscape(key), token, parsed)
		})
}

func newAdminSeedSettingsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_seed_settings", "Seed Platform Settings",
		"Seed all default platform settings. Safe to run multiple times — only fills in missing keys.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/settings/seed", token, map[string]interface{}{})
		})
}

func newAdminTestEmailTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_test_email", "Test Email Config",
		"Send a test email to verify SMTP configuration.",
		objSchema(map[string]interface{}{
			"to": strProp("Recipient email address (defaults to admin's email)"),
		}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			body := map[string]interface{}{}
			if v, _ := args["to"].(string); v != "" {
				body["to"] = v
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/settings/test-email", token, body)
		})
}

func newAdminGenerateSitemapTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_generate_sitemap", "Generate Sitemap",
		"Generate the XML sitemap for the platform.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/settings/generate-sitemap", token, map[string]interface{}{})
		})
}

// ─── GITHUB SYNC ──────────────────────────────────────────────────────────────

func newAdminListGitHubReposTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_github_repos", "List GitHub Repos",
		"List connected GitHub repositories.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/github/repos", token, nil)
		})
}

func newAdminSyncGitHubIssuesTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_sync_github_issues", "Sync GitHub Issues",
		"Sync issues from connected GitHub repositories.",
		objSchema(map[string]interface{}{}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/github/sync", token, map[string]interface{}{})
		})
}

// ── Admin Docs (framework documentation) ──────────────────────────────────────

func newAdminListDocsTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_list_docs", "List Framework Docs",
		"List all framework documentation pages (the /docs section of the site).",
		objSchema(map[string]interface{}{
			"category": strProp("Filter by category (optional)"),
		}),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			path := "/api/docs"
			if cat, _ := args["category"].(string); cat != "" {
				path += "?category=" + cat
			}
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, path, token, nil)
		})
}

func newAdminGetDocTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_get_doc", "Get Framework Doc",
		"Get a single framework documentation page by ID.",
		objSchema(map[string]interface{}{
			"id": strProp("Doc ID"),
		}, "id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodGet, fmt.Sprintf("/api/docs/%v", args["id"]), token, nil)
		})
}

func newAdminCreateDocTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_create_doc", "Create Framework Doc",
		"Create a new framework documentation page.",
		objSchema(map[string]interface{}{
			"title":    strProp("Doc title"),
			"body":     strProp("Doc content (Markdown)"),
			"category": strProp("Category for grouping (e.g. guides, api, reference)"),
			"icon":     strProp("Icon emoji or identifier"),
			"color":    strProp("Color code or name"),
			"pinned":   boolProp("Pin this doc"),
			"published": boolProp("Publish publicly"),
		}, "title", "body"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/docs", token, args)
		})
}

func newAdminUpdateDocTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_update_doc", "Update Framework Doc",
		"Update an existing framework documentation page.",
		objSchema(map[string]interface{}{
			"id":       strProp("Doc ID"),
			"title":    strProp("New title"),
			"body":     strProp("New content (Markdown)"),
			"category": strProp("New category"),
			"icon":     strProp("New icon"),
			"color":    strProp("New color"),
			"published": boolProp("Publish/unpublish"),
		}, "id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["id"]
			delete(args, "id")
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPut, fmt.Sprintf("/api/docs/%v", id), token, args)
		})
}

func newAdminPinDocTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_pin_doc", "Pin/Unpin Framework Doc",
		"Toggle the pinned state of a framework documentation page.",
		objSchema(map[string]interface{}{
			"id": strProp("Doc ID"),
		}, "id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/docs/%v/pin", args["id"]), token, map[string]interface{}{})
		})
}

func newAdminDeleteDocTool(db *gorm.DB, cfg *config.Config) Tool {
	return adminTool(db, cfg, "admin_delete_doc", "Delete Framework Doc",
		"Delete a framework documentation page by ID.",
		objSchema(map[string]interface{}{
			"id": strProp("Doc ID"),
		}, "id"),
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return adminFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/docs/%v", args["id"]), token, nil)
		})
}

// suppress unused import warning for boolProp
var _ = boolProp
