package user

import (
	"context"

	userrepo "meetopoly-be/internal/repository/user"
)

// Profile is the public user shape for HTTP.
type Profile struct {
	ID              string
	Email           string
	Username        string
	Country         string
	EmailVerified   bool
	ProfileComplete bool
}

// Service reads user profiles.
type Service interface {
	GetByID(ctx context.Context, id string) (*Profile, error)
}

type service struct {
	users userrepo.Repository
}

// New builds a user Service.
func New(users userrepo.Repository) Service {
	return &service{users: users}
}

func (s *service) GetByID(ctx context.Context, id string) (*Profile, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return ToProfile(u), nil
}

// ToProfile maps a repository user to a public profile.
func ToProfile(u *userrepo.User) *Profile {
	return &Profile{
		ID:              u.ID,
		Email:           u.Email,
		Username:        u.Username,
		Country:         u.Country,
		EmailVerified:   u.EmailVerified,
		ProfileComplete: u.ProfileComplete(),
	}
}
