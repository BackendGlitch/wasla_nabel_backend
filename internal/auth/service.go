package auth

import (
	"context"
	"fmt"
	"os"
	"time"

	"station-backend/internal/models"
	"station-backend/pkg/utils"

	"github.com/redis/go-redis/v9"
)

type Service struct {
	repo  Repository
	redis *redis.Client
}

func NewService(repo Repository, redis *redis.Client) *Service {
	return &Service{
		repo:  repo,
		redis: redis,
	}
}

// CRUD wrappers
func (s *Service) ListStaff(ctx context.Context) ([]models.Staff, error) {
	return s.repo.ListStaff(ctx)
}

func (s *Service) GetStaffByID(ctx context.Context, id string) (*models.Staff, error) {
	return s.repo.GetStaffByID(ctx, id)
}

func (s *Service) CreateStaff(ctx context.Context, input models.Staff) (*models.Staff, error) {
	if input.CIN == "" || len(input.CIN) != 8 {
		return nil, fmt.Errorf("invalid CIN")
	}
	if input.FirstName == "" || input.LastName == "" {
		return nil, fmt.Errorf("missing name")
	}
	if input.Role == "" {
		input.Role = "WORKER"
	}
	if input.ID == "" {
		input.ID = fmt.Sprintf("staff_%d", time.Now().UnixNano())
	}
	input.IsActive = true
	return s.repo.CreateStaff(ctx, input)
}

func (s *Service) UpdateStaff(ctx context.Context, id string, input models.Staff) (*models.Staff, error) {
	return s.repo.UpdateStaff(ctx, id, input)
}

func (s *Service) DeleteStaff(ctx context.Context, id string) error {
	return s.repo.DeleteStaff(ctx, id)
}

func (s *Service) ValidateStaff(ctx context.Context, cin string) (*models.Staff, error) {
	staff, err := s.repo.GetStaffByCIN(ctx, cin)
	if err != nil {
		return nil, fmt.Errorf("staff not found: %w", err)
	}

	if !staff.IsActive {
		return nil, fmt.Errorf("staff account is inactive")
	}

	// Keep last_login best-effort and out of the login critical path.
	go func(staffID string) {
		updateCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.repo.UpdateLastLogin(updateCtx, staffID)
	}(staff.ID)

	return staff, nil
}

func (s *Service) GenerateToken(staff *models.Staff) (string, error) {
	secretKey := os.Getenv("JWT_SECRET_KEY")
	if secretKey == "" {
		secretKey = "your-secret-key-change-this-in-production"
	}

	return utils.GenerateJWT(staff.ID, staff.FirstName, staff.LastName, secretKey)
}

func (s *Service) StoreSession(ctx context.Context, staffID, token string) error {
	// Store session in Redis with 24 hour expiration
	expiration := 24 * time.Hour
	return s.redis.Set(ctx, fmt.Sprintf("session:%s", staffID), token, expiration).Err()
}

func (s *Service) ValidateSession(staffID string) (bool, error) {
	ctx := context.Background()

	_, err := s.redis.Get(ctx, fmt.Sprintf("session:%s", staffID)).Result()
	if err == redis.Nil {
		return false, nil // Session not found
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (s *Service) Logout(staffID string) error {
	ctx := context.Background()

	return s.redis.Del(ctx, fmt.Sprintf("session:%s", staffID)).Err()
}
