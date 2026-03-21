package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"github.com/orchestra-mcp/cloud-mcp/internal/permissions"
	"github.com/orchestra-mcp/cloud-mcp/internal/protocol"
)

// userAPIFetch calls the web API with the user's Bearer token.
// Unlike adminAPIFetch, this is for user-owned resources (skills, agents, etc.)
func userAPIFetch(baseURL, method, path, token string, body interface{}) (interface{}, error) {
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

	var result interface{}
	json.Unmarshal(respBody, &result)
	return result, nil
}

func userFetchText(baseURL, method, path, token string, body interface{}) (protocol.ToolResult, error) {
	result, err := userAPIFetch(baseURL, method, path, token, body)
	if err != nil {
		return protocol.ToolResult{
			Content: []protocol.Content{{Type: "text", Text: err.Error()}},
			IsError: true,
		}, nil
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	return protocol.ToolResult{
		Content: []protocol.Content{{Type: "text", Text: string(b)}},
	}, nil
}

// getUserToken reads the current user's raw token from the tokenStore.
func getUserToken(userID uint) (string, error) {
	tokenMu.RLock()
	token, ok := tokenStore[userID]
	tokenMu.RUnlock()
	if !ok || token == "" {
		return "", fmt.Errorf("no token available — reconnect with your Orchestra token")
	}
	return token, nil
}

// userTool builds a user-scoped tool that requires mcp.content permission.
func userTool(cfg *config.Config, name, title, desc string, schema map[string]interface{}, fn func(args map[string]interface{}, token string) (protocol.ToolResult, error)) Tool {
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
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
				return errResult(err.Error()), nil
			}
			return fn(args, token)
		},
	}
}

// ── Skills ──────────────────────────────────────────────────────────────────

func newListSkillsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "list_skills",
			Title:       "List My Skills",
			Description: "List all your Orchestra skills (slash commands for AI agents).",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Annotations: &protocol.ToolAnnotations{Title: "List My Skills", ReadOnlyHint: &readOnly, OpenWorldHint: protocol.BoolPtr(false)},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/skills", token, nil)
		},
	}
}

func newCreateSkillTool(cfg *config.Config) Tool {
	return userTool(cfg, "create_skill", "Create Skill", "Create a new Orchestra skill (slash command).",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":        objProp("string", "Skill name (used as /name slash command)"),
				"description": objProp("string", "What this skill does"),
				"content":     objProp("string", "The skill prompt/instructions"),
				"is_public":   objProp("boolean", "Whether the skill is publicly visible"),
			},
			"required": []string{"name", "content"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/skills", token, args)
		},
	)
}

func newGetSkillTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "get_skill",
			Title:       "Get Skill",
			Description: "Get a specific skill by ID.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"id": objProp("string", "Skill ID")},
				"required":   []string{"id"},
			},
			Annotations: &protocol.ToolAnnotations{Title: "Get Skill", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, fmt.Sprintf("/api/skills/%v", args["id"]), token, nil)
		},
	}
}

func newUpdateSkillTool(cfg *config.Config) Tool {
	return userTool(cfg, "update_skill", "Update Skill", "Update an existing skill.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":          objProp("string", "Skill ID"),
				"name":        objProp("string", "New name"),
				"description": objProp("string", "New description"),
				"content":     objProp("string", "New prompt content"),
				"is_public":   objProp("boolean", "Public visibility"),
			},
			"required": []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["id"]
			delete(args, "id")
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPut, fmt.Sprintf("/api/skills/%v", id), token, args)
		},
	)
}

func newDeleteSkillTool(cfg *config.Config) Tool {
	return userTool(cfg, "delete_skill", "Delete Skill", "Delete a skill by ID.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"id": objProp("string", "Skill ID")},
			"required":   []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/skills/%v", args["id"]), token, nil)
		},
	)
}

// ── Agents ──────────────────────────────────────────────────────────────────

func newListAgentsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "list_agents",
			Title:       "List My Agents",
			Description: "List all your Orchestra AI agents.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Annotations: &protocol.ToolAnnotations{Title: "List My Agents", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/agents", token, nil)
		},
	}
}

func newCreateAgentTool(cfg *config.Config) Tool {
	return userTool(cfg, "create_agent", "Create Agent", "Create a new Orchestra AI agent with a system prompt and tools.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":          objProp("string", "Agent name"),
				"description":   objProp("string", "What this agent does"),
				"system_prompt": objProp("string", "The agent's system prompt / instructions"),
				"model":         objProp("string", "Model to use (e.g. claude-sonnet-4-6, gpt-4o)"),
				"is_public":     objProp("boolean", "Whether the agent is publicly visible"),
			},
			"required": []string{"name", "system_prompt"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/agents", token, args)
		},
	)
}

func newGetAgentTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "get_agent",
			Title:       "Get Agent",
			Description: "Get a specific agent by ID.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"id": objProp("string", "Agent ID")},
				"required":   []string{"id"},
			},
			Annotations: &protocol.ToolAnnotations{Title: "Get Agent", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, fmt.Sprintf("/api/agents/%v", args["id"]), token, nil)
		},
	}
}

func newUpdateAgentTool(cfg *config.Config) Tool {
	return userTool(cfg, "update_agent", "Update Agent", "Update an existing agent.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":            objProp("string", "Agent ID"),
				"name":          objProp("string", "New name"),
				"description":   objProp("string", "New description"),
				"system_prompt": objProp("string", "New system prompt"),
				"model":         objProp("string", "New model"),
				"is_public":     objProp("boolean", "Public visibility"),
			},
			"required": []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["id"]
			delete(args, "id")
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPut, fmt.Sprintf("/api/agents/%v", id), token, args)
		},
	)
}

func newDeleteAgentTool(cfg *config.Config) Tool {
	return userTool(cfg, "delete_agent", "Delete Agent", "Delete an agent by ID.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"id": objProp("string", "Agent ID")},
			"required":   []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/agents/%v", args["id"]), token, nil)
		},
	)
}

// ── Workflows ────────────────────────────────────────────────────────────────

func newListWorkflowsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "list_workflows",
			Title:       "List My Workflows",
			Description: "List all your Orchestra workflows.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Annotations: &protocol.ToolAnnotations{Title: "List My Workflows", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/workflows", token, nil)
		},
	}
}

func newCreateWorkflowTool(cfg *config.Config) Tool {
	return userTool(cfg, "create_workflow", "Create Workflow", "Create a new Orchestra workflow.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":        objProp("string", "Workflow name"),
				"description": objProp("string", "What this workflow does"),
				"steps":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object"}, "description": "Workflow steps"},
				"is_public":   objProp("boolean", "Whether the workflow is publicly visible"),
			},
			"required": []string{"name"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/workflows", token, args)
		},
	)
}

func newGetWorkflowTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "get_workflow",
			Title:       "Get Workflow",
			Description: "Get a specific workflow by ID.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"id": objProp("string", "Workflow ID")},
				"required":   []string{"id"},
			},
			Annotations: &protocol.ToolAnnotations{Title: "Get Workflow", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, fmt.Sprintf("/api/workflows/%v", args["id"]), token, nil)
		},
	}
}

func newUpdateWorkflowTool(cfg *config.Config) Tool {
	return userTool(cfg, "update_workflow", "Update Workflow", "Update an existing workflow.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":          objProp("string", "Workflow ID"),
				"name":        objProp("string", "New name"),
				"description": objProp("string", "New description"),
				"steps":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "object"}, "description": "Updated steps"},
				"is_public":   objProp("boolean", "Public visibility"),
			},
			"required": []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["id"]
			delete(args, "id")
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPatch, fmt.Sprintf("/api/workflows/%v", id), token, args)
		},
	)
}

func newDeleteWorkflowTool(cfg *config.Config) Tool {
	return userTool(cfg, "delete_workflow", "Delete Workflow", "Delete a workflow by ID.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"id": objProp("string", "Workflow ID")},
			"required":   []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/workflows/%v", args["id"]), token, nil)
		},
	)
}

// ── Notes ────────────────────────────────────────────────────────────────────

func newListNotesTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "list_notes",
			Title:       "List My Notes",
			Description: "List all your Orchestra notes.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Annotations: &protocol.ToolAnnotations{Title: "List My Notes", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/notes", token, nil)
		},
	}
}

func newCreateNoteTool(cfg *config.Config) Tool {
	return userTool(cfg, "create_note", "Create Note", "Create a new note.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title":     objProp("string", "Note title"),
				"content":   objProp("string", "Note content (markdown supported)"),
				"is_pinned": objProp("boolean", "Pin this note"),
			},
			"required": []string{"title", "content"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/notes", token, args)
		},
	)
}

func newUpdateNoteTool(cfg *config.Config) Tool {
	return userTool(cfg, "update_note", "Update Note", "Update an existing note.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":        objProp("string", "Note ID"),
				"title":     objProp("string", "New title"),
				"content":   objProp("string", "New content"),
				"is_pinned": objProp("boolean", "Pin status"),
			},
			"required": []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["id"]
			delete(args, "id")
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPut, fmt.Sprintf("/api/notes/%v", id), token, args)
		},
	)
}

func newDeleteNoteTool(cfg *config.Config) Tool {
	return userTool(cfg, "delete_note", "Delete Note", "Delete a note by ID.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"id": objProp("string", "Note ID")},
			"required":   []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/notes/%v", args["id"]), token, nil)
		},
	)
}

// ── API Collections ──────────────────────────────────────────────────────────

func newListAPICollectionsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "list_api_collections",
			Title:       "List API Collections",
			Description: "List all your Orchestra API collections.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Annotations: &protocol.ToolAnnotations{Title: "List API Collections", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/api-collections", token, nil)
		},
	}
}

func newCreateAPICollectionTool(cfg *config.Config) Tool {
	return userTool(cfg, "create_api_collection", "Create API Collection", "Create a new API collection for organizing endpoints.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name":        objProp("string", "Collection name"),
				"description": objProp("string", "What this collection is for"),
				"base_url":    objProp("string", "Base URL for all endpoints"),
				"is_public":   objProp("boolean", "Publicly visible"),
			},
			"required": []string{"name"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/api-collections", token, args)
		},
	)
}

func newGetAPICollectionTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "get_api_collection",
			Title:       "Get API Collection",
			Description: "Get a specific API collection by ID, including its endpoints.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"id": objProp("string", "Collection ID")},
				"required":   []string{"id"},
			},
			Annotations: &protocol.ToolAnnotations{Title: "Get API Collection", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, fmt.Sprintf("/api/api-collections/%v", args["id"]), token, nil)
		},
	}
}

func newAddAPIEndpointTool(cfg *config.Config) Tool {
	return userTool(cfg, "add_api_endpoint", "Add API Endpoint", "Add an endpoint to an API collection.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"collection_id": objProp("string", "Collection ID"),
				"method":        map[string]interface{}{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE"}, "description": "HTTP method"},
				"path":          objProp("string", "Endpoint path (e.g. /users/:id)"),
				"name":          objProp("string", "Endpoint name"),
				"description":   objProp("string", "What this endpoint does"),
				"headers":       map[string]interface{}{"type": "object", "description": "Default headers"},
				"body":          map[string]interface{}{"type": "object", "description": "Example request body"},
			},
			"required": []string{"collection_id", "method", "path"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["collection_id"]
			delete(args, "collection_id")
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/api-collections%v/endpoints", id), token, args)
		},
	)
}

func newDeleteAPICollectionTool(cfg *config.Config) Tool {
	return userTool(cfg, "delete_api_collection", "Delete API Collection", "Delete an API collection by ID.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"id": objProp("string", "Collection ID")},
			"required":   []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/api-collections/%v", args["id"]), token, nil)
		},
	)
}

// ── Presentations / Slides ───────────────────────────────────────────────────

func newListPresentationsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "list_presentations",
			Title:       "List My Presentations",
			Description: "List all your Orchestra presentations.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Annotations: &protocol.ToolAnnotations{Title: "List My Presentations", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/presentations", token, nil)
		},
	}
}

func newCreatePresentationTool(cfg *config.Config) Tool {
	return userTool(cfg, "create_presentation", "Create Presentation", "Create a new presentation with slides.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title":       objProp("string", "Presentation title"),
				"description": objProp("string", "Brief description"),
				"is_public":   objProp("boolean", "Publicly visible"),
			},
			"required": []string{"title"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/presentations", token, args)
		},
	)
}

func newAddSlideTool(cfg *config.Config) Tool {
	return userTool(cfg, "add_slide", "Add Slide", "Add a slide to a presentation.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"presentation_id": objProp("string", "Presentation ID"),
				"title":           objProp("string", "Slide title"),
				"content":         objProp("string", "Slide content (markdown)"),
				"layout":          objProp("string", "Layout type (e.g. title, content, two-column)"),
			},
			"required": []string{"presentation_id", "content"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["presentation_id"]
			delete(args, "presentation_id")
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, fmt.Sprintf("/api/presentations%v/slides", id), token, args)
		},
	)
}

func newDeletePresentationTool(cfg *config.Config) Tool {
	return userTool(cfg, "delete_presentation", "Delete Presentation", "Delete a presentation by ID.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"id": objProp("string", "Presentation ID")},
			"required":   []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/presentations/%v", args["id"]), token, nil)
		},
	)
}

// ── Community Posts ──────────────────────────────────────────────────────────

func newListCommunityPostsTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "list_my_community_posts",
			Title:       "List My Community Posts",
			Description: "List all your community posts.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Annotations: &protocol.ToolAnnotations{Title: "List My Community Posts", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/community/posts", token, nil)
		},
	}
}

func newCreateCommunityPostTool(cfg *config.Config) Tool {
	return userTool(cfg, "create_community_post", "Create Community Post", "Create a new community post.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title":       objProp("string", "Post title"),
				"content":     objProp("string", "Post content (markdown supported)"),
				"tags":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Tags"},
				"is_question": objProp("boolean", "Whether this is a question post"),
			},
			"required": []string{"title", "content"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/community/posts", token, args)
		},
	)
}

func newUpdateCommunityPostTool(cfg *config.Config) Tool {
	return userTool(cfg, "update_community_post", "Update Community Post", "Update one of your community posts.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":      objProp("string", "Post ID"),
				"title":   objProp("string", "New title"),
				"content": objProp("string", "New content"),
				"tags":    map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Tags"},
			},
			"required": []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			id := args["id"]
			delete(args, "id")
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPut, fmt.Sprintf("/api/community/posts/%v", id), token, args)
		},
	)
}

func newDeleteCommunityPostTool(cfg *config.Config) Tool {
	return userTool(cfg, "delete_community_post", "Delete Community Post", "Delete one of your community posts.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"id": objProp("string", "Post ID")},
			"required":   []string{"id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodDelete, fmt.Sprintf("/api/community/posts/%v", args["id"]), token, nil)
		},
	)
}

// ── Shares (public content sharing) ─────────────────────────────────────────

func newListSharesTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission:   permissions.PermContent,
		VisibleToAll: true,
		Definition: protocol.ToolDefinition{
			Name:        "list_shares",
			Title:       "List My Shared Content",
			Description: "List all content you've shared publicly.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			Annotations: &protocol.ToolAnnotations{Title: "List My Shared Content", ReadOnlyHint: &readOnly},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			token, err := getUserToken(userID)
			if err != nil {
				return errResult(err.Error()), nil
			}
			return userFetchText(cfg.WebAPIBaseURL, http.MethodGet, "/api/community/shares", token, nil)
		},
	}
}

func newCreateShareTool(cfg *config.Config) Tool {
	return userTool(cfg, "create_share", "Share Content", "Share a skill, agent, workflow, or collection publicly on the community.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"entity_type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"skill", "agent", "workflow", "api_collection", "presentation", "note"},
					"description": "Type of content to share",
				},
				"entity_id":   objProp("string", "ID of the content to share"),
				"title":       objProp("string", "Share title (defaults to content name)"),
				"description": objProp("string", "Share description"),
				"tags":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Tags"},
			},
			"required": []string{"entity_type", "entity_id"},
		},
		func(args map[string]interface{}, token string) (protocol.ToolResult, error) {
			return userFetchText(cfg.WebAPIBaseURL, http.MethodPost, "/api/community/shares", token, args)
		},
	)
}

// ── helper ───────────────────────────────────────────────────────────────────

func objProp(typ, desc string) map[string]interface{} {
	return map[string]interface{}{"type": typ, "description": desc}
}
