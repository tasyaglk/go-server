package repositories

import (
	"database/sql"
	"fmt"
	"go-server/entities"

	"github.com/google/uuid"
)

type GoalRepository struct {
	db *sql.DB
}

func NewGoalRepository(db *sql.DB) *GoalRepository {
	return &GoalRepository{db: db}
}

func (r *GoalRepository) Create(tx *sql.Tx, goal *entities.Goal) error {
	result, err := tx.Exec(`
        INSERT INTO goals (id, user_id, title, start_time, duration, color, is_pinned)
        VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		goal.ID, goal.UserID, goal.Title, goal.StartTime, goal.Duration, goal.Color, goal.IsPinned)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("failed to create goal")
	}
	return nil
}

func (r *GoalRepository) GetByUserID(userID string) ([]entities.Goal, error) {
	var userIDInt int
	if _, err := fmt.Sscanf(userID, "%d", &userIDInt); err != nil {
		return nil, fmt.Errorf("invalid user_id format: %w", err)
	}
	rows, err := r.db.Query("SELECT id, user_id, title, start_time, duration, color, is_pinned FROM goals WHERE user_id = $1", userIDInt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var goals []entities.Goal
	for rows.Next() {
		var g entities.Goal
		if err := rows.Scan(&g.ID, &g.UserID, &g.Title, &g.StartTime, &g.Duration, &g.Color, &g.IsPinned); err != nil {
			return nil, err
		}
		goals = append(goals, g)
	}
	return goals, nil
}

func (r *GoalRepository) Update(tx *sql.Tx, goal *entities.Goal) error {
	result, err := tx.Exec(`
        UPDATE goals SET title = $2, start_time = $3, duration = $4, color = $5, is_pinned = $6
        WHERE id = $1`,
		goal.ID, goal.Title, goal.StartTime, goal.Duration, goal.Color, goal.IsPinned)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("goal not found")
	}
	return nil
}

func (r *GoalRepository) Delete(tx *sql.Tx, goalID uuid.UUID, userID string) error {
	var userIDInt int
	if _, err := fmt.Sscanf(userID, "%d", &userIDInt); err != nil {
		return fmt.Errorf("invalid user_id format: %w", err)
	}
	result, err := tx.Exec("DELETE FROM goals WHERE id = $1 AND user_id = $2", goalID, userIDInt)
	if err != nil {
		return fmt.Errorf("failed to delete goal: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking deletion: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("goal not found or user_id does not match")
	}
	return nil
}

func (r *GoalRepository) DeleteByUserID(tx *sql.Tx, userID int) error {
	_, err := tx.Exec("DELETE FROM goals WHERE user_id = $1", userID)
	return err
}

func (r *GoalRepository) Begin() (*sql.Tx, error) {
	return r.db.Begin()
}
