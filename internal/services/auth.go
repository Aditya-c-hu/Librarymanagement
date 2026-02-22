package services

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/Aditya-c-hu/Librarymanagement/internal/models"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailTaken         = errors.New("email already registered")
	ErrUserNotFound       = errors.New("user not found")
)

type AuthService struct {
	db        *sql.DB
	jwtSecret []byte
	expiry    time.Duration
}

func NewAuthService(db *sql.DB, jwtSecret string, expiry time.Duration) *AuthService {
	return &AuthService{
		db:        db,
		jwtSecret: []byte(jwtSecret),
		expiry:    expiry,
	}
}

func (s *AuthService) Register(req models.RegisterRequest) (*models.AuthResponse, error) {
	if req.Role != models.RoleLibrarian && req.Role != models.RoleStudent {
		return nil, fmt.Errorf("role must be 'librarian' or 'student'")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	result, err := s.db.Exec(
		`INSERT INTO users (email, password, name, role) VALUES (?, ?, ?, ?)`,
		req.Email, string(hash), req.Name, string(req.Role),
	)
	if err != nil {
		return nil, ErrEmailTaken
	}

	id, _ := result.LastInsertId()
	user := models.User{
		ID:    id,
		Email: req.Email,
		Name:  req.Name,
		Role:  req.Role,
	}

	token, err := s.generateToken(user)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{Token: token, User: user}, nil
}

func (s *AuthService) Login(req models.LoginRequest) (*models.AuthResponse, error) {
	var user models.User
	var hash string

	err := s.db.QueryRow(
		`SELECT id, email, password, name, role, created_at, updated_at FROM users WHERE email = ?`,
		req.Email,
	).Scan(&user.ID, &user.Email, &hash, &user.Name, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := s.generateToken(user)
	if err != nil {
		return nil, err
	}

	return &models.AuthResponse{Token: token, User: user}, nil
}

func (s *AuthService) GetUserByID(id int64) (*models.User, error) {
	var user models.User
	err := s.db.QueryRow(
		`SELECT id, email, password, name, role, created_at, updated_at FROM users WHERE id = ?`, id,
	).Scan(&user.ID, &user.Email, &user.Password, &user.Name, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return &user, nil
}

type Claims struct {
	UserID int64       `json:"user_id"`
	Email  string      `json:"email"`
	Role   models.Role `json:"role"`
	jwt.RegisteredClaims
}

func (s *AuthService) generateToken(user models.User) (string, error) {
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *AuthService) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
