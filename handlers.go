package main

import (
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/xsynch/chirpy/internal/database"
)


type apiConfig struct {
	fileserverHits atomic.Int32
	db *database.Queries
	secret string 

}

type incomingChirp struct {
	Body string `json:"body"`
	UserID uuid.UUID `json:"user_id"`
}
type chirpError struct {
	Error string `json:"error"`
}

type chirpSuccess struct{
	ID uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body string 		`json:"body"`
	UserID uuid.UUID	`json:"user_id"`
}

type chirpUser struct {
	Email string `json:"email"`
	Password string `json:"password"`
	ExpiresInSeconds int `json:"expires_in_seconds,omitempty"`
}

type createDBUserResponse struct {
	ID uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Email string 	`json:"email"`
	Token string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

type NewJWT struct {
	Token string `json:"token"`
}