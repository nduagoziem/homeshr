// Package profile provides a service for retrieving user profile information from the database.
package profile

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nduagoziem/homeshr/services/user/internal/db"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrQueriesRequired = errors.New("user profile requires db queries")
)

type ProfileService struct {
	Queries *db.Queries
}

type UserProfile struct {
	ID       pgtype.UUID `json:"id"`
	Email    string      `json:"email"`
	FullName pgtype.Text `json:"full_name"`
}

// GetProfile returns the profile of the user identified by email.
//
// It backs the protected profile endpoint: Envoy validates the caller's JWT and
// forwards the verified identity, and this looks up the corresponding record.
func (p *ProfileService) GetProfile(ctx context.Context, email string) (UserProfile, error) {
	if p.Queries == nil {
		return UserProfile{}, ErrQueriesRequired
	}

	user, err := p.Queries.FindUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserProfile{}, ErrUserNotFound
	}
	if err != nil {
		return UserProfile{}, err
	}

	return UserProfile{
		ID:       user.ID,
		Email:    user.Email,
		FullName: user.FullName,
	}, nil
}
