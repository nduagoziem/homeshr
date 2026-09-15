// Package auth provides authentication services for the user micro-service.
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nduagoziem/homeshr/services/user/internal/cache"
	"github.com/nduagoziem/homeshr/services/user/internal/db"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserAlreadyExists   = errors.New("user already exists")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrQueriesRequired     = errors.New("auth service requires db queries")
	ErrInvalidOTP          = errors.New("invalid or expired verification code")
	ErrUserNotFound        = errors.New("user not found")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
)

const defaultRefreshTokenTTL = 30 * 24 * time.Hour

// AuthService provides authentication functionality for the user micro-service.
type AuthService struct {
	queries        *db.Queries
	redis          *cache.Cache
	accessTokenTTL time.Duration
	jwtSecret      []byte
	resendAPIKey   string
	otpSender      string // homeshr mail address OTPs are sent from
}

// AuthServiceConfig carries the dependencies and settings required by AuthService.
type AuthServiceConfig struct {
	Queries        *db.Queries
	Redis          *cache.Cache
	AccessTokenTTL time.Duration
	JWTSecret      string
	ResendAPIKey   string
	OTPSender      string
}

// NewAuthService creates a new authentication service.
func NewAuthService(cfg AuthServiceConfig) *AuthService {
	return &AuthService{
		queries:        cfg.Queries,
		redis:          cfg.Redis,
		accessTokenTTL: cfg.AccessTokenTTL,
		jwtSecret:      []byte(cfg.JWTSecret),
		resendAPIKey:   cfg.ResendAPIKey,
		otpSender:      cfg.OTPSender,
	}
}

// AuthResponse represents the response returned after a successful login or registration.
type AuthResponse struct {
	User                  AuthRequest `json:"user"`
	AccessToken           string      `json:"access_token"`
	RefreshToken          string      `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time   `json:"-"`
}

type AuthRequest struct {
	ID       pgtype.UUID `json:"id"`
	Email    string      `json:"email"`
	FullName pgtype.Text `json:"full_name"`
}

// SendRegistrationOTP verifies that the email is not already taken and emails a
// one-time verification code to it. This is the first step of registration: the
// user must supply the returned code back to Register to prove ownership of the
// email before an account is created.
func (s *AuthService) SendRegistrationOTP(ctx context.Context, email string) error {
	if s.queries == nil {
		return ErrQueriesRequired
	}

	// Reject if a user already owns this email.
	_, err := s.queries.FindUserByEmail(ctx, email)
	if err == nil {
		return ErrUserAlreadyExists
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	return sendOTP(sendOTPParams{
		ctx:      ctx,
		redis:    *s.redis,
		apiKey:   s.resendAPIKey,
		sender:   s.otpSender,
		receiver: email,
	})
}

// Register creates a new user with the provided credentials and logs in immediately.
//
// The code is the OTP previously emailed by SendRegistrationOTP; it must be valid
// for the given email or the account is not created.
func (s *AuthService) Register(ctx context.Context, email, fullName, password, code string) (AuthResponse, error) {

	//Check if the queries are set
	if s.queries == nil {
		return AuthResponse{}, ErrQueriesRequired
	}

	// Check if user exists
	_, err := s.queries.FindUserByEmail(ctx, email)
	if err == nil {
		return AuthResponse{}, ErrUserAlreadyExists
	}

	// Only proceed if the error was "user not found"
	if !errors.Is(err, pgx.ErrNoRows) {
		return AuthResponse{}, err
	}

	// Verify the OTP proves the user owns this email before creating the account.
	valid, err := validateOTP(ctx, code, email, *s.redis)
	if err != nil {
		// A missing key means the code was never sent or has expired.
		if errors.Is(err, redis.Nil) {
			return AuthResponse{}, ErrInvalidOTP
		}
		return AuthResponse{}, err
	}
	if !valid {
		return AuthResponse{}, ErrInvalidOTP
	}

	// Hash the user's password
	pass, err := hashPassword(password)
	if err != nil {
		return AuthResponse{}, err
	}

	user, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		ID:           pgtype.UUID{Bytes: uuid.New(), Valid: true},
		Email:        email,
		PasswordHash: pass,
		FullName:     pgtype.Text{String: fullName, Valid: fullName != ""},
	})
	if err != nil {
		return AuthResponse{}, err
	}

	return s.issueTokens(ctx, AuthRequest{
		ID:       user.ID,
		Email:    user.Email,
		FullName: user.FullName,
	})
}

func (s *AuthService) Login(ctx context.Context, email, password string) (AuthResponse, error) {
	if s.queries == nil {
		return AuthResponse{}, ErrQueriesRequired
	}

	user, err := s.queries.FindUserByEmail(ctx, email)

	// Check if the user exists
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthResponse{}, ErrInvalidCredentials
	}
	if err != nil {
		return AuthResponse{}, err
	}

	if err := verifyPassword(user.PasswordHash, password); err != nil {
		return AuthResponse{}, ErrInvalidCredentials
	}

	return s.issueTokens(ctx, AuthRequest{
		ID:       user.ID,
		Email:    user.Email,
		FullName: user.FullName,
	})
}

// RefreshAccessToken generates a new access token when the existing one has expired.
func (s *AuthService) RefreshAccessToken(ctx context.Context, refreshToken string) (AuthResponse, error) {
	if s.queries == nil {
		return AuthResponse{}, ErrQueriesRequired
	}
	if refreshToken == "" {
		return AuthResponse{}, ErrInvalidRefreshToken
	}

	storedToken, err := s.queries.GetRefreshToken(ctx, refreshToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthResponse{}, ErrInvalidRefreshToken
	}
	if err != nil {
		return AuthResponse{}, err
	}

	now := time.Now().UTC()
	if storedToken.Revoked || !storedToken.ExpiresAt.Valid || !storedToken.ExpiresAt.Time.After(now) {
		if !storedToken.Revoked {
			_ = s.queries.RevokeRefreshTokenByID(ctx, storedToken.ID)
		}
		return AuthResponse{}, ErrInvalidRefreshToken
	}

	user, err := s.queries.FindUserByID(ctx, storedToken.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return AuthResponse{}, ErrInvalidRefreshToken
	}
	if err != nil {
		return AuthResponse{}, err
	}

	if err := s.queries.RevokeRefreshTokenByID(ctx, storedToken.ID); err != nil {
		return AuthResponse{}, err
	}

	return s.issueTokens(ctx, AuthRequest{
		ID:       user.ID,
		Email:    user.Email,
		FullName: user.FullName,
	})
}

func hashPassword(password string) (string, error) {

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

func verifyPassword(hashedPass, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPass), []byte(password))
}

// generateAccessToken creates a new JWT access token.
func (s *AuthService) generateAccessToken(user AuthRequest, refreshTokenID pgtype.UUID) (string, error) {
	// Set the expiration time
	now := time.Now().UTC()
	expirationTime := now.Add(s.accessTokenTTL)

	// Create the JWT claims
	claims := jwt.MapClaims{
		"sub":   uuid.UUID(user.ID.Bytes).String(),        // subject (user ID)
		"email": user.Email,                               // custom claim
		"rtid":  uuid.UUID(refreshTokenID.Bytes).String(), // refresh token ID
		"exp":   expirationTime.Unix(),                    // expiration time
		"iat":   now.Unix(),                               // issued at time
	}

	// Create the token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token with our secret key
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// issueTokens issues access tokens from generateAccessToken
// and rotates/creates a new refresh token.
func (s *AuthService) issueTokens(ctx context.Context, user AuthRequest) (AuthResponse, error) {
	now := time.Now().UTC()
	refreshTokenExpiresAt := now.Add(defaultRefreshTokenTTL)
	refreshToken := uuid.NewString()
	storedToken, err := s.queries.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		Token:     refreshToken,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: refreshTokenExpiresAt, Valid: true},
		Revoked:   false,
	})
	if err != nil {
		return AuthResponse{}, err
	}

	accessToken, err := s.generateAccessToken(user, storedToken.ID)
	if err != nil {
		return AuthResponse{}, err
	}

	if err := s.queries.UpdateLastLoginStatus(ctx, db.UpdateLastLoginStatusParams{
		LastLogin: pgtype.Timestamptz{Time: now, Valid: true},
		ID:        user.ID,
	}); err != nil {
		return AuthResponse{}, err
	}

	return AuthResponse{
		User:                  user,
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		RefreshTokenExpiresAt: refreshTokenExpiresAt,
	}, nil
}

// AuthenticateWithTOTP uses a password-less/TOTP style of authentication.
//
// The user provides an email and a Time based One Time Password (TOTP) is sent to the email.
// If the TOTP is valid and email exists, the user is logged in, else a new user is created.
// func AuthenticateWithTOTP(email, fullName, totp string,) (string, error) {

// }
