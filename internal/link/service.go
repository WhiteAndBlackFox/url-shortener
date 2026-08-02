package link

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

const maxCodeGenerationAttempts = 5

// Service contains the business logic for creating and resolving short links.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateLink validates longURL, generates a unique short code and persists the link.
func (s *Service) CreateLink(ctx context.Context, longURL string) (*Link, error) {
	if err := validateLongURL(longURL); err != nil {
		return nil, err
	}

	for attempt := 0; attempt < maxCodeGenerationAttempts; attempt++ {
		code, err := generateCode()
		if err != nil {
			return nil, fmt.Errorf("link: generate code: %w", err)
		}

		l := &Link{Code: code, LongURL: longURL}
		if err := s.repo.Create(ctx, l); err != nil {
			if errors.Is(err, ErrCodeConflict) {
				continue
			}
			return nil, err
		}
		return l, nil
	}

	return nil, fmt.Errorf("link: could not generate unique code after %d attempts", maxCodeGenerationAttempts)
}

// ResolveCode looks up the long URL behind a short code.
func (s *Service) ResolveCode(ctx context.Context, code string) (*Link, error) {
	return s.repo.GetByCode(ctx, code)
}

func validateLongURL(raw string) error {
	if raw == "" {
		return ErrInvalidURL
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	if u.Host == "" {
		return ErrInvalidURL
	}
	return nil
}
