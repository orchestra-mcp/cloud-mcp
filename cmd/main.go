package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	fiberlog "github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/orchestra-mcp/cloud-mcp/internal/auth"
	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"github.com/orchestra-mcp/cloud-mcp/internal/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	cfg := config.Load()

	// Connect to database.
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}

	// Run migrations — creates user_mcp_permissions table if it doesn't exist.
	if err := db.AutoMigrate(&auth.UserMCPPermission{}); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// Build Fiber app.
	app := fiber.New(fiber.Config{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		AppName:      "Orchestra Cloud MCP",
	})

	// CORS — allow Claude Desktop, orchestra-mcp.dev, and localhost.
	app.Use(cors.New(cors.Config{
		AllowOrigins:  cfg.AllowedOrigins,
		AllowMethods:  []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:  []string{"Content-Type", "Authorization", "Mcp-Session-Id"},
		ExposeHeaders: []string{"Mcp-Session-Id"},
	}))

	// Request logger.
	app.Use(fiberlog.New(fiberlog.Config{
		Format: "[${time}] ${status} ${method} ${path} (${latency})\n",
	}))

	// Build MCP handler (contains tool registry + session store).
	handler := mcp.NewHandler(db, cfg)

	// Routes.
	app.Get("/health", handleHealth)

	// MCP Streamable HTTP transport (MCP 2025-11-25).
	// Auth is handled per-tool inside the handler.
	app.Post("/mcp", handler.HandlePost)
	app.Get("/mcp", handler.HandleGet) // SSE stream

	// Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Port)
		log.Printf("Orchestra Cloud MCP listening on %s (protocol 2025-11-25)", addr)
		if err := app.Listen(addr); err != nil {
			log.Printf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down...")
	if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func handleHealth(c fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":   "ok",
		"service":  "orchestra-cloud-mcp",
		"version":  "1.0.0",
		"protocol": "2025-11-25",
	})
}
