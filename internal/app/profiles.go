// Package app holds the services the frontend calls. They stay thin: the work
// happens in the other internal packages, which know nothing about the UI.
package app

import (
	"errors"
	"strings"

	"github.com/diablo2org/launcher/internal/spec"
)

// ProfileService lets the frontend check server profiles.
type ProfileService struct {
	example string
}

// NewProfileService returns a service that offers example as a starting point.
func NewProfileService(example []byte) *ProfileService {
	return &ProfileService{example: string(example)}
}

// CheckResult is what the frontend shows after checking a profile.
type CheckResult struct {
	OK       bool     `json:"ok"`
	Name     string   `json:"name"`
	Problems []string `json:"problems"`
}

// Check parses a profile and reports every problem found.
func (s *ProfileService) Check(profile string) CheckResult {
	p, err := spec.ParseProfile([]byte(profile))
	if err == nil {
		return CheckResult{OK: true, Name: p.Name, Problems: []string{}}
	}

	var problems spec.Problems
	if errors.As(err, &problems) {
		return CheckResult{Problems: problems}
	}

	return CheckResult{Problems: strings.Split(strings.TrimSpace(err.Error()), "\n")}
}

// Example returns the example SlashDiablo profile.
func (s *ProfileService) Example() string {
	return s.example
}
