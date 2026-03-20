// Package protocol contains MCP 2025-11-25 type definitions for the cloud-mcp service.
package protocol

// ProtocolVersion is the MCP protocol version implemented by this server.
const ProtocolVersion = "2025-11-25"

// Standard JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      interface{}    `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServerInfo identifies the cloud MCP server (MCP 2025-11-25).
type ServerInfo struct {
	Name        string  `json:"name"`
	Version     string  `json:"version"`
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Icons       []Icon  `json:"icons,omitempty"`
	WebsiteURL  string  `json:"websiteUrl,omitempty"`
}

// Icon represents an icon (MCP 2025-11-25).
type Icon struct {
	Src      string   `json:"src"`
	MimeType string   `json:"mimeType,omitempty"`
	Sizes    []string `json:"sizes,omitempty"`
}

// ToolAnnotations provides behavior hints (MCP 2025-11-25).
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// ToolDefinition describes a tool (MCP 2025-11-25).
type ToolDefinition struct {
	Name         string           `json:"name"`
	Title        string           `json:"title,omitempty"`
	Description  string           `json:"description,omitempty"`
	Icons        []Icon           `json:"icons,omitempty"`
	InputSchema  interface{}      `json:"inputSchema"`
	OutputSchema interface{}      `json:"outputSchema,omitempty"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
}

// Content is a content block in a tool result.
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// ToolResult is the tool call response payload.
type ToolResult struct {
	Content []Content      `json:"content"`
	IsError bool           `json:"isError,omitempty"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

// ServerCapabilities describes what the server supports.
type ServerCapabilities struct {
	Tools   *ToolsCapability `json:"tools,omitempty"`
	Logging *struct{}        `json:"logging,omitempty"`
}

// ToolsCapability signals tool list-change notifications.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// InitializeResult is the initialize response.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	SessionID       string             `json:"_sessionId,omitempty"`
}

// TextResult builds a plain-text ToolResult.
func TextResult(text string) ToolResult {
	return ToolResult{
		Content: []Content{{Type: "text", Text: text}},
	}
}

// ErrorResult builds an error ToolResult.
func ErrorResult(text string) ToolResult {
	return ToolResult{
		Content: []Content{{Type: "text", Text: text}},
		IsError: true,
	}
}

// BoolPtr is a helper to get a *bool.
func BoolPtr(v bool) *bool { return &v }
