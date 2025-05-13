package repositories

import (
	"database/sql"
	"go-server/entities"

	"github.com/google/uuid"
)

type SubtaskRepository struct {
	db *sql.DB
}

func (r *SubtaskRepository) Begin() (*sql.Tx, error) {
	return r.db.Begin()
}

func NewSubtaskRepository(db *sql.DB) *SubtaskRepository {
	return &SubtaskRepository{db: db}
}

func (r *SubtaskRepository) Create(tx *sql.Tx, subtask *entities.Subtask, goalID uuid.UUID) error {
	_, err := tx.Exec(`
        INSERT INTO subtasks (id, goal_id, title, deadline, is_completed, color, goal_name, calendar_event_id)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		subtask.ID, goalID, subtask.Title, subtask.Deadline, subtask.IsCompleted, subtask.Color, subtask.GoalName, subtask.CalendarEventID)
	return err
}

func (r *SubtaskRepository) GetByGoalID(goalID uuid.UUID) ([]entities.Subtask, error) {
	rows, err := r.db.Query(`
        SELECT id, title, deadline, is_completed, color, goal_name, calendar_event_id
        FROM subtasks
        WHERE goal_id = $1`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subtasks []entities.Subtask
	for rows.Next() {
		var s entities.Subtask
		if err := rows.Scan(&s.ID, &s.Title, &s.Deadline, &s.IsCompleted, &s.Color, &s.GoalName, &s.CalendarEventID); err != nil {
			return nil, err
		}
		subtasks = append(subtasks, s)
	}
	return subtasks, nil
}

func (r *SubtaskRepository) GetAllByUserID(userID string) ([]entities.Subtask, error) {
	rows, err := r.db.Query(`
        SELECT s.id, s.title, s.deadline, s.is_completed, s.color, g.title, s.calendar_event_id
        FROM subtasks s
        INNER JOIN goals g ON s.goal_id = g.id
        WHERE g.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subtasks []entities.Subtask
	for rows.Next() {
		var s entities.Subtask
		if err := rows.Scan(&s.ID, &s.Title, &s.Deadline, &s.IsCompleted, &s.Color, &s.GoalName, &s.CalendarEventID); err != nil {
			return nil, err
		}
		subtasks = append(subtasks, s)
	}
	return subtasks, nil
}

func (r *SubtaskRepository) ToggleCompletion(subtaskID uuid.UUID, isCompleted bool) error {
	_, err := r.db.Exec("UPDATE subtasks SET is_completed = $1 WHERE id = $2", !isCompleted, subtaskID)
	return err
}

func (r *SubtaskRepository) DeleteByGoalID(tx *sql.Tx, goalID uuid.UUID) error {
	_, err := tx.Exec("DELETE FROM subtasks WHERE goal_id = $1", goalID)
	return err
}

func (r *SubtaskRepository) DeleteByUserID(tx *sql.Tx, userID int) error {
	_, err := tx.Exec(`
        DELETE FROM subtasks 
        WHERE goal_id IN (SELECT id FROM goals WHERE user_id = $1)`, userID)
	return err
}
