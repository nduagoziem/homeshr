// Package grpcserver adapts the domain auth.AuthService to the generated gRPC
// Authservice interface. It performs no business logic of its own: it maps
// requests/responses between the proto types and the auth package, and
// translates domain errors into gRPC status codes (which Envoy, configured with
// convert_grpc_status, turns back into HTTP+JSON for the client).
package grpcserver

import (
	"context"
	"errors"
	"log"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/nduagoziem/homeshr/services/user/auth"
	authpb "github.com/nduagoziem/homeshr/services/user/proto"
)

// Server implements authpb.AuthserviceServer.
type Server struct {
	authpb.UnimplementedAuthserviceServer
	auth *auth.AuthService
}

// New returns a gRPC server backed by the given auth service.
func New(a *auth.AuthService) *Server {
	return &Server{auth: a}
}

// SendRegistrationOTP emails a one-time verification code to the address.
func (s *Server) SendRegistrationOTP(ctx context.Context, req *authpb.SendRegistrationOTPRequest) (*authpb.SendRegistrationOTPResponse, error) {
	if err := s.auth.SendRegistrationOTP(ctx, req.GetEmail()); err != nil {
		return nil, mapErr(err)
	}
	return &authpb.SendRegistrationOTPResponse{
		Message: "verification code sent",
	}, nil
}

// Register creates the account (verifying the OTP) and logs the user in.
func (s *Server) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.LoginResponse, error) {
	res, err := s.auth.Register(ctx, req.GetEmail(), req.GetFullName(), req.GetPassword(), req.GetCode())
	if err != nil {
		return nil, mapErr(err)
	}
	return pbLoginResponse(res), nil
}

// Login authenticates the user and returns access + refresh tokens.
func (s *Server) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	res, err := s.auth.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, mapErr(err)
	}
	return pbLoginResponse(res), nil
}

// GetProfile returns the caller's profile. Envoy has already verified the JWT
// and forwarded the identity as the x-user-email metadata header.
func (s *Server) GetProfile(ctx context.Context, _ *authpb.GetProfileRequest) (*authpb.UserProfile, error) {
	email := metadataValue(ctx, "x-user-email")
	if email == "" {
		return nil, status.Error(codes.Unauthenticated, "missing verified identity")
	}

	user, err := s.auth.GetProfile(ctx, email)
	if err != nil {
		return nil, mapErr(err)
	}

	return &authpb.UserProfile{
		Id:       userID(user),
		Email:    user.Email,
		FullName: fullName(user),
	}, nil
}

// pbLoginResponse maps a domain LoginResponse to the proto message.
func pbLoginResponse(res auth.LoginResponse) *authpb.LoginResponse {
	return &authpb.LoginResponse{
		User: &authpb.User{
			Id:       userID(res.User),
			Email:    res.User.Email,
			FullName: fullName(res.User),
		},
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
	}
}

func userID(u auth.LoginUser) string {
	if !u.ID.Valid {
		return ""
	}
	return uuid.UUID(u.ID.Bytes).String()
}

func fullName(u auth.LoginUser) string {
	if !u.FullName.Valid {
		return ""
	}
	return u.FullName.String
}

// metadataValue returns the first value for key in the incoming gRPC metadata.
func metadataValue(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// mapErr translates domain errors into gRPC status errors. Unknown errors are
// reported as Internal with a generic message so implementation details do not
// leak to clients.
func mapErr(err error) error {
	switch {
	case errors.Is(err, auth.ErrUserAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, auth.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, auth.ErrInvalidOTP):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, auth.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		// Log the real cause server-side; return a generic message so
		// implementation details do not leak to clients.
		log.Printf("grpcserver: unhandled error, returning Internal: %v", err)
		return status.Error(codes.Internal, "internal error")
	}
}
