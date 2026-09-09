package main

import (
	"context"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/nduagoziem/homeshr/services/user/auth"
	"github.com/nduagoziem/homeshr/services/user/internal/cache"
	"github.com/nduagoziem/homeshr/services/user/internal/db"
	"github.com/nduagoziem/homeshr/services/user/internal/grpcserver"
	authpb "github.com/nduagoziem/homeshr/services/user/proto"
)

const (
	defaultGRPCPort       = ":50051"
	defaultAccessTokenTTL = 15 * time.Minute
)

func main() {
	_ = loadServiceEnv()

	ctx := context.Background()

	queries := db.NewPostgresPool(ctx, os.Getenv("DATABASE_URL"))
	redis := cache.NewRedisCache(ctx, os.Getenv("REDIS_URL"))

	authService := auth.NewAuthService(auth.AuthServiceConfig{
		Queries:        queries,
		Redis:          redis,
		AccessTokenTTL: accessTokenTTL(),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		ResendAPIKey:   os.Getenv("RESEND_API_KEY"),
		OTPSender:      os.Getenv("OTP_SENDER"),
	})

	gRPCPort := os.Getenv("GRPC_PORT")
	if gRPCPort == "" {
		gRPCPort = defaultGRPCPort
	}

	lis, err := net.Listen("tcp", gRPCPort)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", gRPCPort, err)
	}

	grpcServer := grpc.NewServer()
	authpb.RegisterAuthserviceServer(grpcServer, grpcserver.New(authService))
	// Reflection lets tools like grpcurl introspect the service during development.
	reflection.Register(grpcServer)

	log.Printf("gRPC auth server listening on %s", gRPCPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server stopped: %v", err)
	}
}

// accessTokenTTL reads ACCESS_TOKEN_TTL (a Go duration string, e.g. "15m") and
// falls back to defaultAccessTokenTTL when unset or invalid.
func accessTokenTTL() time.Duration {
	raw := os.Getenv("ACCESS_TOKEN_TTL")
	if raw == "" {
		return defaultAccessTokenTTL
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("invalid ACCESS_TOKEN_TTL %q, using default %s", raw, defaultAccessTokenTTL)
		return defaultAccessTokenTTL
	}
	return ttl
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
