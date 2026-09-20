# Implementation Plan - Booking Service

Introduce the `booking` microservice to Homeshr for handling property reservations, ensuring consistency, preventing double-bookings, and integrating with Envoy for REST/JSON transcoding and JWT authentication.

## Architecture & Flow

```
+------------------------+
| Client / Postman (REST)|
+-----------+------------+
            | HTTP/JSON (Port 32769)
            v
+------------------------+
|   Envoy Gateway        |
| - jwt_authn            | (Validates JWT, injects x-user-id, x-user-email)
| - grpc_json_transcoder | (Transcodes HTTP/JSON <-> gRPC)
+-----------+------------+
            | gRPC (host.docker.internal:50052)
            v
+------------------------+
| Booking gRPC Server    | (services/booking/internal/grpcserver)
+-----------+------------+
            |
            v
+------------------------+
| Booking Domain Service | (services/booking/booking)
| - Date validation      |
| - Price calculation    |
| - Concurrency control  |
+-----+------------+-----+
      |            |
      v            v
+-----------+  +-----------+
| PostgreSQL|  |   Redis   |
| (Port 5433|  | (Port 6378|
| bookings) |  | locks)    |
+-----------+  +-----------+
```

---

## Infrastructure & Port Mappings

| Component | Container Name | Host Port | Container Port | Purpose |
| :--- | :--- | :--- | :--- | :--- |
| **Envoy Gateway** | `homeshr-booking-service-envoy` | `32769` | `10000` | REST/JSON to gRPC transcoding & JWT verification |
| **PostgreSQL** | `homeshr-booking-service-db` | `5433` | `5432` | Bookings persistence (Database: `homeshr`) |
| **Redis** | `homeshr-booking-service-redis` | `6378` | `6379` | Distributed locking & availability cache |
| **Booking gRPC App**| Host process | `50052` | N/A | Booking Service gRPC server |

---

## Implementation Details

### 1. Configuration & Tooling
- `Makefile`: Goose migrations (`up`, `down`, `status`, `create`), proto generation (`proto`, `vendor-proto`), and server run (`run`).
- `.env` & `.env.example`: DB connection, Redis connection, gRPC port (:50052), JWT secret.
- `envoy.yaml`:
  - Port 10000.
  - JWT auth filter with HS256 secret (`bXlfc3VwZXJfc2VjcmV0X2tleV8xMjM0NTY3ODk`).
  - `grpc_json_transcoder` pointing to `/etc/envoy/booking.pb`.
  - Upstream pointing to `host.docker.internal:50052`.

### 2. Protocol Buffers (`proto/`)
- `booking.proto`:
  - `CreateBooking`: `POST /v1/bookings` (requires auth, checks overlap, creates booking)
  - `GetBooking`: `GET /v1/bookings/{id}` (requires auth, checks guest ownership)
  - `ListUserBookings`: `GET /v1/bookings/list` (requires auth, lists bookings for caller)
  - `CancelBooking`: `POST /v1/bookings/{id}/cancel` (requires auth, cancels reservation)
  - `CheckAvailability`: `GET /v1/properties/{property_id}/availability` (public check)

### 3. Database & Migrations (`internal/`)
- Goose migration `20260917000000_create_bookings_table.sql`:
  - `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
  - `property_id UUID NOT NULL`
  - `guest_id UUID NOT NULL`
  - `check_in_date DATE NOT NULL`
  - `check_out_date DATE NOT NULL`
  - `total_guests INT NOT NULL DEFAULT 1`
  - `nightly_price BIGINT NOT NULL`
  - `total_price BIGINT NOT NULL`
  - `status VARCHAR(32) NOT NULL DEFAULT 'CONFIRMED'`
  - `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
  - `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`
- `sqlc.yaml` & `bookings.sql`: Queries for creating, fetching, listing, cancelling bookings, and atomic overlap checking.
- `internal/db/postgres.go`: Pgx connection pool initialization.

### 4. Redis Distributed Locking (`internal/cache/`)
- `redis.go`: Client connection and health ping.
- `lock.go`: Distributed lock helper with atomic acquire and Lua script release (`lock:property:{property_id}`) to serialize concurrent booking operations on the same property.

### 5. Domain Logic & gRPC Server
- `booking/booking.go`: Validates dates, acquires distributed lock, checks conflicts, calculates total price, and records booking.
- `booking/errors.go`: Domain errors (`ErrDatesUnavailable`, `ErrInvalidDates`, `ErrBookingNotFound`, `ErrUnauthorized`, etc.).
- `internal/grpcserver/server.go`: Maps gRPC requests/responses, unpacks metadata headers (`x-user-id`, `x-user-email`), and maps domain errors to gRPC codes (`codes.AlreadyExists`, `codes.InvalidArgument`, `codes.NotFound`, `codes.PermissionDenied`).
- `main.go`: Loads `.env`, establishes DB and Redis connections, registers gRPC server with reflection, and serves on `:50052`.

---

## Verification Plan

1. Compile protobuf stubs and Envoy descriptor: `make vendor-proto && make proto`.
2. Apply database migrations: `make up`.
3. Generate queries: `sqlc generate -f internal/sqlc.yaml`.
4. Compile and run booking service: `go run .`.
5. Test Endpoints:
   - Direct gRPC / grpcurl tests.
   - REST endpoints through Envoy on port `32769`:
     - Test checking availability.
     - Test creating booking with JWT token.
     - Test double-booking collision prevention (overlapping dates).
     - Test fetching booking details and listing user bookings.
     - Test cancelling booking and verifying dates become available again.

