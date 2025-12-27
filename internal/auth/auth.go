package auth

import (
	"database/sql"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// User represents a user in the system
type User struct {
	ID           int64
	Username     string
	PasswordHash string
}

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserExists         = errors.New("user already exists")
	ErrUserNotFound       = errors.New("user not found")
)

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	// Cost factor 12 is a good balance between security and performance
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a password with a hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CreateUser creates a new user in the database
func CreateUser(db *sql.DB, username, password string) error {
	// Hash the password
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}

	// Insert into database
	query := `INSERT INTO users (username, password_hash) VALUES (?, ?)`
	_, err = db.Exec(query, username, hash)
	if err != nil {
		return ErrUserExists
	}

	return nil
}

// GetUserByUsername retrieves a user by username
func GetUserByUsername(db *sql.DB, username string) (*User, error) {
	query := `SELECT id, username, password_hash FROM users WHERE username = ?`

	user := &User{}
	err := db.QueryRow(query, username).Scan(&user.ID, &user.Username, &user.PasswordHash)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	return user, nil
}

// Authenticate verifies username and password
func Authenticate(db *sql.DB, username, password string) (*User, error) {
	user, err := GetUserByUsername(db, username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !CheckPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// HasUsers checks if any users exist in the database
func HasUsers(db *sql.DB) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM users`
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to count users: %w", err)
	}
	return count > 0, nil
}
