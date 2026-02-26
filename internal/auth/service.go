package auth

import (
	"errors"
	"time"

	"gv-api/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidToken    = errors.New("invalid token")
)

func GenerateToken(expiringTime time.Duration) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiringTime)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.JwtSecret))
}

func Login(password string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if password != cfg.Password {
		return "", ErrInvalidPassword
	}
	return GenerateToken(time.Minute * 5)
}

func ValidateToken(tokenString string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrInvalidKeyType
		}
		return []byte(cfg.JwtSecret), nil
	})

	if err != nil || !token.Valid {
		return ErrInvalidToken
	}

	return nil
}
