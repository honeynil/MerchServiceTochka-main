package auth

import (
	"context"
	"encoding/json"
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
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "authorization header missing"})
				return
			}

			parts := strings.Split(authHeader, " ")

			if len(parts) != 2 || parts[0] != "Bearer" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid authorization header"})
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
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid token claims"})
				return
			}

			userID, ok := claims["user_id"].(float64)
			if !ok {

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid user_id in token"})
				return
			}

			redisKey := fmt.Sprintf("user:%d:token", int32(userID))
			storedToken, err := redisClient.Get(r.Context(), redisKey)
			if err != nil || storedToken != tokenStr {
				slog.Error("invalid or revoked token", "user_id", userID, "error", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid or revoked token"})
				return
			}

			ctx := context.WithValue(r.Context(), "user_id", int32(userID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
