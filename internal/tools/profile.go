package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"github.com/orchestra-mcp/cloud-mcp/internal/protocol"
	"github.com/orchestra-mcp/cloud-mcp/internal/permissions"
	"gorm.io/gorm"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// newGetProfileTool returns the get_profile tool.
// Requires JWT + mcp.profile.read permission toggle.
func newGetProfileTool(db *gorm.DB, cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermProfileRead,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:  "get_profile",
			Title: "Get My Profile",
			Description: "Retrieve the authenticated user's Orchestra profile including name, email, role, plan, " +
				"timezone, and usage statistics. Requires the 'Read my profile' permission toggle to be ON at orchestra-mcp.dev/settings/mcp",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
			Annotations: &protocol.ToolAnnotations{
				Title:        "Get My Profile",
				ReadOnlyHint: &readOnly,
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			profile, err := fetchProfile(cfg.WebAPIBaseURL, userID)
			if err != nil {
				return protocol.ToolResult{
					Content: []protocol.Content{{Type: "text", Text: fmt.Sprintf("Failed to fetch profile: %v", err)}},
					IsError: true,
				}, nil
			}

			// Format as markdown.
			text := fmt.Sprintf("## Your Orchestra Profile\n\n"+
				"**Name:** %s\n"+
				"**Email:** %s\n"+
				"**Role:** %s\n"+
				"**Plan:** %s\n"+
				"**Member since:** %s\n",
				strField(profile, "name"),
				strField(profile, "email"),
				strField(profile, "role"),
				strField(profile, "plan"),
				strField(profile, "created_at"),
			)

			if tz := strField(profile, "timezone"); tz != "" {
				text += fmt.Sprintf("**Timezone:** %s\n", tz)
			}
			if gh := strField(profile, "github_username"); gh != "" {
				text += fmt.Sprintf("**GitHub:** @%s\n", gh)
			}
			if bio := strField(profile, "bio"); bio != "" {
				text += fmt.Sprintf("\n**Bio:** %s\n", bio)
			}

			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: text}},
			}, nil
		},
	}
}

// newUpdateProfileTool returns the update_profile tool.
// Requires JWT + mcp.profile.write permission toggle.
func newUpdateProfileTool(db *gorm.DB, cfg *config.Config) Tool {
	idempotent := true
	return Tool{
		Permission:   permissions.PermProfileWrite,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:  "update_profile",
			Title: "Update My Profile",
			Description: "Update the authenticated user's Orchestra profile fields. " +
				"Requires the 'Update my profile' permission toggle to be ON at orchestra-mcp.dev/settings/mcp",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Display name",
					},
					"timezone": map[string]interface{}{
						"type":        "string",
						"description": "IANA timezone (e.g. America/New_York, Europe/Paris)",
					},
					"github_username": map[string]interface{}{
						"type":        "string",
						"description": "GitHub username (without @)",
					},
					"bio": map[string]interface{}{
						"type":        "string",
						"description": "Short bio or description",
					},
				},
			},
			Annotations: &protocol.ToolAnnotations{
				Title:          "Update My Profile",
				IdempotentHint: &idempotent,
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			// Build update payload — only include provided fields.
			update := map[string]interface{}{}
			for _, field := range []string{"name", "timezone", "github_username", "bio"} {
				if v, ok := args[field]; ok && v != nil {
					if s, ok := v.(string); ok && s != "" {
						update[field] = s
					}
				}
			}

			if len(update) == 0 {
				return protocol.ToolResult{
					Content: []protocol.Content{{Type: "text", Text: "No fields provided to update."}},
					IsError: true,
				}, nil
			}

			profile, err := patchProfile(cfg.WebAPIBaseURL, userID, update)
			if err != nil {
				return protocol.ToolResult{
					Content: []protocol.Content{{Type: "text", Text: fmt.Sprintf("Failed to update profile: %v", err)}},
					IsError: true,
				}, nil
			}

			// Build confirmation message.
			text := "Profile updated successfully!\n\n"
			for k, v := range update {
				text += fmt.Sprintf("- **%s** → %v\n", k, v)
			}
			_ = profile
			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: text}},
			}, nil
		},
	}
}

// fetchProfile calls the web API to get the user's profile.
func fetchProfile(baseURL string, userID uint) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/mcp/profile?user_id=%d", baseURL, userID)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// patchProfile calls the web API to update the user's profile.
func patchProfile(baseURL string, userID uint, update map[string]interface{}) (map[string]interface{}, error) {
	update["user_id"] = userID
	body, _ := json.Marshal(update)
	url := fmt.Sprintf("%s/api/mcp/profile", baseURL)

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	return result, nil
}

func strField(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
