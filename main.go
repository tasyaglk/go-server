package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
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

func connectDB() *sql.DB {
	connStr := "user=postgres dbname=postgres password=mypassword host=localhost sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	// Создаём таблицы при подключении
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			surname TEXT NOT NULL,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS goals (
			id UUID PRIMARY KEY,
			user_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			start_time TIMESTAMP WITH TIME ZONE NOT NULL,
			duration TEXT NOT NULL,
			color TEXT NOT NULL,
			is_pinned BOOLEAN NOT NULL
		);

		CREATE TABLE IF NOT EXISTS subtasks (
			id UUID PRIMARY KEY,
			goal_id UUID REFERENCES goals(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			deadline TIMESTAMP WITH TIME ZONE NOT NULL,
			is_completed BOOLEAN NOT NULL DEFAULT false,
			color TEXT,
			goal_name TEXT,
			calendar_event_id TEXT
		);
	`)
	if err != nil {
		log.Fatal("Migration failed: ", err)
	}

	return db
}

// Обработчик удаления пользователя
func deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Получаем user_id из URL
	vars := mux.Vars(r)
	userIDStr := vars["id"]
	if userIDStr == "" {
		http.Error(w, "Missing user_id parameter", http.StatusBadRequest)
		return
	}

	// Проверяем, что user_id — число
	var userID int
	_, err := fmt.Sscanf(userIDStr, "%d", &userID)
	if err != nil {
		http.Error(w, "Invalid user_id format", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	// Начинаем транзакцию
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Failed to start transaction: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Удаляем подзадачи
	result, err := tx.Exec(`
		DELETE FROM subtasks 
		WHERE goal_id IN (SELECT id FROM goals WHERE user_id = $1)`, userID)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to delete subtasks for user %d: %v", userID, err)
		http.Error(w, "Failed to delete subtasks", http.StatusInternalServerError)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	log.Printf("Deleted %d subtasks for user %d", rowsAffected, userID)

	// Удаляем цели пользователя
	result, err = tx.Exec("DELETE FROM goals WHERE user_id = $1", userID)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to delete goals for user %d: %v", userID, err)
		http.Error(w, "Failed to delete goals", http.StatusInternalServerError)
		return
	}
	rowsAffected, _ = result.RowsAffected()
	log.Printf("Deleted %d goals for user %d", rowsAffected, userID)

	// Удаляем пользователя
	result, err = tx.Exec("DELETE FROM users WHERE id = $1", userID)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to delete user %d: %v", userID, err)
		http.Error(w, "Failed to delete user", http.StatusInternalServerError)
		return
	}

	// Проверяем, был ли удалён пользователь
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		tx.Rollback()
		log.Printf("Error checking user deletion for user %d: %v", userID, err)
		http.Error(w, "Error checking deletion", http.StatusInternalServerError)
		return
	}
	if rowsAffected == 0 {
		tx.Rollback()
		log.Printf("User %d not found", userID)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Подтверждаем транзакцию
	err = tx.Commit()
	if err != nil {
		log.Printf("Failed to commit transaction for user %d: %v", userID, err)
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully deleted user %d", userID)
	// Возвращаем ответ
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "User and associated data deleted successfully",
	})
}

// Остальные обработчики (без изменений)
func createGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var goal Goal
	if err := json.NewDecoder(r.Body).Decode(&goal); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Вставка основной цели
	goal.ID = uuid.New()
	_, err = tx.Exec(`
        INSERT INTO goals (id, user_id, title, start_time, duration, color, is_pinned)
        VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		goal.ID, goal.UserID, goal.Title, goal.StartTime, goal.Duration, goal.Color, goal.IsPinned)

	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Вставка подзадач
	for _, subtask := range goal.Subtasks {
		if subtask.Title == "" {
			continue
		}

		if subtask.ID == uuid.Nil {
			subtask.ID = uuid.New()
		}

		_, err = tx.Exec(`
			INSERT INTO subtasks (id, goal_id, title, deadline, is_completed, color, goal_name, calendar_event_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			subtask.ID, goal.ID, subtask.Title, subtask.Deadline, subtask.IsCompleted,
			subtask.Color, subtask.GoalName, subtask.CalendarEventID)

		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	tx.Commit()
	json.NewEncoder(w).Encode(goal)
}

func getGoalsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "Missing user_id parameter", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	// Получение целей
	rows, err := db.Query(`
		SELECT id, user_id, title, start_time, duration, color, is_pinned 
		FROM goals 
		WHERE user_id = $1`, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var goals []Goal
	for rows.Next() {
		var g Goal
		err := rows.Scan(
			&g.ID,
			&g.UserID,
			&g.Title,
			&g.StartTime,
			&g.Duration,
			&g.Color,
			&g.IsPinned,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Получение подзадач
		subtaskRows, err := db.Query(`
			SELECT id, title, deadline, is_completed, color, goal_name, calendar_event_id
			FROM subtasks
			WHERE goal_id = $1`, g.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var subtasks []Subtask
		completedCount := 0
		for subtaskRows.Next() {
			var s Subtask
			err := subtaskRows.Scan(&s.ID, &s.Title, &s.Deadline, &s.IsCompleted, &s.Color, &s.GoalName, &s.CalendarEventID)

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if s.IsCompleted {
				completedCount++
			}
			subtasks = append(subtasks, s)
		}
		subtaskRows.Close()

		g.Subtasks = subtasks
		g.CompletedSubtaskCount = completedCount
		goals = append(goals, g)
	}

	if len(goals) == 0 {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(goals)
}

func updateGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var goal Goal
	if err := json.NewDecoder(r.Body).Decode(&goal); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Обновление основной цели
	_, err = tx.Exec(`
		UPDATE goals 
		SET title = $1, start_time = $2, duration = $3, 
			color = $4, is_pinned = $5 
		WHERE id = $6 AND user_id = $7`,
		goal.Title, goal.StartTime, goal.Duration,
		goal.Color, goal.IsPinned, goal.ID, goal.UserID)

	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Удаление старых подзадач
	_, err = tx.Exec("DELETE FROM subtasks WHERE goal_id = $1", goal.ID)
	if err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Вставка новых подзадач
	for _, subtask := range goal.Subtasks {
		if subtask.Title == "" {
			continue
		}

		if subtask.ID == uuid.Nil {
			subtask.ID = uuid.New()
		}

		_, err = tx.Exec(`
			INSERT INTO subtasks (id, goal_id, title, deadline, is_completed, color, goal_name, calendar_event_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			subtask.ID, goal.ID, subtask.Title, subtask.Deadline, subtask.IsCompleted,
			subtask.Color, subtask.GoalName, subtask.CalendarEventID)

		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	tx.Commit()
	json.NewEncoder(w).Encode(goal)
}

func deleteGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	vars := mux.Vars(r)
	goalID, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "Invalid goal ID", http.StatusBadRequest)
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "Missing user_id parameter", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	_, err = db.Exec(`
        DELETE FROM goals 
        WHERE id = $1 AND user_id = $2`,
		goalID, userID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Goal and related subtasks deleted successfully",
	})
}

func getAllSubtasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	userIDStr := r.URL.Query().Get("userId")
	if userIDStr == "" {
		http.Error(w, "Missing userId parameter", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	rows, err := db.Query(`
		SELECT s.id, s.title, s.deadline, s.is_completed, s.color, g.title, s.calendar_event_id
		FROM subtasks s
		INNER JOIN goals g ON s.goal_id = g.id
		WHERE g.user_id = $1
	`, userIDStr)

	if err != nil {
		http.Error(w, "Failed to fetch subtasks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var subtasks []Subtask
	for rows.Next() {
		var s Subtask
		err := rows.Scan(&s.ID, &s.Title, &s.Deadline, &s.IsCompleted, &s.Color, &s.GoalName, &s.CalendarEventID)

		if err != nil {
			http.Error(w, "Error scanning subtask", http.StatusInternalServerError)
			return
		}
		subtasks = append(subtasks, s)
	}

	if subtasks == nil {
		subtasks = []Subtask{}
	}

	json.NewEncoder(w).Encode(subtasks)
}

func toggleSubtaskCompletionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	vars := mux.Vars(r)
	subtaskID, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "Invalid subtask ID", http.StatusBadRequest)
		return
	}

	var subtask Subtask
	if err := json.NewDecoder(r.Body).Decode(&subtask); err != nil {
		http.Error(w, "Invalid subtask data", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	_, err = db.Exec(`
		UPDATE subtasks
		SET is_completed = $1
		WHERE id = $2`, !subtask.IsCompleted, subtaskID)

	if err != nil {
		http.Error(w, "Failed to update subtask", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Subtask updated successfully"})
}

func getSubtasksByGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	vars := mux.Vars(r)
	goalIDStr := vars["goal_id"]
	goalID, err := uuid.Parse(goalIDStr)
	if err != nil {
		http.Error(w, "Invalid goal ID", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, title, deadline, is_completed, color, goal_name, calendar_event_id
		FROM subtasks
		WHERE goal_id = $1`, goalID)
	if err != nil {
		http.Error(w, "Failed to fetch subtasks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var subtasks []Subtask
	for rows.Next() {
		var s Subtask
		err := rows.Scan(&s.ID, &s.Title, &s.Deadline, &s.IsCompleted, &s.Color, &s.GoalName, &s.CalendarEventID)

		if err != nil {
			http.Error(w, "Error scanning subtask", http.StatusInternalServerError)
			return
		}
		subtasks = append(subtasks, s)
	}

	if subtasks == nil {
		subtasks = []Subtask{}
	}

	json.NewEncoder(w).Encode(subtasks)
}

func getUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	db := connectDB()
	defer db.Close()

	rows, err := db.Query("SELECT id, name, surname, email FROM users")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		err := rows.Scan(&u.ID, &u.Name, &u.Surname, &u.Email)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	json.NewEncoder(w).Encode(users)
}

func createUserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	var newUser User
	err := json.NewDecoder(r.Body).Decode(&newUser)
	if err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	sqlStatement := `
	INSERT INTO users (name, surname, email)
	VALUES ($1, $2, $3)
	RETURNING id`

	err = db.QueryRow(sqlStatement, newUser.Name, newUser.Surname, newUser.Email).Scan(&newUser.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(newUser)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var newUser struct {
		Name     string `json:"name"`
		Surname  string `json:"surname"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	err := json.NewDecoder(r.Body).Decode(&newUser)
	if err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newUser.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	db := connectDB()
	defer db.Close()

	sqlStatement := `
    INSERT INTO users (name, surname, email, password_hash)
    VALUES ($1, $2, $3, $4)
    RETURNING id`

	var userID int
	err = db.QueryRow(sqlStatement, newUser.Name, newUser.Surname, newUser.Email, string(hashedPassword)).Scan(&userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]int{"id": userID})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var credentials struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	err := json.NewDecoder(r.Body).Decode(&credentials)
	if err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	var user User
	err = db.QueryRow("SELECT id, name, surname, email, password_hash FROM users WHERE email = $1", credentials.Email).Scan(
		&user.ID, &user.Name, &user.Surname, &user.Email, &user.PasswordHash)
	if err != nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(credentials.Password))
	if err != nil {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(user)
}

func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var request struct {
		Email       string `json:"email"`
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	db := connectDB()
	defer db.Close()

	var passwordHash string
	err = db.QueryRow("SELECT password_hash FROM users WHERE email = $1", request.Email).Scan(&passwordHash)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusUnauthorized)
		} else {
			http.Error(w, "Server error", http.StatusInternalServerError)
		}
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(request.OldPassword))
	if err != nil {
		http.Error(w, "Invalid old password", http.StatusUnauthorized)
		return
	}

	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec("UPDATE users SET password_hash = $1 WHERE email = $2", newHashedPassword, request.Email)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Password updated successfully"})
}

// reformulateGoalHandler handles goal reformulation
func reformulateGoalHandler(deepSeek *DeepSeekManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var request struct {
			Goal string `json:"goal"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid data", http.StatusBadRequest)
			return
		}

		result, err := deepSeek.ReformulateGoal(request.Goal)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"result": result})
	}
}

// validateGoalHandler validates a goal's moral and social acceptability
func validateGoalHandler(deepSeek *DeepSeekManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var request struct {
			Goal string `json:"goal"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid data", http.StatusBadRequest)
			return
		}

		result, err := deepSeek.ValidateGoal(request.Goal)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"is_valid": result})
	}
}

// validatePlanFeedbackHandler checks if feedback relates to the goal's plan
func validatePlanFeedbackHandler(deepSeek *DeepSeekManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var request struct {
			Feedback string `json:"feedback"`
			Goal     string `json:"goal"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid data", http.StatusBadRequest)
			return
		}

		result, err := deepSeek.ValidatePlanFeedback(request.Feedback, request.Goal)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"is_valid": result})
	}
}

// validateScheduleFeedbackHandler checks if feedback relates to scheduling
func validateScheduleFeedbackHandler(deepSeek *DeepSeekManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var request struct {
			Feedback string `json:"feedback"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid data", http.StatusBadRequest)
			return
		}

		result, err := deepSeek.ValidateScheduleFeedback(request.Feedback)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"is_valid": result})
	}
}

// generateStepsHandler generates steps to achieve a goal
func generateStepsHandler(deepSeek *DeepSeekManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var request struct {
			Goal      string `json:"goal"`
			Knowledge string `json:"knowledge"`
			Feedback  string `json:"feedback"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid data", http.StatusBadRequest)
			return
		}

		result, err := deepSeek.GenerateSteps(request.Goal, request.Knowledge, request.Feedback)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string][]string{"steps": result})
	}
}

// generateScheduleHandler creates a schedule for given steps
func generateScheduleHandler(deepSeek *DeepSeekManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		var request struct {
			Steps        []string    `json:"steps"`
			Availability string      `json:"availability"`
			Frequency    string      `json:"frequency"`
			Feedback     string      `json:"feedback"`
			BusySlots    [][2]string `json:"busy_slots"` // Expecting [start, end] in RFC3339
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid data", http.StatusBadRequest)
			return
		}

		// Parse busy slots
		var busySlots [][2]time.Time
		for _, slot := range request.BusySlots {
			start, err := time.Parse(time.RFC3339, slot[0])
			if err != nil {
				http.Error(w, "Invalid start time format", http.StatusBadRequest)
				return
			}
			end, err := time.Parse(time.RFC3339, slot[1])
			if err != nil {
				http.Error(w, "Invalid end time format", http.StatusBadRequest)
				return
			}
			busySlots = append(busySlots, [2]time.Time{start, end})
		}

		result, err := deepSeek.GenerateSchedule(request.Steps, request.Availability, request.Frequency, request.Feedback, busySlots)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"schedule": result})
	}
}

func main() {
	router := mux.NewRouter()
	deepSeekManager := NewDeepSeekManager()

	// CORS middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	})

	// Регистрация обработчиков
	router.HandleFunc("/goals", createGoalHandler).Methods("POST")
	router.HandleFunc("/goals", getGoalsHandler).Methods("GET")
	router.HandleFunc("/goals/{id}", updateGoalHandler).Methods("PUT")
	router.HandleFunc("/goals/{id}", deleteGoalHandler).Methods("DELETE")
	router.HandleFunc("/subtasks", getAllSubtasksHandler).Methods("GET")
	router.HandleFunc("/subtasks/{id}/complete", toggleSubtaskCompletionHandler).Methods("PUT")
	router.HandleFunc("/goals/{goal_id}/subtasks", getSubtasksByGoalHandler).Methods("GET")

	// Пользователи
	router.HandleFunc("/users", getUsersHandler).Methods("GET")
	router.HandleFunc("/users", createUserHandler).Methods("POST")
	router.HandleFunc("/users/{id}", deleteUserHandler).Methods("DELETE")
	router.HandleFunc("/register", registerHandler).Methods("POST")
	router.HandleFunc("/login", loginHandler).Methods("POST")
	router.HandleFunc("/change-password", changePasswordHandler).Methods("POST")

	router.HandleFunc("/deepseek/reformulate-goal", reformulateGoalHandler(deepSeekManager)).Methods("POST")
	router.HandleFunc("/deepseek/validate-goal", validateGoalHandler(deepSeekManager)).Methods("POST")
	router.HandleFunc("/deepseek/validate-plan-feedback", validatePlanFeedbackHandler(deepSeekManager)).Methods("POST")
	router.HandleFunc("/deepseek/validate-schedule-feedback", validateScheduleFeedbackHandler(deepSeekManager)).Methods("POST")
	router.HandleFunc("/deepseek/generate-steps", generateStepsHandler(deepSeekManager)).Methods("POST")
	router.HandleFunc("/deepseek/generate-schedule", generateScheduleHandler(deepSeekManager)).Methods("POST")

	fmt.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}
