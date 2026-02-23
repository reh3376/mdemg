// Go Parser Test Fixture
// Tests all symbol extraction capabilities following canonical patterns

package testdata

import (
	"context"
	"time"
)

// === Pattern 1: Constants ===
// Line 13-16
const MaxRetries = 3
const DefaultTimeout = 30 * time.Second
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)

// === Pattern 7: Type Aliases ===
// Line 21-22
type UserID string
type ItemList []Item

// === Pattern 4: Interfaces ===
// Line 25-30
type UserRepository interface {
	FindByID(ctx context.Context, id UserID) (*User, error)
	Create(ctx context.Context, user *User) error
	Delete(ctx context.Context, id UserID) error
}

// === Pattern 3: Structs ===
// Line 33-39
type User struct {
	ID        UserID    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// === Pattern 3 continued: Service struct ===
// Line 43-46
type UserService struct {
	repo   UserRepository
	logger Logger
}

// === Pattern 6: Methods (receiver functions) ===
// Line 50-52
func (s *UserService) FindByID(ctx context.Context, id UserID) (*User, error) {
	return s.repo.FindByID(ctx, id)
}

// Line 55-57
func (s *UserService) Create(ctx context.Context, user *User) error {
	return s.repo.Create(ctx, user)
}

// Line 60-62
func (s *UserService) Delete(ctx context.Context, id UserID) error {
	return s.repo.Delete(ctx, id)
}

// === Pattern 2: Standalone Functions ===
// Line 65-67
func ValidateEmail(email string) bool {
	return len(email) > 0
}

// Line 70-72
func FormatUser(user *User) string {
	return user.Name + " <" + user.Email + ">"
}

// Line 75-81
func NewUserService(repo UserRepository, logger Logger) *UserService {
	return &UserService{
		repo:   repo,
		logger: logger,
	}
}

// === Supporting types ===
type Item struct {
	ID   string
	Name string
}

type Logger interface {
	Info(msg string)
	Error(msg string)
}
