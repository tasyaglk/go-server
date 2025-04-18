package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Subtask struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Deadline    time.Time `json:"deadline"`
	IsCompleted bool      `json:"is_completed"`
	Color       string    `json:"color"`
	GoalName    string    `json:"goal_name"`
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
			is_completed BOOLEAN NOT NULL DEFAULT false
		);
	`)
	if err != nil {
		log.Fatal("Migration failed: ", err)
	}

	return db
}

// Обработчики для целей с поддержкой подзадач
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
			INSERT INTO subtasks (id, goal_id, title, deadline, is_completed, color, goal_name)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			subtask.ID, goal.ID, subtask.Title, subtask.Deadline, subtask.IsCompleted, subtask.Color, subtask.GoalName)

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
			SELECT id, title, deadline, is_completed, color, goal_name
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
			err := subtaskRows.Scan(&s.ID, &s.Title, &s.Deadline, &s.IsCompleted, &s.Color, &s.GoalName)
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

	json.NewEncoder(w).Encode(goals)
}

// Обновлённый обработчик для целей
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
			INSERT INTO subtasks (id, goal_id, title, deadline, is_completed, color, goal_name)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			subtask.ID, goal.ID, subtask.Title, subtask.Deadline, subtask.IsCompleted, subtask.Color, subtask.GoalName)

		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	tx.Commit()
	json.NewEncoder(w).Encode(goal)
}

// Delete goal handler (обновлённая версия)
func deleteGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Получаем параметры из URL
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

	// Удаление цели (подзадачи удалятся каскадно благодаря FOREIGN KEY)
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

	db := connectDB()
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, title, deadline, is_completed, color, goal_name
		FROM subtasks`)
	if err != nil {
		http.Error(w, "Failed to fetch subtasks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var subtasks []Subtask
	for rows.Next() {
		var s Subtask
		// var goalID uuid.UUID // если вдруг нужно будет вернуть goal_id тоже

		err := rows.Scan(&s.ID, &s.Title, &s.Deadline, &s.IsCompleted, &s.Color, &s.GoalName)
		if err != nil {
			http.Error(w, "Error scanning subtask", http.StatusInternalServerError)
			return
		}
		subtasks = append(subtasks, s)
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

func main() {
	router := mux.NewRouter()

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

	// Обработчики пользователей
	router.HandleFunc("/users", getUsersHandler).Methods("GET")
	router.HandleFunc("/users", createUserHandler).Methods("POST")
	router.HandleFunc("/register", registerHandler).Methods("POST")
	router.HandleFunc("/login", loginHandler).Methods("POST")
	router.HandleFunc("/change-password", changePasswordHandler).Methods("POST")

	fmt.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}
