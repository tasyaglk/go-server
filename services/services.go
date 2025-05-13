package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"go-server/deepseek"
	"go-server/entities"
	"go-server/repositories"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

type Services struct {
	userRepo    *repositories.UserRepository
	goalRepo    *repositories.GoalRepository
	subtaskRepo *repositories.SubtaskRepository
	deepSeek    *deepseek.DeepSeekManager
}

func NewServices(userRepo *repositories.UserRepository, goalRepo *repositories.GoalRepository, subtaskRepo *repositories.SubtaskRepository, deepSeek *deepseek.DeepSeekManager) *Services {
	return &Services{
		userRepo:    userRepo,
		goalRepo:    goalRepo,
		subtaskRepo: subtaskRepo,
		deepSeek:    deepSeek,
	}
}

func (s *Services) CreateGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var goal entities.Goal
	if err := json.NewDecoder(r.Body).Decode(&goal); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	tx, err := s.userRepo.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	goal.ID = uuid.New()
	if err := s.goalRepo.Create(tx, &goal); err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, subtask := range goal.Subtasks {
		if subtask.Title == "" {
			continue
		}
		if subtask.ID == uuid.Nil {
			subtask.ID = uuid.New()
		}
		if err := s.subtaskRepo.Create(tx, &subtask, goal.ID); err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(goal)
}

func (s *Services) GetGoalsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "Missing user_id parameter", http.StatusBadRequest)
		return
	}

	goals, err := s.goalRepo.GetByUserID(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for i, g := range goals {
		subtasks, err := s.subtaskRepo.GetByGoalID(g.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		completedCount := 0
		for _, s := range subtasks {
			if s.IsCompleted {
				completedCount++
			}
		}
		goals[i].Subtasks = subtasks
		goals[i].CompletedSubtaskCount = completedCount
	}

	if len(goals) == 0 {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(goals)
}

func (s *Services) UpdateGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var goal entities.Goal
	if err := json.NewDecoder(r.Body).Decode(&goal); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	tx, err := s.userRepo.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if err := s.goalRepo.Update(tx, &goal); err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := s.subtaskRepo.DeleteByGoalID(tx, goal.ID); err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, subtask := range goal.Subtasks {
		if subtask.Title == "" {
			continue
		}
		if subtask.ID == uuid.Nil {
			subtask.ID = uuid.New()
		}
		if err := s.subtaskRepo.Create(tx, &subtask, goal.ID); err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(goal)
}

func (s *Services) DeleteGoalHandler(w http.ResponseWriter, r *http.Request) {
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

	tx, err := s.userRepo.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if err := s.goalRepo.Delete(tx, goalID, userID); err != nil {
		tx.Rollback()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Goal and related subtasks deleted successfully",
	})
}

func (s *Services) GetAllSubtasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	userIDStr := r.URL.Query().Get("userId")
	if userIDStr == "" {
		http.Error(w, "Missing userId parameter", http.StatusBadRequest)
		return
	}

	subtasks, err := s.subtaskRepo.GetAllByUserID(userIDStr)
	if err != nil {
		http.Error(w, "Failed to fetch subtasks", http.StatusInternalServerError)
		return
	}

	if subtasks == nil {
		subtasks = []entities.Subtask{}
	}

	json.NewEncoder(w).Encode(subtasks)
}

func (s *Services) ToggleSubtaskCompletionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	vars := mux.Vars(r)
	subtaskID, err := uuid.Parse(vars["id"])
	if err != nil {
		http.Error(w, "Invalid subtask ID", http.StatusBadRequest)
		return
	}

	var subtask entities.Subtask
	if err := json.NewDecoder(r.Body).Decode(&subtask); err != nil {
		http.Error(w, "Invalid subtask data", http.StatusBadRequest)
		return
	}

	if err := s.subtaskRepo.ToggleCompletion(subtaskID, subtask.IsCompleted); err != nil {
		http.Error(w, "Failed to update subtask", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Subtask updated successfully"})
}

func (s *Services) GetSubtasksByGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	vars := mux.Vars(r)
	goalIDStr := vars["goal_id"]
	goalID, err := uuid.Parse(goalIDStr)
	if err != nil {
		http.Error(w, "Invalid goal ID", http.StatusBadRequest)
		return
	}

	subtasks, err := s.subtaskRepo.GetByGoalID(goalID)
	if err != nil {
		http.Error(w, "Failed to fetch subtasks", http.StatusInternalServerError)
		return
	}

	if subtasks == nil {
		subtasks = []entities.Subtask{}
	}

	json.NewEncoder(w).Encode(subtasks)
}

func (s *Services) GetUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	users, err := s.userRepo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(users)
}

func (s *Services) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	var newUser entities.User
	if err := json.NewDecoder(r.Body).Decode(&newUser); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	if err := s.userRepo.Create(&newUser); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(newUser)
}

func (s *Services) DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	vars := mux.Vars(r)
	userIDStr := vars["id"]
	var userID int
	_, err := fmt.Sscanf(userIDStr, "%d", &userID)
	if err != nil {
		http.Error(w, "Invalid user_id format", http.StatusBadRequest)
		return
	}

	tx, err := s.userRepo.Begin()
	if err != nil {
		log.Printf("Failed to start transaction: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if err := s.subtaskRepo.DeleteByUserID(tx, userID); err != nil {
		tx.Rollback()
		log.Printf("Failed to delete subtasks for user %d: %v", userID, err)
		http.Error(w, "Failed to delete subtasks", http.StatusInternalServerError)
		return
	}

	if err := s.goalRepo.DeleteByUserID(tx, userID); err != nil {
		tx.Rollback()
		log.Printf("Failed to delete goals for user %d: %v", userID, err)
		http.Error(w, "Failed to delete goals", http.StatusInternalServerError)
		return
	}

	result, err := s.userRepo.Delete(userID)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to delete user %d: %v", userID, err)
		http.Error(w, "Failed to delete user", http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
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

	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction for user %d: %v", userID, err)
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully deleted user %d", userID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "User and associated data deleted successfully",
	})
}

func (s *Services) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var newUser struct {
		Name     string `json:"name"`
		Surname  string `json:"surname"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&newUser); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newUser.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	user := &entities.User{
		Name:         newUser.Name,
		Surname:      newUser.Surname,
		Email:        newUser.Email,
		PasswordHash: string(hashedPassword),
	}

	if err := s.userRepo.Create(user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]int{"id": user.ID})
}

func (s *Services) LoginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var credentials struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	user, err := s.userRepo.GetByEmail(credentials.Email)
	if err != nil {
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(credentials.Password)); err != nil {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	json.NewEncoder(w).Encode(user)
}

func (s *Services) ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var request struct {
		Email       string `json:"email"`
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	user, err := s.userRepo.GetByEmail(request.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusUnauthorized)
		} else {
			http.Error(w, "Server error", http.StatusInternalServerError)
		}
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.OldPassword)); err != nil {
		http.Error(w, "Invalid old password", http.StatusUnauthorized)
		return
	}

	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	if err := s.userRepo.UpdatePassword(request.Email, string(newHashedPassword)); err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Password updated successfully"})
}

func (s *Services) ReformulateGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var request struct {
		Goal string `json:"goal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	result, err := s.deepSeek.ReformulateGoal(request.Goal)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"result": result})
}

func (s *Services) ValidateGoalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var request struct {
		Goal string `json:"goal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	result, err := s.deepSeek.ValidateGoal(request.Goal)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]bool{"is_valid": result})
}

func (s *Services) ValidatePlanFeedbackHandler(w http.ResponseWriter, r *http.Request) {
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

	result, err := s.deepSeek.ValidatePlanFeedback(request.Feedback, request.Goal)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]bool{"is_valid": result})
}

func (s *Services) ValidateScheduleFeedbackHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var request struct {
		Feedback string `json:"feedback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

	result, err := s.deepSeek.ValidateScheduleFeedback(request.Feedback)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]bool{"is_valid": result})
}

func (s *Services) GenerateStepsHandler(w http.ResponseWriter, r *http.Request) {
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

	result, err := s.deepSeek.GenerateSteps(request.Goal, request.Knowledge, request.Feedback)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string][]string{"steps": result})
}

func (s *Services) GenerateScheduleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var request struct {
		Steps        []string    `json:"steps"`
		Availability string      `json:"availability"`
		Frequency    string      `json:"frequency"`
		Feedback     string      `json:"feedback"`
		BusySlots    [][2]string `json:"busy_slots"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid data", http.StatusBadRequest)
		return
	}

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

	result, err := s.deepSeek.GenerateSchedule(request.Steps, request.Availability, request.Frequency, request.Feedback, busySlots)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"schedule": result})
}
