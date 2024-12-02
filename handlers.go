package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/xsynch/chirpy/internal/auth"
	"github.com/xsynch/chirpy/internal/database"
)


type apiConfig struct {
	fileserverHits atomic.Int32
	db *database.Queries
	secret string 
	polka_key string 

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
	IsChirpyRd bool `json:"is_chirpy_red"`
}

type NewJWT struct {
	Token string `json:"token"`
}

type chirpyRedUpdate struct {
	Event string `json:"event"`
	Data struct {
		UserID uuid.UUID `json:"user_id"`		
	} `json:"data"`

}

func (cfg *apiConfig) upgradeChirpyUser (w http.ResponseWriter, r *http.Request) {
	authKey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Error getting authorization key"))
		log.Printf("Authorization key error: %s",err)
		return 
	}
	if authKey != cfg.polka_key{
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Error getting authorization key"))
		log.Printf("Authorization key error: %s",err)
		return 

	}
	chirpyEvent := chirpyRedUpdate{}
	
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&chirpyEvent)
	if err != nil {
		log.Printf("Error decoding the json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Data Error"))
		return 
	}
	if chirpyEvent.Event != "user.upgraded"{
		w.WriteHeader(http.StatusNoContent)
		return 
	} else {
		
		params := database.AddChirpyRedToUserParams{
			IsChirpyRed: true,
			ID: chirpyEvent.Data.UserID,

		}
		err = cfg.db.AddChirpyRedToUser(r.Context(), params)
		if err != nil {
			log.Printf("Error updating user %v, %s", chirpyEvent.Data.UserID, err)
			w.WriteHeader(http.StatusNotFound)			
			return 
		}
		w.WriteHeader(http.StatusNoContent)
	}
}