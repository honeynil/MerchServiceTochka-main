package models

import "time"

type TokenClaims struct {
	UserID    int64                  `json:"user_id"`
	ExpiresAt time.Time              `json:"exp"`
	IssuedAt  time.Time              `json:"iat"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Для refresh-токена можно добавить отдельную структуру,
// если у них разные claim-ы
type RefreshTokenClaims struct {
	UserID    int64     `json:"user_id"`
	TokenID   string    `json:"jti"` // Уникальный идентификатор токена
	ExpiresAt time.Time `json:"exp"`
}
