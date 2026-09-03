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
	"github.com/nduagoziem/homeshr/services/user/internal/db"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrQueriesRequired    = errors.New("auth service requires db queries")
)

const defaultRefreshTokenTTL = 30 * 24 * time.Hour

// AuthService provides authentication functionality for the user micro-service.
type AuthService struct {
	queries        *db.Queries
	accessTokenTTL time.Duration
	jwtSecret      []byte
}

// NewAuthService creates a new authentication service.
func NewAuthService(queries *db.Queries, accessTokenTTL time.Duration, jwtSecret string) *AuthService {
	return &AuthService{
		queries:        queries,
		accessTokenTTL: accessTokenTTL,
		jwtSecret:      []byte(jwtSecret),
	}
}

// Register creates a new user with the provided credentials.
func (s *AuthService) Register(ctx context.Context, email, fullName, password string) (db.CreateUserRow, error) {

	//Check if the queries are set
	if s.queries == nil {
		return db.CreateUserRow{}, ErrQueriesRequired
	}

	// Check if user exists
	_, err := s.queries.FindUserByEmail(ctx, email)
	if err == nil {
		return db.CreateUserRow{}, ErrUserAlreadyExists
	}

	// Only proceed if the error was "user not found"
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.CreateUserRow{}, err
	}

	// Hash the user's password
	pass, err := hashPassword(password)
	if err != nil {
		return db.CreateUserRow{}, err
	}

	return s.queries.CreateUser(ctx, db.CreateUserParams{
		ID:           pgtype.UUID{Bytes: uuid.New(), Valid: true},
		Email:        email,
		PasswordHash: pass,
		FullName:     pgtype.Text{String: fullName, Valid: fullName != ""},
	})
}

type LoginResponse struct {
	User         LoginUser `json:"user"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
}

type LoginUser struct {
	ID       pgtype.UUID `json:"id"`
	Email    string      `json:"email"`
	FullName pgtype.Text `json:"full_name"`
}

func (s *AuthService) Login(ctx context.Context, email, password string) (LoginResponse, error) {
	if s.queries == nil {
		return LoginResponse{}, ErrQueriesRequired
	}

	user, err := s.queries.FindUserByEmail(ctx, email)

	// Check if the user exists
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginResponse{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResponse{}, err
	}

	if err := verifyPassword(user.PasswordHash, password); err != nil {
		return LoginResponse{}, ErrInvalidCredentials
	}

	accessToken, err := s.generateAccessToken(user)
	if err != nil {
		return LoginResponse{}, err
	}

	// Generate a refresh token and store it in the database
	now := time.Now()
	refreshToken := uuid.NewString()
	_, err = s.queries.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		Token:     refreshToken,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: now.Add(defaultRefreshTokenTTL), Valid: true},
		Revoked:   false,
	})
	if err != nil {
		return LoginResponse{}, err
	}

	// User logged in, update the timestamp/status of last login
	if err := s.queries.UpdateLastLoginStatus(ctx, db.UpdateLastLoginStatusParams{
		LastLogin: pgtype.Timestamptz{Time: now, Valid: true},
		ID:        user.ID,
	}); err != nil {
		return LoginResponse{}, err
	}

	return LoginResponse{
		User: LoginUser{
			ID:       user.ID,
			Email:    user.Email,
			FullName: user.FullName,
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
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

// generateAccessToken creates a new JWT access token
func (s *AuthService) generateAccessToken(user db.FindUserByEmailRow) (string, error) {
	// Set the expiration time
	now := time.Now().UTC()
	expirationTime := now.Add(s.accessTokenTTL)

	// Create the JWT claims
	claims := jwt.MapClaims{
		"sub":   uuid.UUID(user.ID.Bytes).String(), // subject (user ID)
		"email": user.Email,                        // custom claim
		"exp":   expirationTime.Unix(),             // expiration time
		"iat":   now.Unix(),                        // issued at time
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

// AuthenticateWithTOTP uses a password-less/TOTP style for authentication.
//
// The user provides an email and a Time based One Time Password (TOTP) is sent to the email.
// If the TOTP is valid and email exists, the user is logged in, else a new user is created.
// func AuthenticateWithTOTP(email, fullName, totp string,) (string, error) {

// }
