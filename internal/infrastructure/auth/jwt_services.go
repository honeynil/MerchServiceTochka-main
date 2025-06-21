package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

func SetJWTSecret(secret []byte) {
	jwtSecret = secret
}

func GenerateJWT(username string) (string, error) {
	if len(jwtSecret) == 0 {
		return "", fmt.Errorf("JWT secret not set")
	}
	claims := jwt.MapClaims
}

func ValedateJWT() {}
