package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/orchestra-mcp/cloud-mcp/internal/config"
	"gorm.io/gorm"
)

// UserMCPPermission stores per-user MCP permission toggles.
// Shared DB table with apps/web.
type UserMCPPermission struct {
	UserID     uint      `gorm:"primaryKey;column:user_id"`
	Permission string    `gorm:"primaryKey;column:permission;size:64"`
	Enabled    bool      `gorm:"default:true;column:enabled"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (UserMCPPermission) TableName() string { return "user_mcp_permissions" }

// User is a minimal projection of the users table for auth lookups.
type User struct {
	ID       uint           `gorm:"primarykey"`
	Email    string         `gorm:"uniqueIndex"`
	Name     string
	Role     string
	Status   string
	Settings datatypesJSON  `gorm:"type:jsonb"`
}

func (User) TableName() string { return "users" }

// datatypesJSON is a []byte that marshals/unmarshals as JSON.
type datatypesJSON []byte

// Claims extends jwt.RegisteredClaims with user identity.
type Claims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// ValidateToken parses and validates a Bearer token (JWT or orch_* API key).
// Returns the user ID on success.
func ValidateToken(tokenStr string, cfg *config.Config, db *gorm.DB) (uint, error) {
	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
	tokenStr = strings.TrimPrefix(tokenStr, "bearer ")
	tokenStr = strings.TrimSpace(tokenStr)

	if strings.HasPrefix(tokenStr, "orch_") {
		return validateAPIKey(tokenStr, db)
	}
	return validateJWT(tokenStr, cfg)
}

func validateJWT(tokenStr string, cfg *config.Config) (uint, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return 0, jwt.ErrSignatureInvalid
	}
	return claims.UserID, nil
}

func validateAPIKey(token string, db *gorm.DB) (uint, error) {
	h := sha256.Sum256([]byte(token))
	keyHash := hex.EncodeToString(h[:])

	var users []User
	if err := db.Where("settings::text LIKE ?", "%"+keyHash+"%").Find(&users).Error; err != nil {
		return 0, err
	}

	for _, u := range users {
		if u.Status == "blocked" {
			continue
		}
		var meta map[string]interface{}
		if json.Unmarshal([]byte(u.Settings), &meta) != nil {
			continue
		}
		raw, ok := meta["api_keys"]
		if !ok {
			continue
		}
		b, _ := json.Marshal(raw)
		var keys []struct {
			Hash string `json:"hash"`
		}
		if json.Unmarshal(b, &keys) != nil {
			continue
		}
		for _, k := range keys {
			if k.Hash == keyHash {
				return u.ID, nil
			}
		}
	}
	return 0, jwt.ErrSignatureInvalid
}

// GetUser loads a user by ID for profile tools.
func GetUser(userID uint, db *gorm.DB) (*User, error) {
	var u User
	if err := db.First(&u, userID).Error; err != nil {
		return nil, err
	}
	return &u, nil
}
