package auth

import (
	"errors"
	"time"

	"gv-api/internal/config"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"
)

var (
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidToken    = errors.New("invalid token")
	ErrInvalidCode     = errors.New("invalid 2fa code")
)

var validateTOTP = totp.Validate // for easy mocking

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

func Login2FA(tokenString, code string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	err = ValidateToken(tokenString)
	if err != nil {
		return "", err
	}
	valid := validateTOTP(code, cfg.TotpSecret)
	if !valid {
		return "", ErrInvalidCode
	}

	token, err := GenerateToken(time.Hour * 24)
	if err != nil {
		return "", err
	}
	return token, nil
}
