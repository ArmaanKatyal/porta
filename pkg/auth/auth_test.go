package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/ArmaanKatyal/go-api-gateway/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// Mock for a failing reader (returns an error on read)
type failingReader struct{}

func (failingReader) Read([]byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestAuthNewJwtAuth(t *testing.T) {
	t.Run("valid file input", func(t *testing.T) {
		input := "test_secret_data"
		reader := bytes.NewReader([]byte(input))
		jwtAuth := NewJwtAuth(&config.AuthSettings{Enabled: true, Anonymous: false,
			Routes: []string{"route1", "route2"}, Secret: "/path"}, reader)
		assert.True(t, jwtAuth.Enabled)
		assert.False(t, jwtAuth.Anonymous)
		assert.Len(t, jwtAuth.Routes, 2)
		assert.Equal(t, []byte(input), jwtAuth.getSecret())
	})
	t.Run("invalid file input", func(t *testing.T) {
		reader := failingReader{}
		jwtAuth := NewJwtAuth(&config.AuthSettings{Enabled: true, Anonymous: false,
			Routes: []string{"route1", "route2"}, Secret: "/path"}, reader)
		assert.True(t, jwtAuth.Enabled)
		assert.False(t, jwtAuth.Anonymous)
		assert.Len(t, jwtAuth.Routes, 2)
		assert.Equal(t, []byte(DefaultSecret), jwtAuth.getSecret())
	})
}

func TestAuthPathInRoutes(t *testing.T) {
	t.Run("path in routes", func(t *testing.T) {
		input := "test_secret_data"
		reader := bytes.NewReader([]byte(input))
		jwtAuth := NewJwtAuth(&config.AuthSettings{Enabled: true, Anonymous: false,
			Routes: []string{"route1", "route2"}, Secret: "/path"}, reader)
		assert.True(t, jwtAuth.pathInRoutes("route1"))
	})
	t.Run("path not in routes", func(t *testing.T) {
		input := "test_secret_data"
		reader := bytes.NewReader([]byte(input))
		jwtAuth := NewJwtAuth(&config.AuthSettings{Enabled: true, Anonymous: false,
			Routes: []string{"route1", "route2"}, Secret: "/path"}, reader)
		assert.False(t, jwtAuth.pathInRoutes("route3"))
	})
}

func TestAuthIsEnabled(t *testing.T) {
	input := "test_secret_data"
	reader := bytes.NewReader([]byte(input))
	jwtAuth := NewJwtAuth(&config.AuthSettings{Enabled: true, Anonymous: false,
		Routes: []string{"route1", "route2"}, Secret: "/path"}, reader)
	assert.True(t, jwtAuth.IsEnabled())
}

func TestAuthGetSecret(t *testing.T) {
	input := "test_secret_data"
	reader := bytes.NewReader([]byte(input))
	jwtAuth := NewJwtAuth(&config.AuthSettings{Enabled: true, Anonymous: false,
		Routes: []string{"route1", "route2"}, Secret: "/path"}, reader)
	assert.Equal(t, []byte(input), jwtAuth.getSecret())
}

func generateRequest(token string, path string) *http.Request {
	req := &http.Request{
		Header: http.Header{
			"Authorization": []string{token},
		},
		URL: &url.URL{
			Path: path,
		},
	}
	return req
}

func generateToken(key string, exp int64) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"service": "test",
		"exp":     exp,
	})
	return t.SignedString([]byte(key))
}

func TestAuthAuthenticate(t *testing.T) {

	defJwtAuth := func() *config.AuthSettings {
		return &config.AuthSettings{
			Enabled:   true,
			Anonymous: false,
			Routes:    []string{"/route1"},
			Secret:    "/test/path",
		}
	}

	t.Run("path not in routes", func(t *testing.T) {
		j := NewJwtAuth(defJwtAuth(), bytes.NewReader([]byte("test")))
		err := j.Authenticate(generateRequest("test", "/test/route2"))
		assert.Nil(t, err)
	})
	t.Run("auth disabled", func(t *testing.T) {
		j := NewJwtAuth(&config.AuthSettings{
			Enabled:   false,
			Anonymous: false,
			Routes:    []string{"/route1"},
		}, bytes.NewReader([]byte("test")))
		err := j.Authenticate(generateRequest("test", "/test/route1"))
		assert.Nil(t, err)
	})
	t.Run("token missing", func(t *testing.T) {
		j := NewJwtAuth(defJwtAuth(), bytes.NewReader([]byte("test")))
		err := j.Authenticate(generateRequest("", "/test/route1"))
		assert.Equal(t, ErrTokenMissing, err)
	})
	t.Run("token missing anonymous enabled", func(t *testing.T) {
		j := NewJwtAuth(&config.AuthSettings{
			Enabled:   true,
			Anonymous: true,
			Routes:    []string{"/route1"},
		}, bytes.NewReader([]byte("test")))
		err := j.Authenticate(generateRequest("", "/test/route1"))
		assert.Nil(t, err)
	})
	t.Run("invalid token", func(t *testing.T) {
		j := NewJwtAuth(defJwtAuth(), bytes.NewReader([]byte("test")))
		token, err := generateToken("test", 0)
		assert.Nil(t, err)
		err = j.Authenticate(generateRequest(token, "/test/route1"))
		assert.Equal(t, ErrInvalidToken, err)
	})
	t.Run("valid token", func(t *testing.T) {
		exp := time.Now().Add(time.Hour * 24).Unix()
		token, err := generateToken("test", exp)
		req := generateRequest(token, "/test/route1")
		assert.Nil(t, err)
		j := NewJwtAuth(defJwtAuth(), bytes.NewReader([]byte("test")))
		err = j.Authenticate(req)
		assert.Nil(t, err)
		claims := map[string]interface{}{
			"service": "test",
			"exp":     exp,
		}
		expected, err := json.Marshal(claims)
		assert.Nil(t, err)
		assert.JSONEq(t, string(expected), req.Header.Get("X-Claims"))
	})
}
