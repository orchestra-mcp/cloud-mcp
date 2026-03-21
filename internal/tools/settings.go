package tools

import (
	"fmt"

	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"github.com/orchestra-mcp/cloud-mcp/internal/permissions"
	"github.com/orchestra-mcp/cloud-mcp/internal/protocol"
)

// userSettingsTool builds a user settings tool (requires mcp.content permission).
func userSettingsTool(cfg *config.Config, name, title, desc string, schema map[string]interface{}, fn func(args map[string]interface{}, token string) (protocol.ToolResult, error)) Tool {
	return Tool{
		Permission: permissions.PermContent,
		Definition: protocol.ToolDefinition{
			Name:        name,
			Title:       title,
			Description: desc,
			InputSchema: schema,
			Annotations: &protocol.ToolAnnotations{Title: title},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult("No token available. Reconnect with your Orchestra token."), nil
			}
			return fn(args, token)
		},
	}
}

// ── Preferences ─────────────────────────────────────────────────────────────

func newGetPreferencesTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermProfileRead,
		Definition: protocol.ToolDefinition{
			Name:        "get_preferences",
			Title:       "Get My Preferences",
			Description: "Get the current user's app preferences (theme, language, editor settings, notification preferences).",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{Title: "Get My Preferences", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult("No token available."), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "GET", "/api/settings/preferences", token, nil)
		},
	}
}

func newUpdatePreferencesTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "update_preferences", "Update My Preferences",
		"Update app preferences: theme (dark/light/system), language, editor font size, tab size, word wrap, compact mode, notification toggles.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"theme": map[string]interface{}{
					"type":        "string",
					"description": "UI theme: dark, light, or system",
					"enum":        []string{"dark", "light", "system"},
				},
				"language": map[string]interface{}{
					"type":        "string",
					"description": "UI language code (e.g. en, fr, de, es)",
				},
				"sidebar_collapsed": map[string]interface{}{
					"type":        "boolean",
					"description": "Keep sidebar collapsed by default",
				},
				"notifications_email": map[string]interface{}{
					"type":        "boolean",
					"description": "Receive email notifications",
				},
				"notifications_push": map[string]interface{}{
					"type":        "boolean",
					"description": "Receive push notifications",
				},
				"notifications_sound": map[string]interface{}{
					"type":        "boolean",
					"description": "Play sound for notifications",
				},
				"editor_font_size": map[string]interface{}{
					"type":        "integer",
					"description": "Editor font size in pixels (e.g. 14)",
				},
				"editor_tab_size": map[string]interface{}{
					"type":        "integer",
					"description": "Editor tab size (e.g. 2 or 4)",
				},
				"editor_word_wrap": map[string]interface{}{
					"type":        "boolean",
					"description": "Enable word wrap in editor",
				},
				"compact_mode": map[string]interface{}{
					"type":        "boolean",
					"description": "Enable compact/dense UI mode",
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, "PATCH", "/api/settings/preferences", token, args)
		},
	)
}

// ── Notifications ────────────────────────────────────────────────────────────

func newListNotificationsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermProfileRead,
		Definition: protocol.ToolDefinition{
			Name:        "list_notifications",
			Title:       "List My Notifications",
			Description: "List the current user's notifications (last 50, newest first).",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{Title: "List My Notifications", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult("No token available."), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "GET", "/api/notifications", token, nil)
		},
	}
}

func newMarkNotificationReadTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "mark_notification_read", "Mark Notification Read",
		"Mark a specific notification as read by its ID.",
		map[string]interface{}{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Notification ID",
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return errResult("id is required"), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "PATCH", fmt.Sprintf("/api/notifications/%s/read", id), token, nil)
		},
	)
}

func newMarkAllNotificationsReadTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "mark_all_notifications_read", "Mark All Notifications Read",
		"Mark all notifications as read.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, "PATCH", "/api/notifications/read-all", token, nil)
		},
	)
}

func newDeleteNotificationTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "delete_notification", "Delete Notification",
		"Delete a specific notification by its ID.",
		map[string]interface{}{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Notification ID",
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return errResult("id is required"), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "DELETE", fmt.Sprintf("/api/notifications/%s", id), token, nil)
		},
	)
}

// ── API Keys ─────────────────────────────────────────────────────────────────

func newListAPIKeysTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermProfileRead,
		Definition: protocol.ToolDefinition{
			Name:        "list_api_keys",
			Title:       "List My API Keys",
			Description: "List the current user's Orchestra API keys (names and prefixes only — full tokens are not shown after creation).",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{Title: "List My API Keys", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult("No token available."), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "GET", "/api/settings/api-keys", token, nil)
		},
	}
}

func newCreateAPIKeyTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "create_api_key", "Create API Key",
		"Create a new named API key. Returns the full token once — save it immediately.",
		map[string]interface{}{
			"type":     "object",
			"required": []string{"name"},
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Descriptive name for the API key (e.g. 'CI/CD Pipeline', 'Cursor Integration')",
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, "POST", "/api/settings/api-keys", token, args)
		},
	)
}

func newRevokeAPIKeyTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "revoke_api_key", "Revoke API Key",
		"Permanently revoke an API key by its ID. This cannot be undone.",
		map[string]interface{}{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "API key ID",
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return errResult("id is required"), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "DELETE", fmt.Sprintf("/api/settings/api-keys/%s", id), token, nil)
		},
	)
}

// ── Sessions ─────────────────────────────────────────────────────────────────

func newListSessionsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermProfileRead,
		Definition: protocol.ToolDefinition{
			Name:        "list_sessions",
			Title:       "List My Sessions",
			Description: "List active sessions / connected devices for the current user.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{Title: "List My Sessions", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult("No token available."), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "GET", "/api/settings/sessions", token, nil)
		},
	}
}

func newRevokeSessionTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "revoke_session", "Revoke Session",
		"Revoke a session / disconnect a device by session ID. Cannot revoke 'current' session.",
		map[string]interface{}{
			"type":     "object",
			"required": []string{"id"},
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "Session ID (e.g. 'device:abc123')",
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id, _ := args["id"].(string)
			if id == "" {
				return errResult("id is required"), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "DELETE", fmt.Sprintf("/api/settings/sessions/%s", id), token, nil)
		},
	)
}

// ── Connected Accounts / OAuth ───────────────────────────────────────────────

func newListConnectedAccountsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermProfileRead,
		Definition: protocol.ToolDefinition{
			Name:        "list_connected_accounts",
			Title:       "List Connected Accounts",
			Description: "List OAuth accounts connected to the current user (GitHub, Google, etc.).",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{Title: "List Connected Accounts", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult("No token available."), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "GET", "/api/settings/connected-accounts", token, nil)
		},
	}
}

func newUnlinkAccountTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "unlink_account", "Unlink OAuth Account",
		"Unlink an OAuth provider account (e.g. github, google) from the current user.",
		map[string]interface{}{
			"type":     "object",
			"required": []string{"provider"},
			"properties": map[string]interface{}{
				"provider": map[string]interface{}{
					"type":        "string",
					"description": "OAuth provider name (e.g. github, google)",
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			provider, _ := args["provider"].(string)
			if provider == "" {
				return errResult("provider is required"), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "DELETE", fmt.Sprintf("/api/settings/connected-accounts/%s", provider), token, nil)
		},
	)
}

// ── Integrations (Discord / Slack) ──────────────────────────────────────────

func newListIntegrationsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermProfileRead,
		Definition: protocol.ToolDefinition{
			Name:        "list_integrations",
			Title:       "List My Integrations",
			Description: "List the current user's Discord and Slack integration configurations.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{Title: "List My Integrations", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult("No token available."), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "GET", "/api/settings/integrations/user", token, nil)
		},
	}
}

func newUpsertIntegrationTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "upsert_integration", "Configure Integration",
		"Configure or update a Discord or Slack integration for the current user.",
		map[string]interface{}{
			"type":     "object",
			"required": []string{"provider"},
			"properties": map[string]interface{}{
				"provider": map[string]interface{}{
					"type":        "string",
					"description": "Integration provider: discord or slack",
					"enum":        []string{"discord", "slack"},
				},
				"guild_id": map[string]interface{}{
					"type":        "string",
					"description": "Discord server/guild ID",
				},
				"channel_id": map[string]interface{}{
					"type":        "string",
					"description": "Discord or Slack channel ID",
				},
				"team_id": map[string]interface{}{
					"type":        "string",
					"description": "Slack workspace/team ID",
				},
				"webhook_url": map[string]interface{}{
					"type":        "string",
					"description": "Incoming webhook URL",
				},
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether this integration is enabled",
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			provider, _ := args["provider"].(string)
			if provider == "" {
				return errResult("provider is required"), nil
			}
			body := map[string]interface{}{}
			for _, f := range []string{"guild_id", "channel_id", "team_id", "webhook_url"} {
				if v, ok := args[f].(string); ok && v != "" {
					body[f] = v
				}
			}
			if v, ok := args["enabled"]; ok && v != nil {
				body["enabled"] = v
			}
			return userFetchText(cfg.WebAPIBaseURL, "PUT", fmt.Sprintf("/api/settings/integrations/user/%s", provider), token, body)
		},
	)
}

func newDeleteIntegrationTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "delete_integration", "Remove Integration",
		"Remove a Discord or Slack integration for the current user.",
		map[string]interface{}{
			"type":     "object",
			"required": []string{"provider"},
			"properties": map[string]interface{}{
				"provider": map[string]interface{}{
					"type":        "string",
					"description": "Integration provider: discord or slack",
					"enum":        []string{"discord", "slack"},
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			provider, _ := args["provider"].(string)
			if provider == "" {
				return errResult("provider is required"), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "DELETE", fmt.Sprintf("/api/settings/integrations/user/%s", provider), token, nil)
		},
	)
}

// ── Issues (support tickets) ─────────────────────────────────────────────────

func newListMyIssuesTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermProfileRead,
		Definition: protocol.ToolDefinition{
			Name:        "list_my_issues",
			Title:       "List My Support Issues",
			Description: "List support issues / bug reports submitted by the current user.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{Title: "List My Support Issues", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult("No token available."), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, "GET", "/api/issues", token, nil)
		},
	}
}

func newCreateIssueTool(cfg *config.Config) Tool {
	return userSettingsTool(cfg, "create_issue", "Create Support Issue",
		"Submit a support issue or bug report to the Orchestra team.",
		map[string]interface{}{
			"type":     "object",
			"required": []string{"title", "description"},
			"properties": map[string]interface{}{
				"title": map[string]interface{}{
					"type":        "string",
					"description": "Short title for the issue",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Detailed description of the issue",
				},
				"priority": map[string]interface{}{
					"type":        "string",
					"description": "Priority level: low, medium, or high",
					"enum":        []string{"low", "medium", "high"},
				},
			},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, "POST", "/api/issues", token, args)
		},
	)
}
