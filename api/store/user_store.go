// Package store keeps off-chain household profiles (display data only).
// Credits, readings and bills live exclusively on the ledger; personal data stays
// off-chain, following the LGPD privacy-by-design principle of the white paper.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// User is a household profile. ID is the on-chain wallet identifier.
type User struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	Type         string `json:"type"` // prosumer | consumer
	Substation   string `json:"substation"`
	ConsumerUnit string `json:"consumerUnit"` // utility account number (UC)
}

// UserStore persists users in a JSON file.
type UserStore struct {
	filePath string
	mu       sync.RWMutex
}

// NewUserStore creates the store and the JSON file if needed.
func NewUserStore(filePath string) (*UserStore, error) {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(filePath, []byte("[]"), 0o644); err != nil {
			return nil, err
		}
	}
	return &UserStore{filePath: filePath}, nil
}

// List returns every user.
func (s *UserStore) List() ([]User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.read()
}

// Create adds a user.
func (s *UserStore) Create(user User) error {
	if user.ID == "" || user.DisplayName == "" {
		return errors.New("user ID and display name are required")
	}
	if user.Type != "prosumer" && user.Type != "consumer" {
		return errors.New("type must be 'prosumer' or 'consumer'")
	}
	if user.Substation == "" {
		return errors.New("substation is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.read()
	if err != nil {
		return err
	}
	for _, existing := range current {
		if existing.ID == user.ID {
			return errors.New("user already exists")
		}
	}
	current = append(current, user)
	encoded, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, encoded, 0o644)
}

func (s *UserStore) read() ([]User, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, err
	}
	result := []User{}
	if len(data) == 0 {
		return result, nil
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
