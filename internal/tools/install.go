package tools

import (
	"fmt"
	"strings"

	"github.com/orchestra-mcp/cloud-mcp/internal/protocol"
)

// newInstallOrchestralTool returns the install_orchestra tool.
// PUBLIC — no authentication required.
func newInstallOrchestralTool() Tool {
	return Tool{
		Permission: "", // public
		Definition: protocol.ToolDefinition{
			Name:  "install_orchestra",
			Title: "Install Orchestra MCP",
			Description: "Generate the shell commands to install Orchestra MCP on the user's machine and initialize it for their IDE. " +
				"Returns shell commands for Claude to run locally.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"ide": map[string]interface{}{
						"type":        "string",
						"description": "Target IDE: claude (default), cursor, vscode, windsurf, codex, gemini, zed, continue, cline, all",
						"enum":        []string{"claude", "cursor", "vscode", "windsurf", "codex", "gemini", "zed", "continue", "cline", "all"},
						"default":     "claude",
					},
					"workspace": map[string]interface{}{
						"type":        "string",
						"description": "Project directory to initialize (default: current directory)",
						"default":     ".",
					},
					"install_desktop": map[string]interface{}{
						"type":        "boolean",
						"description": "Also install the Orchestra desktop app",
						"default":     false,
					},
				},
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			ide := "claude"
			if v, ok := args["ide"].(string); ok && v != "" {
				ide = v
			}
			workspace := "."
			if v, ok := args["workspace"].(string); ok && v != "" {
				workspace = v
			}
			installDesktop, _ := args["install_desktop"].(bool)

			var sb strings.Builder
			sb.WriteString("Run these commands to install and configure Orchestra MCP:\n\n```bash\n")

			// Install Orchestra CLI.
			sb.WriteString("# Install Orchestra MCP\n")
			sb.WriteString("curl -fsSL https://orchestra-mcp.dev/install.sh | sh\n\n")

			// Initialize for target IDE.
			sb.WriteString("# Initialize Orchestra for your project\n")
			if ide == "all" {
				fmt.Fprintf(&sb, "orchestra init --workspace %q --all\n", workspace)
			} else {
				fmt.Fprintf(&sb, "orchestra init --workspace %q --ide %s\n", workspace, ide)
			}

			if installDesktop {
				sb.WriteString("\n# Download and install the Orchestra desktop app\n")
				sb.WriteString("# (The install_desktop_app tool can provide platform-specific commands)\n")
			}

			sb.WriteString("```\n\n")
			sb.WriteString("After installation:\n")
			sb.WriteString("1. **Restart your IDE** to load the new MCP configuration\n")
			sb.WriteString("2. Orchestra MCP will auto-start when your IDE loads\n")
			sb.WriteString("3. Run `/orchestra` to begin the onboarding flow\n")

			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: sb.String()}},
			}, nil
		},
	}
}

// newInstallDesktopTool returns the install_desktop_app tool.
// PUBLIC — no authentication required.
func newInstallDesktopTool() Tool {
	return Tool{
		Permission: "", // public
		Definition: protocol.ToolDefinition{
			Name:  "install_desktop_app",
			Title: "Install Orchestra Desktop App",
			Description: "Generate platform-specific commands to download and install the Orchestra desktop app. " +
				"Returns shell commands for Claude to run locally.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"platform": map[string]interface{}{
						"type":        "string",
						"description": "Target platform: macos, windows, or linux",
						"enum":        []string{"macos", "windows", "linux"},
					},
				},
				"required": []string{"platform"},
			},
		},
		Handler: func(args map[string]interface{}, userID uint) (protocol.ToolResult, error) {
			platform, _ := args["platform"].(string)
			baseURL := "https://github.com/orchestra-mcp/desktop/releases/latest/download"
			fence := "```"

			var text string
			switch platform {
			case "macos":
				text = "Install Orchestra Desktop on macOS:\n\n" +
					"**Option 1 — Direct download:**\n" +
					fence + "bash\n" +
					fmt.Sprintf("curl -Lo ~/Downloads/Orchestra.dmg %s/Orchestra.dmg\n", baseURL) +
					"open ~/Downloads/Orchestra.dmg\n" +
					fence + "\n\n" +
					"**Option 2 — Homebrew:**\n" +
					fence + "bash\n" +
					"brew install --cask orchestra\n" +
					fence + "\n\n" +
					"After installing, launch Orchestra from your Applications folder.\n" +
					"Orchestra will appear in your menu bar as a tray icon."

			case "windows":
				text = "Install Orchestra Desktop on Windows:\n\n" +
					"**Option 1 — Direct download:**\n" +
					fence + "powershell\n" +
					fmt.Sprintf("Invoke-WebRequest -Uri \"%s/OrchestraSetup.exe\" -OutFile \"$env:TEMP\\OrchestraSetup.exe\"\n", baseURL) +
					"Start-Process \"$env:TEMP\\OrchestraSetup.exe\" -Wait\n" +
					fence + "\n\n" +
					"**Option 2 — winget:**\n" +
					fence + "powershell\n" +
					"winget install OrchestraMCP.Orchestra\n" +
					fence + "\n\n" +
					"After installing, launch Orchestra from your Start Menu or system tray."

			case "linux":
				text = "Install Orchestra Desktop on Linux:\n\n" +
					"**AppImage (any distro):**\n" +
					fence + "bash\n" +
					fmt.Sprintf("curl -Lo ~/Downloads/Orchestra.AppImage %s/Orchestra.AppImage\n", baseURL) +
					"chmod +x ~/Downloads/Orchestra.AppImage\n" +
					"~/Downloads/Orchestra.AppImage\n" +
					fence + "\n\n" +
					"**Flatpak:**\n" +
					fence + "bash\n" +
					"flatpak install flathub dev.orchestra_mcp.Orchestra\n" +
					"flatpak run dev.orchestra_mcp.Orchestra\n" +
					fence + "\n\n" +
					"**Debian/Ubuntu (.deb):**\n" +
					fence + "bash\n" +
					fmt.Sprintf("curl -Lo /tmp/orchestra.deb %s/orchestra_amd64.deb\n", baseURL) +
					"sudo dpkg -i /tmp/orchestra.deb\n" +
					fence + "\n\n" +
					"Orchestra will appear in your system tray after launch."

			default:
				return protocol.ToolResult{
					Content: []protocol.Content{{Type: "text", Text: "Unknown platform. Use: macos, windows, or linux"}},
					IsError: true,
				}, nil
			}

			return protocol.ToolResult{
				Content: []protocol.Content{{Type: "text", Text: text}},
			}, nil
		},
	}
}
