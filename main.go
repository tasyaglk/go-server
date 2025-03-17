// package main

// import "fmt"

// func main() {
// 	fmt.Println("Hello, World!")
// }

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	_ "github.com/lib/pq"
)

type User struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Surname      string `json:"surname"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
}

// Подключение к PostgreSQL
func connectDB() *sql.DB {
	connStr := "user=postgres dbname=postgres password=mypassword host=localhost sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	return db
}

// GET /users - получить всех пользователей
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

// POST /users - создать пользователя
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

// POST /users - создать пользователя
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

	// Хэшируем пароль
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

	// Сравниваем хэши
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(credentials.Password))
	if err != nil {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	// Возвращаем данные пользователя (без пароля)
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

	// Получаем хэш старого пароля из базы данных
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

	// Проверяем, совпадает ли старый пароль
	err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(request.OldPassword))
	if err != nil {
		http.Error(w, "Invalid old password", http.StatusUnauthorized)
		return
	}

	// Хэшируем новый пароль
	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Обновляем пароль в базе данных
	_, err = db.Exec("UPDATE users SET password_hash = $1 WHERE email = $2", newHashedPassword, request.Email)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Password updated successfully"})
}

func main() {
	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			getUsersHandler(w, r)
		case "POST":
			createUserHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/change-password", changePasswordHandler)

	fmt.Println("Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
