package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds the cloud-mcp service configuration.
type Config struct {
	// Server.
	Port int

	// Database (shared with apps/web — read-only for permissions/user lookups).
	DSN string

	// Auth — same JWT secret as apps/web so tokens are cross-service compatible.
	JWTSecret string

	// CORS.
	AllowedOrigins []string

	// Web app base URL for API calls (get_profile / update_profile).
	WebAPIBaseURL string

	// Rate limiting.
	PublicRateLimit int // requests per minute per IP
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	port := 8091
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/orchestra_web?sslmode=disable"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "orchestra-secret-change-in-production"
	}

	var allowedOrigins []string
	if raw := os.Getenv("ALLOWED_ORIGINS"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	} else {
		allowedOrigins = []string{
			"https://orchestra-mcp.com",
			"https://www.orchestra-mcp.com",
			"https://app.orchestra-mcp.com",
			"http://localhost:3000",
			"http://localhost:5173",
		}
	}

	webAPI := os.Getenv("WEB_API_BASE_URL")
	if webAPI == "" {
		webAPI = "https://orchestra-mcp.com/api"
	}

	rateLimit := 10
	if r := os.Getenv("PUBLIC_RATE_LIMIT"); r != "" {
		if n, err := strconv.Atoi(r); err == nil {
			rateLimit = n
		}
	}

	return &Config{
		Port:            port,
		DSN:             dsn,
		JWTSecret:       jwtSecret,
		AllowedOrigins:  allowedOrigins,
		WebAPIBaseURL:   webAPI,
		PublicRateLimit: rateLimit,
	}
}
