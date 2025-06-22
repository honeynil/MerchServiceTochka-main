package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/honeynil/MerchServiceTochka-main/internal/infrastructure/redis"
)

func AuthMiddleware(redisClient redis.RedisClient, jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "authorization header missing", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "invalid authorization header", http.StatusUnauthorized)
				return
			}

			tokenStr := parts[1]
			token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
				}
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "invalid token claims", http.StatusUnauthorized)
				return
			}

			userID, ok := claims["user_id"].(float64)
			if !ok {
				http.Error(w, "invalid user_id in token", http.StatusUnauthorized)
				return
			}

			// Check token in Redis
			redisKey := fmt.Sprintf("user:%d:token", int32(userID))
			storedToken, err := redisClient.Get(r.Context(), redisKey)
			if err != nil || storedToken != tokenStr {
				slog.Error("invalid or revoked token", "user_id", userID, "error", err)
				http.Error(w, "invalid or revoked token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), "user_id", int32(userID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
