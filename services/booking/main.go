package main

import (
	"context"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"

	"github.com/nduagoziem/homeshr/services/booking/booking"
	"github.com/nduagoziem/homeshr/services/booking/internal/cache"
	"github.com/nduagoziem/homeshr/services/booking/internal/db"
	"github.com/nduagoziem/homeshr/services/booking/internal/grpcserver"
	bookingpb "github.com/nduagoziem/homeshr/services/booking/proto"
)

const defaultGRPCPort = ":50052"

func main() {
	_ = loadServiceEnv()

	ctx := context.Background()

	queries := db.NewPostgresPool(ctx, os.Getenv("DATABASE_URL"))
	redis := cache.NewRedisCache(ctx, os.Getenv("REDIS_URL"))

	// Booking domain service (Postgres for persistence, Redis for per-property
	// locking).
	bookingService := booking.NewService(booking.ServiceConfig{
		Queries: queries,
		Locker:  redis,
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
	bookingpb.RegisterBookingServiceServer(grpcServer, grpcserver.New(bookingService))

	// Reflection lets tools like grpcurl introspect the service during development.
	// reflection.Register(grpcServer)

	log.Printf("gRPC booking server listening on %s", gRPCPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server stopped: %v", err)
	}
}

// loadServiceEnv loads the .env file in the booking service.
func loadServiceEnv() error {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return godotenv.Load(".env", filepath.Join("services", "booking", ".env"))
	}

	serviceEnv := filepath.Join(filepath.Dir(file), ".env")
	return godotenv.Load(".env", filepath.Join("services", "booking", ".env"), serviceEnv)
}
