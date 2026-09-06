package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/nduagoziem/homeshr/services/user/internal/db"
)

func main() {

	_ = loadServiceEnv()
	databaseURL := os.Getenv("DATABASE_URL")

	ctx := context.Background()

	db.NewPostgresPool(ctx, databaseURL)

	e := echo.New()

	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	e.GET("/", func(c *echo.Context) error {

		return c.JSON(http.StatusOK, map[string]string{"message": "Hello World, from Homeshr❤!"})
	})

	if err := e.Start(":1323"); err != nil {
		e.Logger.Error("failed to start server", "error", err)
	}
}

// loadServiceEnv loads the .env file in user service.
func loadServiceEnv() error {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return godotenv.Load(".env", filepath.Join("services", "user", ".env"))
	}

	serviceEnv := filepath.Join(filepath.Dir(file), ".env")
	return godotenv.Load(".env", filepath.Join("services", "user", ".env"), serviceEnv)
}
