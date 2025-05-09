package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ArmaanKatyal/porta/pkg/config"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Service string `json:"service"`
	jwt.RegisteredClaims
}

type JwtError error

const (
	DefaultSecret = "test"
)

var (
	ErrTokenMissing JwtError = errors.New("missing auth token")
	ErrInvalidToken JwtError = errors.New("invalid auth token")
)

type JwtAuth struct {
	Enabled   bool     `json:"enabled"`
	Anonymous bool     `json:"anonymous"`
	Routes    []string `json:"routes"`
	secret    []byte
}

func (j *JwtAuth) getSecret() []byte {
	return j.secret
}

func resolvePath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[2:], "/")
}

// Authenticate checks if the request has a valid JWT token in the header
func (j *JwtAuth) Authenticate(r *http.Request) JwtError {
	token := r.Header.Get("Authorization")
	path := "/" + resolvePath(r.URL.Path)
	slog.Info("Authenticating request", "path", path)
	exists := j.pathInRoutes(path)
	if exists && j.IsEnabled() {
		if token == "" {
			if j.Anonymous {
				slog.Warn("Anonymous request", "path", path)
				return nil
			}
			return ErrTokenMissing
		}
		// parse token
		claims := &Claims{}
		parsed, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (any, error) {
			return j.getSecret(), nil
		})
		if err != nil {
			if j.Anonymous {
				slog.Warn("Anonymous request", "path", path)
				return nil
			}
			slog.Error("Error parsing token", "error", err.Error(), "path", path)
			return ErrInvalidToken
		}
		if !parsed.Valid {
			slog.Error("Invalid token", "path", path)
			return ErrInvalidToken
		}

		// Check expiration
		if claims.ExpiresAt.Unix() < time.Now().Unix() {
			slog.Error("Token expired", "path", path)
			if j.Anonymous {
				slog.Warn("Anonymous request", "path", path)
				return nil
			}
			return ErrInvalidToken
		}

		c, err := json.Marshal(claims)
		if err != nil {
			slog.Error("Error marshalling claims", "error", err.Error(), "path", path)
			return err
		}

		// Append claims to Header
		r.Header.Add("X-Claims", string(c))
	}
	return nil
}

func (j *JwtAuth) pathInRoutes(path string) bool {
	for _, route := range j.Routes {
		if route == path {
			return true
		}
	}
	return false
}

func (j *JwtAuth) IsEnabled() bool {
	return j.Enabled
}

func NewJwtAuth(conf *config.AuthSettings, reader io.Reader) *JwtAuth {
	ja := &JwtAuth{
		Enabled:   conf.Enabled,
		Anonymous: conf.Anonymous,
		Routes:    conf.Routes,
	}

	// Read from the provided reader, regardless of the type
	data, err := io.ReadAll(reader)
	if err != nil {
		slog.Error("Error reading secret or empty secret file", "error", err)
		data = []byte(DefaultSecret)
	}
	ja.secret = data
	return ja
}
