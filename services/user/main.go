package main

import (
	"context"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/nduagoziem/homeshr/services/user/internal/db"
)

func main() {

	_ = godotenv.Load("/home/nduagoziem/dev/homeshr/services/user/.env")
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
