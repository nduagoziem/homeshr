// Package grpcserver adapts the domain services to the generated gRPC
// UserService interface. It performs no business logic of its own: it maps
// requests/responses between the proto types and the auth/profile packages, and
// translates domain errors into gRPC status codes (which Envoy, configured with
// convert_grpc_status, turns back into HTTP+JSON for the client).
package grpcserver

import (
	"context"
	"errors"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/nduagoziem/homeshr/services/user/auth"
	"github.com/nduagoziem/homeshr/services/user/profile"
	userpb "github.com/nduagoziem/homeshr/services/user/proto"
)

// Server implements userpb.UserServiceServer.
type Server struct {
	userpb.UnimplementedUserServiceServer
	auth    *auth.AuthService
	profile *profile.ProfileService
}

// New returns a gRPC server backed by the given user-domain services.
func New(a *auth.AuthService, p *profile.ProfileService) *Server {
	return &Server{auth: a, profile: p}
}

// SendRegistrationOTP emails a one-time verification code to the address.
func (s *Server) SendRegistrationOTP(ctx context.Context, req *userpb.SendRegistrationOTPRequest) (*userpb.SendRegistrationOTPResponse, error) {
	if err := s.auth.SendRegistrationOTP(ctx, req.GetEmail()); err != nil {
		return nil, mapErr(err)
	}
	return &userpb.SendRegistrationOTPResponse{
		Message: "verification code sent",
	}, nil
}

// Register creates the account (verifying the OTP) and logs the user in.
func (s *Server) Register(ctx context.Context, req *userpb.RegisterRequest) (*userpb.LoginResponse, error) {
	res, err := s.auth.Register(ctx, req.GetEmail(), req.GetFullName(), req.GetPassword(), req.GetCode())
	if err != nil {
		return nil, mapErr(err)
	}
	return pbLoginResponse(res), nil
}

// Login authenticates the user and returns access + refresh tokens.
func (s *Server) Login(ctx context.Context, req *userpb.LoginRequest) (*userpb.LoginResponse, error) {
	res, err := s.auth.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, mapErr(err)
	}
	return pbLoginResponse(res), nil
}

// GetProfile returns the caller's profile. Envoy has already verified the JWT
// and forwarded the identity as the x-user-email metadata header.
func (s *Server) GetProfile(ctx context.Context, _ *userpb.GetProfileRequest) (*userpb.UserProfile, error) {
	email := metadataValue(ctx, "x-user-email")
	if email == "" {
		return nil, status.Error(codes.Unauthenticated, "missing verified identity")
	}

	user, err := s.profile.GetProfile(ctx, email)
	if err != nil {
		return nil, mapErr(err)
	}

	return &userpb.UserProfile{
		Id:       uuidString(user.ID),
		Email:    user.Email,
		FullName: textString(user.FullName),
	}, nil
}

// pbLoginResponse maps a domain LoginResponse to the proto message.
func pbLoginResponse(res auth.LoginResponse) *userpb.LoginResponse {
	return &userpb.LoginResponse{
		User: &userpb.User{
			Id:       uuidString(res.User.ID),
			Email:    res.User.Email,
			FullName: textString(res.User.FullName),
		},
		AccessToken:  res.AccessToken,
		RefreshToken: res.RefreshToken,
	}
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func textString(text pgtype.Text) string {
	if !text.Valid {
		return ""
	}
	return text.String
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
	case errors.Is(err, auth.ErrUserNotFound), errors.Is(err, profile.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		// Log the real cause server-side; return a generic message so
		// implementation details do not leak to clients.
		log.Printf("grpcserver: unhandled error, returning Internal: %v", err)
		return status.Error(codes.Internal, "internal error")
	}
}
