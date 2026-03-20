package permissions

import (
	"sync"
	"time"

	"github.com/orchestra-mcp/cloud-mcp/internal/auth"
	"gorm.io/gorm"
)

// Known permission keys.
const (
	PermInstall      = "mcp.install"
	PermStatusRead   = "mcp.status"
	PermProfileRead  = "mcp.profile.read"
	PermProfileWrite = "mcp.profile.write"
	PermMarketplace  = "mcp.marketplace"
	PermAdmin        = "mcp.admin" // granted only to users with role=admin in the DB
)

// defaults maps each permission to its default enabled state.
var defaults = map[string]bool{
	PermInstall:      true,
	PermStatusRead:   true,
	PermProfileRead:  true,
	PermProfileWrite: false,
	PermMarketplace:  true,
}

// cachedPerms is a simple in-process cache entry.
type cachedPerms struct {
	perms     map[string]bool
	expiresAt time.Time
}

// Checker loads and caches user MCP permission toggles.
type Checker struct {
	db    *gorm.DB
	mu    sync.Mutex
	cache map[uint]*cachedPerms
	ttl   time.Duration
}

// NewChecker creates a permission checker with a 30-second cache TTL.
func NewChecker(db *gorm.DB) *Checker {
	return &Checker{
		db:    db,
		cache: make(map[uint]*cachedPerms),
		ttl:   30 * time.Second,
	}
}

// Can returns whether the user has the given permission enabled.
// Unauthenticated calls (userID=0) only allow public permissions.
func (c *Checker) Can(userID uint, permission string) bool {
	if userID == 0 {
		// Public — only allow public-facing permissions.
		return permission == PermInstall || permission == PermStatusRead
	}

	perms := c.load(userID)
	enabled, ok := perms[permission]
	if !ok {
		// Fall back to default.
		return defaults[permission]
	}
	return enabled
}

// load returns the permission map for a user, using the cache when fresh.
func (c *Checker) load(userID uint) map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.cache[userID]; ok && time.Now().Before(entry.expiresAt) {
		return entry.perms
	}

	perms := make(map[string]bool)

	// Copy defaults first.
	for k, v := range defaults {
		perms[k] = v
	}

	// Load overrides from DB.
	var rows []auth.UserMCPPermission
	c.db.Where("user_id = ?", userID).Find(&rows)
	for _, r := range rows {
		perms[r.Permission] = r.Enabled
	}

	// Grant mcp.admin if the user's role is admin.
	var u auth.User
	if err := c.db.Select("role").First(&u, userID).Error; err == nil {
		perms[PermAdmin] = u.Role == "admin"
	}

	c.cache[userID] = &cachedPerms{
		perms:     perms,
		expiresAt: time.Now().Add(c.ttl),
	}
	return perms
}

// AllForUser returns all permission states for a user (for the settings UI).
func (c *Checker) AllForUser(userID uint) map[string]bool {
	return c.load(userID)
}

// Set updates a permission toggle for a user and invalidates the cache.
func (c *Checker) Set(userID uint, permission string, enabled bool) error {
	perm := auth.UserMCPPermission{
		UserID:     userID,
		Permission: permission,
		Enabled:    enabled,
		UpdatedAt:  time.Now(),
	}
	result := c.db.Save(&perm)
	if result.Error != nil {
		return result.Error
	}

	// Invalidate cache.
	c.mu.Lock()
	delete(c.cache, userID)
	c.mu.Unlock()

	return nil
}
