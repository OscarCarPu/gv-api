package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidToken    = errors.New("invalid token")
)

// TODO: Move to .env
var (
	jwtSecret      = []byte("secret")
	storedPassword = "Abc123.."
)

func GenerateToken(expiringTime time.Duration) (string, error) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiringTime)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func Login(password string) (string, error) {
	if password != storedPassword {
		return "", ErrInvalidPassword
	}
	return GenerateToken(time.Minute * 5)
}

func ValidateToken(tokenString string) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrInvalidKeyType
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return ErrInvalidToken
	}

	return nil
}
