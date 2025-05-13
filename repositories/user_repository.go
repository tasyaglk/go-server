package repositories

import (
	"context"
	"database/sql"
	"go-server/entities"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(user *entities.User) error {
	sqlStatement := `
        INSERT INTO users (name, surname, email, password_hash)
        VALUES ($1, $2, $3, $4)
        RETURNING id`
	return r.db.QueryRow(sqlStatement, user.Name, user.Surname, user.Email, user.PasswordHash).Scan(&user.ID)
}

func (r *UserRepository) Delete(userID int) (sql.Result, error) {
	return r.db.Exec("DELETE FROM users WHERE id = $1", userID)
}

func (r *UserRepository) GetAll() ([]entities.User, error) {
	rows, err := r.db.Query("SELECT id, name, surname, email FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []entities.User
	for rows.Next() {
		var u entities.User
		if err := rows.Scan(&u.ID, &u.Name, &u.Surname, &u.Email); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *UserRepository) GetByEmail(email string) (*entities.User, error) {
	var user entities.User
	err := r.db.QueryRow("SELECT id, name, surname, email, password_hash FROM users WHERE email = $1", email).
		Scan(&user.ID, &user.Name, &user.Surname, &user.Email, &user.PasswordHash)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) UpdatePassword(email, passwordHash string) error {
	_, err := r.db.Exec("UPDATE users SET password_hash = $1 WHERE email = $2", passwordHash, email)
	return err
}

func (r *UserRepository) Begin() (*sql.Tx, error) {
	return r.db.Begin()
}

func (r *UserRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}
