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
func adminAPIFetch(baseURL, method, path, token string, body interface{}) (map[string]interface{}, error) {
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

	var result map[string]interface{}
	if len(respBody) > 0 {
		json.Unmarshal(respBody, &result)
	}
	return result, nil
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

// newAdminPlatformStatsTool returns platform usage statistics.
func newAdminPlatformStatsTool(db *gorm.DB, cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:  "admin_platform_stats",
			Title: "Platform Statistics",
			Description: "Admin: Get platform-wide statistics — total users, active users, admin count. " +
				"Requires admin role.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{Title: "Platform Statistics", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			var total, active, admins int64
			db.Model(&auth.User{}).Count(&total)
			db.Model(&auth.User{}).Where("status = ?", "active").Count(&active)
			db.Model(&auth.User{}).Where("role = ?", "admin").Count(&admins)

			text := fmt.Sprintf("## Platform Statistics\n\n"+
				"**Total users:** %d\n"+
				"**Active users:** %d\n"+
				"**Admin users:** %d\n",
				total, active, admins)

			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: text}},
			}, nil
		},
	}
}

// newAdminListUsersTool lists platform users with optional filters.
func newAdminListUsersTool(db *gorm.DB, cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:  "admin_list_users",
			Title: "List Users",
			Description: "Admin: List all platform users. Supports filtering by role, status, and search query. " +
				"Requires admin role.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"page":   numProp("Page number (default 1)"),
					"limit":  numProp("Results per page (default 20, max 100)"),
					"role":   strProp("Filter by role: admin, member, guest"),
					"status": strProp("Filter by status: active, blocked"),
					"q":      strProp("Search by name or email"),
				},
			},
			Annotations: &protocol.ToolAnnotations{Title: "List Users", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("Failed to get admin token: " + err.Error()), nil
			}

			q := url.Values{}
			if v, _ := args["page"].(float64); v > 0 {
				q.Set("page", fmt.Sprintf("%.0f", v))
			}
			if v, _ := args["limit"].(float64); v > 0 {
				q.Set("limit", fmt.Sprintf("%.0f", v))
			}
			if v, _ := args["role"].(string); v != "" {
				q.Set("role", v)
			}
			if v, _ := args["status"].(string); v != "" {
				q.Set("status", v)
			}
			if v, _ := args["q"].(string); v != "" {
				q.Set("q", v)
			}

			path := "/api/admin/users"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}

			result, err := adminAPIFetch(cfg.WebAPIBaseURL, http.MethodGet, path, token, nil)
			if err != nil {
				return errResult("Failed to list users: " + err.Error()), nil
			}

			b, _ := json.MarshalIndent(result, "", "  ")
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: string(b)}},
			}, nil
		},
	}
}

// newAdminUpdateUserRoleTool changes a user's role.
func newAdminUpdateUserRoleTool(db *gorm.DB, cfg *config.Config) Tool {
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:  "admin_update_user_role",
			Title: "Update User Role",
			Description: "Admin: Change a user's role (admin, member, guest). Requires admin role.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"user_id", "role"},
				"properties": map[string]interface{}{
					"user_id": numProp("Numeric user ID"),
					"role":    strProp("New role: admin, member, guest"),
				},
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("Failed to get admin token: " + err.Error()), nil
			}

			uid, _ := args["user_id"].(float64)
			role, _ := args["role"].(string)
			if uid == 0 || role == "" {
				return errResult("user_id and role are required"), nil
			}

			result, err := adminAPIFetch(cfg.WebAPIBaseURL, http.MethodPatch,
				fmt.Sprintf("/api/admin/users/%.0f/role", uid),
				token, map[string]interface{}{"role": role})
			if err != nil {
				return errResult("Failed to update role: " + err.Error()), nil
			}

			b, _ := json.MarshalIndent(result, "", "  ")
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: fmt.Sprintf("Role updated to %q.\n%s", role, string(b))}},
			}, nil
		},
	}
}

// newAdminGetSettingTool retrieves a platform setting by key.
func newAdminGetSettingTool(db *gorm.DB, cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:  "admin_get_setting",
			Title: "Get Platform Setting",
			Description: "Admin: Get a platform setting value by key (e.g. homepage_hero_title, seo_meta_description). " +
				"Requires admin role.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"key"},
				"properties": map[string]interface{}{
					"key": strProp("Setting key to retrieve"),
				},
			},
			Annotations: &protocol.ToolAnnotations{Title: "Get Platform Setting", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("Failed to get admin token: " + err.Error()), nil
			}

			key, _ := args["key"].(string)
			if key == "" {
				return errResult("key is required"), nil
			}

			result, err := adminAPIFetch(cfg.WebAPIBaseURL, http.MethodGet,
				"/api/admin/settings/"+url.PathEscape(key), token, nil)
			if err != nil {
				return errResult("Failed to get setting: " + err.Error()), nil
			}

			b, _ := json.MarshalIndent(result, "", "  ")
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: string(b)}},
			}, nil
		},
	}
}

// newAdminUpdateSettingTool updates a platform setting.
func newAdminUpdateSettingTool(db *gorm.DB, cfg *config.Config) Tool {
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:  "admin_update_setting",
			Title: "Update Platform Setting",
			Description: "Admin: Update a platform setting value by key. Use this to update homepage hero text, " +
				"SEO metadata, feature flags, download page content, pricing, etc. Requires admin role.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"key", "value"},
				"properties": map[string]interface{}{
					"key":   strProp("Setting key to update"),
					"value": strProp("New value for the setting"),
				},
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("Failed to get admin token: " + err.Error()), nil
			}

			key, _ := args["key"].(string)
			value, _ := args["value"].(string)
			if key == "" {
				return errResult("key is required"), nil
			}

			result, err := adminAPIFetch(cfg.WebAPIBaseURL, http.MethodPatch,
				"/api/admin/settings/"+url.PathEscape(key),
				token, map[string]interface{}{"value": value})
			if err != nil {
				return errResult("Failed to update setting: " + err.Error()), nil
			}

			b, _ := json.MarshalIndent(result, "", "  ")
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: fmt.Sprintf("Setting %q updated.\n%s", key, string(b))}},
			}, nil
		},
	}
}

// newAdminSeedSettingsTool seeds default platform settings.
func newAdminSeedSettingsTool(db *gorm.DB, cfg *config.Config) Tool {
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:  "admin_seed_settings",
			Title: "Seed Platform Settings",
			Description: "Admin: Seed all default platform settings (homepage content, SEO defaults, feature flags, etc.). " +
				"Safe to run multiple times — only fills in missing keys. Requires admin role.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("Failed to get admin token: " + err.Error()), nil
			}

			result, err := adminAPIFetch(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/settings/seed", token, map[string]interface{}{})
			if err != nil {
				return errResult("Failed to seed settings: " + err.Error()), nil
			}

			b, _ := json.MarshalIndent(result, "", "  ")
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: "Settings seeded successfully.\n" + string(b)}},
			}, nil
		},
	}
}

// newAdminListPagesTool lists CMS pages.
func newAdminListPagesTool(db *gorm.DB, cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:        "admin_list_pages",
			Title:       "List CMS Pages",
			Description: "Admin: List all CMS/marketing pages. Requires admin role.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{Title: "List CMS Pages", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("Failed to get admin token: " + err.Error()), nil
			}

			result, err := adminAPIFetch(cfg.WebAPIBaseURL, http.MethodGet, "/api/admin/pages", token, nil)
			if err != nil {
				return errResult("Failed to list pages: " + err.Error()), nil
			}

			b, _ := json.MarshalIndent(result, "", "  ")
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: string(b)}},
			}, nil
		},
	}
}

// newAdminCreatePageTool creates a CMS page.
func newAdminCreatePageTool(db *gorm.DB, cfg *config.Config) Tool {
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:  "admin_create_page",
			Title: "Create CMS Page",
			Description: "Admin: Create a new CMS/marketing page with a title, slug, and markdown/HTML content. " +
				"Use this to generate landing pages, feature pages, or blog posts via AI. Requires admin role.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"title", "slug", "content"},
				"properties": map[string]interface{}{
					"title":      strProp("Page title"),
					"slug":       strProp("URL slug (e.g. features-overview)"),
					"content":    strProp("Page content in Markdown or HTML"),
					"meta_title": strProp("SEO meta title (optional)"),
					"meta_desc":  strProp("SEO meta description (optional)"),
					"published":  boolProp("Whether to publish immediately (default false)"),
				},
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("Failed to get admin token: " + err.Error()), nil
			}

			payload := map[string]interface{}{}
			for _, f := range []string{"title", "slug", "content", "meta_title", "meta_desc"} {
				if v, ok := args[f].(string); ok && v != "" {
					payload[f] = v
				}
			}
			if pub, ok := args["published"].(bool); ok {
				payload["published"] = pub
			}

			result, err := adminAPIFetch(cfg.WebAPIBaseURL, http.MethodPost, "/api/admin/pages", token, payload)
			if err != nil {
				return errResult("Failed to create page: " + err.Error()), nil
			}

			b, _ := json.MarshalIndent(result, "", "  ")
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: "Page created.\n" + string(b)}},
			}, nil
		},
	}
}

// newAdminUpdatePageTool updates a CMS page.
func newAdminUpdatePageTool(db *gorm.DB, cfg *config.Config) Tool {
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:  "admin_update_page",
			Title: "Update CMS Page",
			Description: "Admin: Update an existing CMS/marketing page's content, title, or metadata. Requires admin role.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]interface{}{
					"id":         numProp("Page ID"),
					"title":      strProp("New title"),
					"content":    strProp("New content in Markdown or HTML"),
					"meta_title": strProp("SEO meta title"),
					"meta_desc":  strProp("SEO meta description"),
					"published":  boolProp("Published state"),
				},
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("Failed to get admin token: " + err.Error()), nil
			}

			id, _ := args["id"].(float64)
			if id == 0 {
				return errResult("id is required"), nil
			}

			payload := map[string]interface{}{}
			for _, f := range []string{"title", "content", "meta_title", "meta_desc"} {
				if v, ok := args[f].(string); ok && v != "" {
					payload[f] = v
				}
			}
			if pub, ok := args["published"].(bool); ok {
				payload["published"] = pub
			}

			result, err := adminAPIFetch(cfg.WebAPIBaseURL, http.MethodPut,
				fmt.Sprintf("/api/admin/pages/%.0f", id), token, payload)
			if err != nil {
				return errResult("Failed to update page: " + err.Error()), nil
			}

			b, _ := json.MarshalIndent(result, "", "  ")
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: "Page updated.\n" + string(b)}},
			}, nil
		},
	}
}

// newAdminSendNotificationTool sends a platform notification.
func newAdminSendNotificationTool(db *gorm.DB, cfg *config.Config) Tool {
	return Tool{
		Permission: permissions.PermAdmin,
		Definition: protocol.ToolDefinition{
			Name:  "admin_send_notification",
			Title: "Send Platform Notification",
			Description: "Admin: Send a push/in-app notification to all users or a specific user. Requires admin role.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"title", "body"},
				"properties": map[string]interface{}{
					"title":   strProp("Notification title"),
					"body":    strProp("Notification body text"),
					"user_id": numProp("Target user ID (omit to send to all users)"),
					"url":     strProp("Optional action URL"),
				},
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getAdminToken(userID, db)
			if err != nil {
				return errResult("Failed to get admin token: " + err.Error()), nil
			}

			payload := map[string]interface{}{}
			if v, _ := args["title"].(string); v != "" {
				payload["title"] = v
			}
			if v, _ := args["body"].(string); v != "" {
				payload["body"] = v
			}
			if v, _ := args["url"].(string); v != "" {
				payload["url"] = v
			}
			if v, _ := args["user_id"].(float64); v > 0 {
				payload["user_id"] = v
			}

			result, err := adminAPIFetch(cfg.WebAPIBaseURL, http.MethodPost,
				"/api/admin/notifications/send", token, payload)
			if err != nil {
				return errResult("Failed to send notification: " + err.Error()), nil
			}

			b, _ := json.MarshalIndent(result, "", "  ")
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: "Notification sent.\n" + string(b)}},
			}, nil
		},
	}
}

// getAdminToken retrieves the stored raw token for a user (set per-request by the registry).
func getAdminToken(userID uint, db *gorm.DB) (string, error) {
	tokenMu.RLock()
	token, ok := tokenStore[userID]
	tokenMu.RUnlock()
	if !ok || token == "" {
		return "", fmt.Errorf("no token available for user %d", userID)
	}
	return token, nil
}

// errResult creates an error ToolResult.
func errResult(msg string) protocol.ToolResult {
	return protocol.ToolResult{
		Content: []protocol.Content{{Type: "text", Text: msg}},
		IsError: true,
	}
}
