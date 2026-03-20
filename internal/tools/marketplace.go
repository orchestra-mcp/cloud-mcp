package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"github.com/orchestra-mcp/cloud-mcp/internal/protocol"
	"github.com/orchestra-mcp/cloud-mcp/internal/permissions"
)

// Marketplace tools allow users to browse and install Orchestra packs, plugins,
// skills, agents, and workflows directly from their AI agent.

// newListPacksTool lists available packs from the Orchestra marketplace.
// Requires mcp.marketplace permission.
func newListPacksTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermMarketplace,
		Definition: protocol.ToolDefinition{
			Name:  "list_packs",
			Title: "Browse Marketplace",
			Description: "List available Orchestra packs from the marketplace. Packs bundle skills, agents, hooks, " +
				"and workflows for specific tech stacks (Go, React, Swift, Flutter, Docker, etc.). " +
				"Returns a formatted list you can browse and install from.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"category": map[string]interface{}{
						"type":        "string",
						"description": "Filter by category: all, language, framework, platform, workflow (default: all)",
						"enum":        []string{"all", "language", "framework", "platform", "workflow"},
						"default":     "all",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of packs to return (default: 20, max: 50)",
						"default":     20,
					},
				},
			},
			Annotations: &protocol.ToolAnnotations{
				Title:        "Browse Marketplace",
				ReadOnlyHint: &readOnly,
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			category := "all"
			if v, ok := args["category"].(string); ok && v != "" {
				category = v
			}
			limit := 20
			if v, ok := args["limit"].(float64); ok && v > 0 {
				limit = int(v)
				if limit > 50 {
					limit = 50
				}
			}

			packs, err := fetchPacks(cfg.WebAPIBaseURL, category, limit)
			if err != nil {
				// Fallback to static well-known packs if API is unreachable.
				return staticPackList(category), nil
			}

			return formatPackList(packs), nil
		},
	}
}

// newSearchPacksTool searches the Orchestra marketplace.
func newSearchPacksTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermMarketplace,
		Definition: protocol.ToolDefinition{
			Name:  "search_packs",
			Title: "Search Marketplace",
			Description: "Search the Orchestra marketplace for packs, skills, agents, hooks, or workflows by keyword. " +
				"Use this to find packs for specific tech stacks, frameworks, or use cases.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query (e.g. 'react', 'docker', 'swift', 'testing')",
					},
				},
				"required": []string{"query"},
			},
			Annotations: &protocol.ToolAnnotations{
				Title:        "Search Marketplace",
				ReadOnlyHint: &readOnly,
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			query, _ := args["query"].(string)
			if query == "" {
				return protocol.ToolResult{
					Content: []protocol.Content{{Type: "text", Text: "Please provide a search query."}},
					IsError: true,
				}, nil
			}

			packs, err := searchPacks(cfg.WebAPIBaseURL, query)
			if err != nil {
				return staticSearchFallback(query), nil
			}

			if len(packs) == 0 {
				return protocol.ToolResult{
					Content: []protocol.Content{{
						Type: "text",
						Text: fmt.Sprintf("No packs found for %q. Try `list_packs` to browse all available packs.", query),
					}},
				}, nil
			}

			return formatPackList(packs), nil
		},
	}
}

// newGetPackTool returns detailed information about a specific pack.
func newGetPackTool(cfg *config.Config) Tool {
	readOnly := true
	return Tool{
		Permission: permissions.PermMarketplace,
		Definition: protocol.ToolDefinition{
			Name:  "get_pack",
			Title: "Get Pack Details",
			Description: "Get detailed information about an Orchestra pack including its skills, agents, hooks, " +
				"workflows, and installation requirements.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pack_id": map[string]interface{}{
						"type":        "string",
						"description": "Pack identifier (e.g. 'orchestra-mcp/pack-react', 'orchestra-mcp/pack-docker')",
					},
				},
				"required": []string{"pack_id"},
			},
			Annotations: &protocol.ToolAnnotations{
				Title:        "Get Pack Details",
				ReadOnlyHint: &readOnly,
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			packID, _ := args["pack_id"].(string)
			if packID == "" {
				return protocol.ToolResult{
					Content: []protocol.Content{{Type: "text", Text: "Please provide a pack_id."}},
					IsError: true,
				}, nil
			}

			pack, err := fetchPack(cfg.WebAPIBaseURL, packID)
			if err != nil {
				return protocol.ToolResult{
					Content: []protocol.Content{{
						Type: "text",
						Text: fmt.Sprintf("Pack %q not found or unavailable. Use `list_packs` to browse available packs.", packID),
					}},
					IsError: true,
				}, nil
			}

			return formatPackDetail(pack), nil
		},
	}
}

// newInstallPackTool generates the shell command to install a pack.
// Requires marketplace permission. Returns a shell script for Claude to run locally.
func newInstallPackTool(cfg *config.Config) Tool {
	return Tool{
		Permission: permissions.PermMarketplace,
		Definition: protocol.ToolDefinition{
			Name:  "install_pack",
			Title: "Install Pack",
			Description: "Generate the shell command to install an Orchestra pack on the user's machine. " +
				"Packs add skills (slash commands), agents, hooks, and workflows to your AI workflow. " +
				"Returns a shell command for Claude to run locally (requires Orchestra to be installed).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"pack_id": map[string]interface{}{
						"type":        "string",
						"description": "Pack identifier (e.g. 'orchestra-mcp/pack-react', 'orchestra-mcp/pack-docker')",
					},
					"workspace": map[string]interface{}{
						"type":        "string",
						"description": "Project directory where the pack should be installed (default: current directory)",
						"default":     ".",
					},
				},
				"required": []string{"pack_id"},
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			packID, _ := args["pack_id"].(string)
			if packID == "" {
				return protocol.ToolResult{
					Content: []protocol.Content{{Type: "text", Text: "Please provide a pack_id."}},
					IsError: true,
				}, nil
			}

			workspace := "."
			if v, ok := args["workspace"].(string); ok && v != "" {
				workspace = v
			}

			// Build the install command.
			var text strings.Builder
			fmt.Fprintf(&text, "Run this command to install the **%s** pack:\n\n", packID)
			text.WriteString("```bash\n")
			if workspace == "." {
				fmt.Fprintf(&text, "orchestra pack install %s\n", packID)
			} else {
				fmt.Fprintf(&text, "orchestra pack install %s --workspace %q\n", packID, workspace)
			}
			text.WriteString("```\n\n")
			text.WriteString("After installation:\n")
			text.WriteString("- New skills will be available as slash commands (e.g. `/react`, `/docker`)\n")
			text.WriteString("- New agents will auto-delegate based on task context\n")
			text.WriteString("- Restart your IDE to reload the MCP configuration\n\n")
			text.WriteString("💡 If Orchestra is not installed yet, call `install_orchestra` first.")

			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: text.String()}},
			}, nil
		},
	}
}

// --- API helpers ---

type packEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Category    string   `json:"category"`
	Skills      []string `json:"skills"`
	Agents      []string `json:"agents"`
	Downloads   int      `json:"downloads"`
}

func fetchPacks(baseURL, category string, limit int) ([]packEntry, error) {
	url := fmt.Sprintf("%s/api/marketplace/packs?category=%s&limit=%d", baseURL, category, limit)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Packs []packEntry `json:"packs"`
	}
	json.Unmarshal(body, &result)
	return result.Packs, nil
}

func searchPacks(baseURL, query string) ([]packEntry, error) {
	url := fmt.Sprintf("%s/api/marketplace/packs/search?q=%s", baseURL, query)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Packs []packEntry `json:"packs"`
	}
	json.Unmarshal(body, &result)
	return result.Packs, nil
}

func fetchPack(baseURL, packID string) (*packEntry, error) {
	url := fmt.Sprintf("%s/api/marketplace/packs/%s", baseURL, packID)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var p packEntry
	json.Unmarshal(body, &p)
	return &p, nil
}

func formatPackList(packs []packEntry) protocol.ToolResult {
	if len(packs) == 0 {
		return protocol.ToolResult{
			Content: []protocol.Content{{Type: "text", Text: "No packs found."}},
		}
	}

	var sb strings.Builder
	sb.WriteString("## Orchestra Marketplace Packs\n\n")
	for _, p := range packs {
		fmt.Fprintf(&sb, "### %s `%s`\n", p.Name, p.ID)
		fmt.Fprintf(&sb, "%s\n\n", p.Description)
		if len(p.Skills) > 0 {
			fmt.Fprintf(&sb, "**Skills:** %s\n", strings.Join(p.Skills, ", "))
		}
		if len(p.Agents) > 0 {
			fmt.Fprintf(&sb, "**Agents:** %s\n", strings.Join(p.Agents, ", "))
		}
		fmt.Fprintf(&sb, "**Version:** %s", p.Version)
		if p.Downloads > 0 {
			fmt.Fprintf(&sb, " · **Downloads:** %d", p.Downloads)
		}
		sb.WriteString("\n\n---\n\n")
	}
	sb.WriteString("To install a pack, call `install_pack` with the pack ID.")

	return protocol.ToolResult{
		Content: []protocol.Content{{Type: "text", Text: sb.String()}},
	}
}

func formatPackDetail(p *packEntry) protocol.ToolResult {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s\n\n", p.Name)
	fmt.Fprintf(&sb, "**ID:** `%s`  \n", p.ID)
	fmt.Fprintf(&sb, "**Version:** %s  \n", p.Version)
	if p.Downloads > 0 {
		fmt.Fprintf(&sb, "**Downloads:** %d  \n", p.Downloads)
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "%s\n\n", p.Description)

	if len(p.Skills) > 0 {
		sb.WriteString("### Skills (slash commands)\n")
		for _, s := range p.Skills {
			fmt.Fprintf(&sb, "- `/%s`\n", s)
		}
		sb.WriteString("\n")
	}
	if len(p.Agents) > 0 {
		sb.WriteString("### Agents\n")
		for _, a := range p.Agents {
			fmt.Fprintf(&sb, "- %s\n", a)
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "To install: `orchestra pack install %s`\n", p.ID)
	sb.WriteString("Or call the `install_pack` tool to get the exact command.")

	return protocol.ToolResult{
		Content: []protocol.Content{{Type: "text", Text: sb.String()}},
	}
}

// staticPackList is a fallback when the API is unreachable.
func staticPackList(category string) protocol.ToolResult {
	// Well-known official packs — always available even without API.
	knownPacks := []packEntry{
		{ID: "orchestra-mcp/pack-essentials", Name: "Essentials", Description: "Core skills and agents for any project. Includes docs, QA testing, plugin generator, and project manager.", Version: "v0.1.0", Skills: []string{"docs", "qa-testing", "plugin-generator", "project-manager"}, Category: "workflow"},
		{ID: "orchestra-mcp/pack-go", Name: "Go Development", Description: "Skills and agents for Go projects: proto/gRPC, database patterns, testing.", Version: "v0.1.0", Skills: []string{"proto-grpc", "database-sync"}, Category: "language"},
		{ID: "orchestra-mcp/pack-react", Name: "React + TypeScript", Description: "TypeScript/React skills, TailwindCSS, UI design, and frontend development agents.", Version: "v0.1.0", Skills: []string{"typescript-react", "tailwindcss-development", "ui-design"}, Category: "framework"},
		{ID: "orchestra-mcp/pack-flutter", Name: "Flutter", Description: "Flutter development with platform-specific agents for iOS, Android, macOS, Windows, Linux, and Web.", Version: "v0.1.0", Skills: []string{"react-native-mobile"}, Agents: []string{"flutter-ios", "flutter-android", "flutter-macos"}, Category: "framework"},
		{ID: "orchestra-mcp/pack-swift", Name: "Swift / macOS", Description: "macOS integration, native extensions, widgets, and Raycast/VSCode compatibility.", Version: "v0.1.0", Skills: []string{"macos-integration", "native-extensions", "native-widgets"}, Category: "platform"},
		{ID: "orchestra-mcp/pack-devops", Name: "DevOps", Description: "Docker, GCP infrastructure, CI/CD, and deployment skills.", Version: "v0.1.0", Skills: []string{"gcp-infrastructure"}, Category: "platform"},
		{ID: "orchestra-mcp/pack-ai", Name: "AI Engineering", Description: "AI agent patterns, agentic workflows, Wails desktop apps, and AI integration skills.", Version: "v0.1.0", Skills: []string{"ai-agentic", "wails-desktop"}, Agents: []string{"ai-engineer"}, Category: "workflow"},
		{ID: "orchestra-mcp/pack-flow", Name: "Flow", Description: "Complete flow management system: intake, spec, review, coach, contract, health, tempo, and more.", Version: "v0.1.0", Skills: []string{"flow", "flow-intake", "flow-spec", "flow-review", "flow-coach"}, Category: "workflow"},
	}

	filtered := knownPacks
	if category != "all" && category != "" {
		filtered = []packEntry{}
		for _, p := range knownPacks {
			if p.Category == category {
				filtered = append(filtered, p)
			}
		}
	}

	return formatPackList(filtered)
}

// staticSearchFallback returns a helpful message when the API is unreachable.
func staticSearchFallback(query string) protocol.ToolResult {
	q := strings.ToLower(query)
	var matches []packEntry

	allPacks := []packEntry{
		{ID: "orchestra-mcp/pack-essentials", Name: "Essentials", Description: "Core skills for any project.", Version: "v0.1.0"},
		{ID: "orchestra-mcp/pack-go", Name: "Go Development", Description: "Go, proto/gRPC, database patterns.", Version: "v0.1.0"},
		{ID: "orchestra-mcp/pack-react", Name: "React + TypeScript", Description: "React, TypeScript, TailwindCSS, UI design.", Version: "v0.1.0"},
		{ID: "orchestra-mcp/pack-flutter", Name: "Flutter", Description: "Flutter cross-platform development.", Version: "v0.1.0"},
		{ID: "orchestra-mcp/pack-swift", Name: "Swift / macOS", Description: "macOS, iOS, native extensions, widgets.", Version: "v0.1.0"},
		{ID: "orchestra-mcp/pack-devops", Name: "DevOps", Description: "Docker, GCP, CI/CD, deployment.", Version: "v0.1.0"},
		{ID: "orchestra-mcp/pack-ai", Name: "AI Engineering", Description: "AI agents, agentic workflows.", Version: "v0.1.0"},
		{ID: "orchestra-mcp/pack-flow", Name: "Flow", Description: "Flow management: intake, spec, review, coach.", Version: "v0.1.0"},
	}

	for _, p := range allPacks {
		if strings.Contains(strings.ToLower(p.ID), q) ||
			strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Description), q) {
			matches = append(matches, p)
		}
	}

	if len(matches) == 0 {
		return protocol.ToolResult{
			Content: []protocol.Content{{
				Type: "text",
				Text: fmt.Sprintf("No packs found matching %q. Available packs: essentials, go, react, flutter, swift, devops, ai, flow.\nCall `list_packs` to see all available packs.", query),
			}},
		}
	}

	return formatPackList(matches)
}
