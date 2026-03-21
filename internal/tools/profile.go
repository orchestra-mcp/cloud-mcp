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
			if handle := strField(profile, "handle"); handle != "" {
				text += fmt.Sprintf("**Profile URL:** /@%s\n", handle)
			}
			if about := strField(profile, "about"); about != "" {
				text += fmt.Sprintf("\n---\n### About\n%s\n", about)
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
			Description: "Update the authenticated user's Orchestra profile — name, bio, social links, handle, privacy toggles, appearance, and more. " +
				"Requires the 'Update my profile' permission toggle to be ON at orchestra-mcp.dev/settings/mcp",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Display name",
					},
					"email": map[string]interface{}{
						"type":        "string",
						"description": "Email address",
					},
					"phone": map[string]interface{}{
						"type":        "string",
						"description": "Phone number",
					},
					"gender": map[string]interface{}{
						"type":        "string",
						"description": "Gender",
					},
					"position": map[string]interface{}{
						"type":        "string",
						"description": "Job title / position",
					},
					"timezone": map[string]interface{}{
						"type":        "string",
						"description": "IANA timezone (e.g. America/New_York, Europe/Paris)",
					},
					"bio": map[string]interface{}{
						"type":        "string",
						"description": "Short one-line bio or tagline",
					},
					"about": map[string]interface{}{
						"type":        "string",
						"description": "Full about page content in Markdown — rendered at /@handle/about",
					},
					"handle": map[string]interface{}{
						"type":        "string",
						"description": "Public profile handle / username (used in public profile URL)",
					},
					"cover_url": map[string]interface{}{
						"type":        "string",
						"description": "URL of the profile cover/banner image",
					},
					"social_links": map[string]interface{}{
						"type":        "object",
						"description": "Social links object — keys: twitter, linkedin, github, instagram, website, youtube, etc.",
					},
					"public_profile_enabled": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether the public profile page is enabled",
					},
					"show_badges": map[string]interface{}{
						"type":        "boolean",
						"description": "Show badges on public profile",
					},
					"show_wallet": map[string]interface{}{
						"type":        "boolean",
						"description": "Show wallet/points on public profile",
					},
					"show_teams": map[string]interface{}{
						"type":        "boolean",
						"description": "Show team memberships on public profile",
					},
					"show_sponsors": map[string]interface{}{
						"type":        "boolean",
						"description": "Show sponsors section on public profile",
					},
					"show_comments_on_profile": map[string]interface{}{
						"type":        "boolean",
						"description": "Allow comments on public profile",
					},
					"appearance": map[string]interface{}{
						"type":        "object",
						"description": "Appearance/theme customization object for public profile",
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
			for _, field := range []string{"name", "email", "phone", "gender", "position", "timezone", "bio", "about", "handle", "cover_url"} {
				if v, ok := args[field]; ok && v != nil {
					if s, ok := v.(string); ok && s != "" {
						update[field] = s
					}
				}
			}
			for _, field := range []string{"public_profile_enabled", "show_badges", "show_wallet", "show_teams", "show_sponsors", "show_comments_on_profile"} {
				if v, ok := args[field]; ok && v != nil {
					update[field] = v
				}
			}
			for _, field := range []string{"social_links", "appearance"} {
				if v, ok := args[field]; ok && v != nil {
					update[field] = v
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
