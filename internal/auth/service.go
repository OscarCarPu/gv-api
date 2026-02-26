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

type TOTPValidator func(passcode string, secret string) bool

type Service struct {
	cfg          *config.Config
	validateTOTP TOTPValidator
}

func NewService(cfg *config.Config, totpVal TOTPValidator) *Service {
	if totpVal == nil {
		totpVal = totp.Validate
	}
	return &Service{
		cfg:          cfg,
		validateTOTP: totpVal,
	}
}

func (s *Service) GenerateToken(expiringTime time.Duration) (string, error) {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiringTime)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JwtSecret))
}

func (s *Service) Login(password string) (string, error) {
	if password != s.cfg.Password {
		return "", ErrInvalidPassword
	}
	return s.GenerateToken(time.Minute * 5)
}

func (s *Service) ValidateToken(tokenString string) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrInvalidKeyType
		}
		return []byte(s.cfg.JwtSecret), nil
	})

	if err != nil || !token.Valid {
		return ErrInvalidToken
	}

	return nil
}

func (s *Service) Login2FA(tokenString, code string) (string, error) {
	err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	valid := s.validateTOTP(code, s.cfg.TotpSecret)
	if !valid {
		return "", ErrInvalidCode
	}

	return s.GenerateToken(time.Hour * 24)
}
