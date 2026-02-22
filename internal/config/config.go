package config

import (
	"os"
	"time"
)

type Config struct {
	ServerAddr     string
	DatabasePath   string
	JWTSecret      string
	TokenExpiry    time.Duration
	FinePerDay     float64 // fine in currency units per day late
	LoanPeriodDays int
	MaxRenewals    int
	ReservationTTL time.Duration // how long a fulfilled reservation stays valid before expiring
}

func Load() *Config {
	return &Config{
		ServerAddr:     getEnv("SERVER_ADDR", ":8080"),
		DatabasePath:   getEnv("DATABASE_PATH", "library.db"),
		JWTSecret:      getEnv("JWT_SECRET", "change-me-in-production-please"),
		TokenExpiry:    24 * time.Hour,
		FinePerDay:     0.50,
		LoanPeriodDays: 14,
		MaxRenewals:    2,
		ReservationTTL: 48 * time.Hour,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
