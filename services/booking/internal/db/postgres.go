package db

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresPool(ctx context.Context, databaseURL string) *Queries {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to the database %v\n", err)
	}

	// Testing the connection.
	err = pool.Ping(ctx)
	if err != nil {
		log.Fatalf("Could not ping database: %v\n", err)
	}

	log.Println("Database connection successful...")

	return New(pool)
}
