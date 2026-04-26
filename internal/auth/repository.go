package auth

import (
	"context"
	"fmt"
	"log"
	"time"

	"station-backend/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	GetStaffByCIN(ctx context.Context, cin string) (*models.Staff, error)
	UpdateLastLogin(ctx context.Context, staffID string) error
	// CRUD
	ListStaff(ctx context.Context) ([]models.Staff, error)
	GetStaffByID(ctx context.Context, id string) (*models.Staff, error)
	CreateStaff(ctx context.Context, s models.Staff) (*models.Staff, error)
	UpdateStaff(ctx context.Context, id string, s models.Staff) (*models.Staff, error)
	DeleteStaff(ctx context.Context, id string) error
}

type RepositoryImpl struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return RepositoryImpl{db: db}
}

func (r RepositoryImpl) GetStaffByCIN(ctx context.Context, cin string) (*models.Staff, error) {
	started := time.Now()
	query := `
		SELECT id, cin, phone_number, first_name, last_name, role, is_active, last_login, created_at, updated_at
		FROM staff 
		WHERE cin = $1 AND is_active = true
	`

	var staff models.Staff
	err := r.db.QueryRow(ctx, query, cin).Scan(
		&staff.ID,
		&staff.CIN,
		&staff.PhoneNumber,
		&staff.FirstName,
		&staff.LastName,
		&staff.Role,
		&staff.IsActive,
		&staff.LastLogin,
		&staff.CreatedAt,
		&staff.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			if shouldLogAuthTiming() {
				log.Printf("auth_timing stage=db_get_staff_by_cin status=not_found cin=%s duration_ms=%d", cin, time.Since(started).Milliseconds())
			}
			return nil, fmt.Errorf("staff with CIN %s not found", cin)
		}
		if shouldLogAuthTiming() {
			log.Printf("auth_timing stage=db_get_staff_by_cin status=error cin=%s duration_ms=%d err=%v", cin, time.Since(started).Milliseconds(), err)
		}
		return nil, fmt.Errorf("failed to get staff: %w", err)
	}
	if shouldLogAuthTiming() {
		log.Printf("auth_timing stage=db_get_staff_by_cin status=ok cin=%s staff_id=%s duration_ms=%d", cin, staff.ID, time.Since(started).Milliseconds())
	}

	return &staff, nil
}

func (r RepositoryImpl) UpdateLastLogin(ctx context.Context, staffID string) error {
	started := time.Now()
	query := `UPDATE staff SET last_login = NOW() WHERE id = $1`

	_, err := r.db.Exec(ctx, query, staffID)
	if err != nil {
		if shouldLogAuthTiming() {
			log.Printf("auth_timing stage=db_update_last_login status=error staff_id=%s duration_ms=%d err=%v", staffID, time.Since(started).Milliseconds(), err)
		}
		return fmt.Errorf("failed to update last login: %w", err)
	}
	if shouldLogAuthTiming() {
		log.Printf("auth_timing stage=db_update_last_login status=ok staff_id=%s duration_ms=%d", staffID, time.Since(started).Milliseconds())
	}

	return nil
}

func (r RepositoryImpl) ListStaff(ctx context.Context) ([]models.Staff, error) {
	rows, err := r.db.Query(ctx, `SELECT id, cin, phone_number, first_name, last_name, role, is_active, last_login, created_at, updated_at FROM staff ORDER BY last_login DESC NULLS LAST, first_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list staff: %w", err)
	}
	defer rows.Close()
	var list []models.Staff
	for rows.Next() {
		var s models.Staff
		if err := rows.Scan(&s.ID, &s.CIN, &s.PhoneNumber, &s.FirstName, &s.LastName, &s.Role, &s.IsActive, &s.LastLogin, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

func (r RepositoryImpl) GetStaffByID(ctx context.Context, id string) (*models.Staff, error) {
	row := r.db.QueryRow(ctx, `SELECT id, cin, phone_number, first_name, last_name, role, is_active, last_login, created_at, updated_at FROM staff WHERE id = $1`, id)
	var s models.Staff
	if err := row.Scan(&s.ID, &s.CIN, &s.PhoneNumber, &s.FirstName, &s.LastName, &s.Role, &s.IsActive, &s.LastLogin, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("staff not found")
		}
		return nil, err
	}
	return &s, nil
}

func (r RepositoryImpl) CreateStaff(ctx context.Context, s models.Staff) (*models.Staff, error) {
	row := r.db.QueryRow(ctx, `INSERT INTO staff (id, cin, phone_number, first_name, last_name, role, is_active) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, cin, phone_number, first_name, last_name, role, is_active, last_login, created_at, updated_at`, s.ID, s.CIN, s.PhoneNumber, s.FirstName, s.LastName, s.Role, s.IsActive)
	var out models.Staff
	if err := row.Scan(&out.ID, &out.CIN, &out.PhoneNumber, &out.FirstName, &out.LastName, &out.Role, &out.IsActive, &out.LastLogin, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r RepositoryImpl) UpdateStaff(ctx context.Context, id string, s models.Staff) (*models.Staff, error) {
	// Update selective fields using COALESCE; empty strings keep previous
	_, err := r.db.Exec(ctx, `UPDATE staff SET first_name = COALESCE(NULLIF($2,''), first_name), last_name = COALESCE(NULLIF($3,''), last_name), phone_number = COALESCE(NULLIF($4,''), phone_number), role = COALESCE(NULLIF($5,''), role), is_active = COALESCE($6, is_active), updated_at = NOW() WHERE id = $1`, id, s.FirstName, s.LastName, s.PhoneNumber, s.Role, s.IsActive)
	if err != nil {
		return nil, err
	}
	return r.GetStaffByID(ctx, id)
}

func (r RepositoryImpl) DeleteStaff(ctx context.Context, id string) error {
	// Soft delete: set is_active = false instead of hard delete
	// This preserves all records and prevents data loss
	ct, err := r.db.Exec(ctx, `UPDATE staff SET is_active = false, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("staff not found")
	}
	return nil
}
