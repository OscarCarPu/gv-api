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

type Claims struct {
	jwt.RegisteredClaims
	Kind string `json:"kind"` // "tmp" or "full"
}

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

func (s *Service) GenerateToken(expiringTime time.Duration, kind string) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiringTime)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Kind: kind,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JwtSecret))
}

func (s *Service) Login(password string) (string, string, error) {
	switch password {
	case s.cfg.Password:
		token, err := s.GenerateToken(time.Minute*5, "tmp")
		return token, "tmp", err
	case s.cfg.SemiprivatePassword:
		token, err := s.GenerateToken(time.Hour*24*30, "semi")
		return token, "semi", err
	default:
		return "", "", ErrInvalidPassword
	}
}

func (s *Service) ValidateToken(tokenString, kind string) error {
	var claims Claims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrInvalidKeyType
		}
		return []byte(s.cfg.JwtSecret), nil
	})

	if err != nil || !token.Valid {
		return ErrInvalidToken
	}

	if claims.Kind != kind {
		return ErrInvalidToken
	}

	return nil
}

func (s *Service) Login2FA(tokenString, code string) (string, error) {
	err := s.ValidateToken(tokenString, "tmp")
	if err != nil {
		return "", err
	}

	valid := s.validateTOTP(code, s.cfg.TotpSecret)
	if !valid {
		return "", ErrInvalidCode
	}

	return s.GenerateToken(time.Hour*24*30, "full")
}
