package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"go-server/deepseek"
	"go-server/repositories"
	"go-server/services"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

// connectDB initializes the database connection and runs migrations.
func connectDB() *sql.DB {
	connStr := "user=postgres dbname=postgres password=mypassword host=localhost sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	// Run migrations
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

// Router sets up the HTTP routes and handlers.
type Router struct {
	services *services.Services
}

// NewRouter creates a new Router instance.
func NewRouter(services *services.Services) *Router {
	return &Router{services: services}
}

// SetupRoutes configures the HTTP routes.
func (r *Router) SetupRoutes() *mux.Router {
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

	// Goal routes
	router.HandleFunc("/goals", r.services.CreateGoalHandler).Methods("POST")
	router.HandleFunc("/goals", r.services.GetGoalsHandler).Methods("GET")
	router.HandleFunc("/goals/{id}", r.services.UpdateGoalHandler).Methods("PUT")
	router.HandleFunc("/goals/{id}", r.services.DeleteGoalHandler).Methods("DELETE")
	router.HandleFunc("/subtasks", r.services.GetAllSubtasksHandler).Methods("GET")
	router.HandleFunc("/subtasks/{id}/complete", r.services.ToggleSubtaskCompletionHandler).Methods("PUT")
	router.HandleFunc("/goals/{goal_id}/subtasks", r.services.GetSubtasksByGoalHandler).Methods("GET")

	// User routes
	router.HandleFunc("/users", r.services.GetUsersHandler).Methods("GET")
	router.HandleFunc("/users", r.services.CreateUserHandler).Methods("POST")
	router.HandleFunc("/users/{id}", r.services.DeleteUserHandler).Methods("DELETE")
	router.HandleFunc("/register", r.services.RegisterHandler).Methods("POST")
	router.HandleFunc("/login", r.services.LoginHandler).Methods("POST")
	router.HandleFunc("/change-password", r.services.ChangePasswordHandler).Methods("POST")

	// DeepSeek routes
	router.HandleFunc("/llm/reformulate-goal", r.services.ReformulateGoalHandler).Methods("POST")
	router.HandleFunc("/llm/validate-goal", r.services.ValidateGoalHandler).Methods("POST")
	router.HandleFunc("/llm/validate-plan-feedback", r.services.ValidatePlanFeedbackHandler).Methods("POST")
	router.HandleFunc("/llm/validate-schedule-feedback", r.services.ValidateScheduleFeedbackHandler).Methods("POST")
	router.HandleFunc("/llm/generate-steps", r.services.GenerateStepsHandler).Methods("POST")
	router.HandleFunc("/llm/generate-schedule", r.services.GenerateScheduleHandler).Methods("POST")

	return router
}

func main() {
	// Initialize database connection
	db := connectDB()
	defer db.Close()

	// Initialize repositories
	userRepo := repositories.NewUserRepository(db)
	goalRepo := repositories.NewGoalRepository(db)
	subtaskRepo := repositories.NewSubtaskRepository(db)

	// Initialize DeepSeek manager
	deepSeekManager := deepseek.NewDeepSeekManager()

	// Initialize services
	services := services.NewServices(userRepo, goalRepo, subtaskRepo, deepSeekManager)

	// Initialize router
	router := NewRouter(services).SetupRoutes()

	fmt.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", router))
}
