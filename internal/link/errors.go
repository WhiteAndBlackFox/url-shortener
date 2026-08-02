package link

import "errors"

var (
	// ErrNotFound is returned when a link with the given code does not exist.
	ErrNotFound = errors.New("link: not found")
	// ErrInvalidURL is returned when the submitted long URL fails validation.
	ErrInvalidURL = errors.New("link: invalid url")
	// ErrCodeConflict is returned by a Repository when the generated code already exists.
	ErrCodeConflict = errors.New("link: code conflict")
)
