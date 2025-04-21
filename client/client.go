package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
)

var Port int

type PrivateBody struct {
	Key     string `json:"key"`
	Message string `json:"Message"`
	TraceID string `json:"trace_id"`
	Claims  string `json:"claims"`
}

func main() {
	flag.IntVar(&Port, "port", 3000, "Port to listen on")
	flag.Parse()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Logger.SetLevel(log.INFO)

	e.GET("/", hello)
	e.GET("/private", private)
	e.GET("/login", login)
	e.GET("/health", health)
	e.GET("/redirect", redirect)

	if err := e.Start(":" + fmt.Sprint(Port)); err != nil && err != http.ErrServerClosed {
		e.Logger.Fatal("shutting down the server")
	}
}

func hello(c echo.Context) error {
	return c.String(http.StatusOK, "Hello World! from client")
}

func health(c echo.Context) error {
	return c.String(http.StatusOK, "OK")
}

func private(c echo.Context) error {
	cookie, err := c.Cookie("token")
	var authToken string
	if err == nil {
		authToken = cookie.Value
	} else {
		authToken = c.Request().Header.Get("Authorization")
	}
	return c.JSON(http.StatusOK, PrivateBody{
		Key:     authToken,
		TraceID: c.Request().Header.Get("X-Trace-ID"),
		Message: "Accessing Private Area",
		Claims:  c.Request().Header.Get("X-Claims"),
	})
}

func redirect(c echo.Context) error {
	return c.Redirect(http.StatusPermanentRedirect, "http://localhost:8080/private")
}

func login(c echo.Context) error {
	token, err := generateJWT()
	if err != nil {
		return c.String(http.StatusInternalServerError, "jwt generation failed")
	}
	c.SetCookie(&http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/example",
		HttpOnly: true,
	})
	return c.String(http.StatusOK, token)
}

func generateJWT() (string, error) {
	key := "test"
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"service": "test_client",
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})
	return t.SignedString([]byte(key))
}
