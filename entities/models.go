package entities

import (
	"time"

	"github.com/google/uuid"
)

type Subtask struct {
	ID              uuid.UUID `json:"id"`
	Title           string    `json:"title"`
	Deadline        time.Time `json:"deadline"`
	IsCompleted     bool      `json:"is_completed"`
	Color           string    `json:"color"`
	GoalName        string    `json:"goal_name"`
	CalendarEventID *string   `json:"calendar_event_id"`
}

type Goal struct {
	ID                    uuid.UUID `json:"id"`
	UserID                int       `json:"user_id"`
	Title                 string    `json:"title"`
	StartTime             time.Time `json:"start_time"`
	Duration              string    `json:"duration"`
	Color                 string    `json:"color"`
	IsPinned              bool      `json:"is_pinned"`
	Subtasks              []Subtask `json:"subtasks"`
	CompletedSubtaskCount int       `json:"completed_subtask_count"`
}

type User struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Surname      string `json:"surname"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
}
