package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/xsynch/chirpy/internal/auth"
	"github.com/xsynch/chirpy/internal/database"
)







func handleHealth(w http.ResponseWriter, r *http.Request){
	w.Header().Add("Content-Type","text/plain; charset=utf-8")	
	
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(http.StatusText(http.StatusOK)))
	if err != nil {
		log.Fatal(err)
	}
}

func (cfg *apiConfig) reset(w http.ResponseWriter, r *http.Request){
	cfg.fileserverHits = atomic.Int32{}
	cfg.db.DeleteUser(r.Context())
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(http.StatusText(http.StatusOK)))
	if err != nil {
		log.Fatal(err)
	}

}

func (cfg *apiConfig)getMetrics(w http.ResponseWriter, r *http.Request){
	currentHits := cfg.fileserverHits.Load()
	hitsText := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>",currentHits)
	w.Header().Add("Content-Type","text/html")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(hitsText))
	if err != nil {
		log.Fatal(err)
	}
}

func (cfg *apiConfig) createUsers(w http.ResponseWriter, r *http.Request){
	decoder := json.NewDecoder(r.Body)
	userRequest := chirpUser{}
	err := decoder.Decode(&userRequest)
	if err != nil {
		log.Printf("Error unmarshalling json: %s with error: %s", r.Body, err)
	}
	password, err := auth.HashPassword(userRequest.Password)
	if err != nil {
		log.Fatalf("Error hashing password %s: %s", userRequest.Password, err)
	}
	params := database.CreateUserParams{Email: userRequest.Email, HashedPassword: password}
	user, err := cfg.db.CreateUser(r.Context(), params)
	if err != nil {
		log.Fatalf("Error creating user %s: %s", userRequest.Email,err )
	}
	dbuser := createDBUserResponse{
		ID: user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email: user.Email,
	}
	dst, err := json.Marshal(dbuser)
	if err != nil {
		log.Fatalf("error marshalling %s with error: %s", dst, err)
	}
	w.Header().Set("Content-Type","application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(dst)
	

}



func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
		
		
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
			cfg.fileserverHits.Add(1)
			next.ServeHTTP(w,r)
		}) 
}

func (cfg *apiConfig) addHeaders(next http.Handler) http.Handler{
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
		w.Header().Add("Cache-Control","no-cache")
		next.ServeHTTP(w,r)
	})
}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, r *http.Request){
	headerToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("User must be logged in"))
		return
	}
	tokenUserID,err := auth.ValidateJWT(headerToken, cfg.secret)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("bad token"))
		log.Printf("%s",err)
		return 
	}
	// log.Println(tokenUserID)
	decoder := json.NewDecoder(r.Body)
	chirps := incomingChirp{}
	err = decoder.Decode(&chirps)
	if err != nil {
		log.Printf("Error decoding chirp: %s", err)
		ce := chirpError{
			Error: "Something went wrong",
		}
		dst, err := json.Marshal(ce)
		if err != nil {
			respondWithError(w,500,fmt.Sprintf("Error marshalling json: %s",err))
			log.Printf("Error marshalling json: %s", err)
			w.WriteHeader(500)
			return 
		}
		w.WriteHeader(500)
		w.Header().Set("Content-Type","application/json")
		w.Write(dst)
		
		return 
	}
	if utf8.RuneCountInString(chirps.Body) > 140 {
		ce := chirpError{
			Error: "Chirp is too long",
		}
		dst, err := json.Marshal(ce)
		if err != nil {
			log.Printf("Error marshalling json: %s", err)
			w.WriteHeader(500)
			return 
		}
		w.WriteHeader(400)
		w.Header().Set("Content-Type","application/json")
		w.Write(dst)
		return 
	}
	params := database.InsertChirpParams{
		Body: chirps.Body,
		// UserID: chirps.UserID,
		UserID: tokenUserID,
	}
	
	newChirp, err := cfg.db.InsertChirp(r.Context(),params)
	if err != nil {
		log.Fatalf("error inserting %v into db: %s", params, err)
	}
	cs := chirpSuccess{
		// ID: newChirp.ID,
		ID: tokenUserID,
		CreatedAt: newChirp.CreatedAt,
		UpdatedAt: newChirp.UpdatedAt,
		Body: cleanChirp(chirps.Body),
		UserID: newChirp.UserID,
	}
	dst, err := json.Marshal(cs)
	if err != nil {
		log.Printf("Error marshalling json: %s",err)
		w.WriteHeader(500)
		return 
	}
	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type","application/json")
	w.Write(dst)	 

}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request){
	var allChirps []chirpSuccess
	results, err := cfg.db.GetAllChirps(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error getting chirps"))
		log.Fatalf("There was an error getting chirps: %s",err)
	}
	for _, val := range results{
		newChirp := chirpSuccess{
			ID: val.ID,
			CreatedAt: val.CreatedAt,
			UpdatedAt: val.UpdatedAt,
			Body: val.Body,
			UserID: val.UserID,
		}
		allChirps = append(allChirps, newChirp)
	}
	dst, err := json.Marshal(allChirps)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("Error getting db data %v: %s", dst, err)))
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type","application/json")
	w.Write([]byte(dst))

}

func (cfg *apiConfig) getOneChirp(w http.ResponseWriter, r *http.Request){
	chirpID,_ := uuid.Parse(r.PathValue("chirpID"))
	var chirp chirpSuccess
	results, err := cfg.db.GetOneChirp(r.Context(), chirpID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error getting chirps"))
		log.Fatalf("There was an error getting chirps: %s",err)
	}
	
	chirp = chirpSuccess{
			ID: results.ID,
			CreatedAt: results.CreatedAt,
			UpdatedAt: results.UpdatedAt,
			Body: results.Body,
			UserID: results.UserID,
	}
	dst, err := json.Marshal(chirp)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("Error getting db data %v: %s", dst, err)))
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type","application/json")
	w.Write([]byte(dst))

}

func (cfg *apiConfig) chirpLogin(w http.ResponseWriter, r *http.Request){
	chirpUser := chirpUser{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&chirpUser)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("There was an error logging in."))
		log.Fatalf("Error decoding the request %v: %s",r.Body,err)
	}
	
	chirpUser.ExpiresInSeconds = int(time.Second) * 3600
	
	// log.Println("The user expiration duration is set to ", chirpUser.ExpiresInSeconds)
	userLookup, err := cfg.db.LookupUser(r.Context(), chirpUser.Email)
	if err != nil {
		log.Fatalf("Could not lookup user %s: %s",chirpUser.Email, err)
	}
	if userLookup.Email != "" {
		passCheck := auth.CheckPasswordHash(chirpUser.Password, userLookup.HashedPassword)
		if passCheck != nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Incorrect email or password"))
			return 			
		}
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Incorrect email or password"))
		return 	
	}
	chirpUserToken, err := auth.MakeJWT(userLookup.ID, cfg.secret, time.Duration(chirpUser.ExpiresInSeconds))
	
	// log.Println("the token that was created is: ",chirpUserToken)
	if err != nil {
		log.Printf("Error getting jwt: %s\n", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Server Error, Please try again"))
		return 
	}
	chirpUserRefreshtoken, err := auth.MakeRefreshToken()
	if err != nil {
		log.Printf("There was an error creating refresh token: %s\n",err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Server Error, Please try again"))
		return 
		 
	}
	refreshTokenParams := database.InsertRefreshTokenParams{UserID: userLookup.ID, Token: chirpUserRefreshtoken}
	rt, err := cfg.db.InsertRefreshToken(r.Context(), refreshTokenParams)
	if err != nil {
		log.Printf("Error inserting the refresh token %v: %s",refreshTokenParams, err )
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error with the server, please try again."))
		return 
	}
	log.Printf("Refresh token %v inserted successfully\n", rt)

	finalUser := createDBUserResponse{ ID: userLookup.ID, CreatedAt: userLookup.CreatedAt, UpdatedAt: userLookup.UpdatedAt, Email: userLookup.Email, Token: chirpUserToken, RefreshToken: chirpUserRefreshtoken}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type","application/json")
	dst, err := json.Marshal(finalUser)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("There was an error marshalling the final request"))
		log.Fatalf("Error marshalling %v: %s", dst, err)
		return 
	}
	w.Write([]byte(dst))
	
}

func (cfg *apiConfig) getRefreshToken(w http.ResponseWriter, r *http.Request){
	headerToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("User must be logged in"))
		return 
	}
	userFromToken, err := cfg.db.GetRefreshToken(r.Context(),headerToken)
	if err != nil {
		log.Printf("There was an error getting %s from the database: %s", headerToken, err)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Server Error, please try again."))
		return 
	}
	var emptyRefeshStruct database.RefreshToken
	log.Printf("The user found from the database has user id of: %v",userFromToken.UserID)
	if emptyRefeshStruct == userFromToken  {
		log.Printf("User not found in the db\n")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized User"))
		return

	}
	if userFromToken.ExpiresAt.Compare(time.Now().Add(time.Hour * 24 * 60)) == +1 {
		log.Printf("Token has expired for user %v at %v", userFromToken.UserID, userFromToken.ExpiresAt)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Token has expired"))
		return 

	}
	ok,err := userFromToken.RevokedAt.Value()
	if err != nil {
		log.Printf("This is the error from sql nulltime: %s",err)
	}
	// log.Printf("This is the ok from sql null time: %s",ok)
	if ok != nil {
		log.Printf("The token was revoked at: %v",userFromToken.RevokedAt)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid Token"))
		return 
	}
	// if userFromToken.RevokedAt.Time.GoString() != "0001-01-01 00:00:00 +0000 UTC" {
	// 	log.Printf("Token was revoked at %v", userFromToken.RevokedAt)
	// 	w.WriteHeader(http.StatusUnauthorized)
	// 	// w.Write([]byte("Invalid Token"))
	// 	return 

	// }
	val, err := auth.MakeJWT(userFromToken.UserID, cfg.secret, time.Hour)
	if err != nil {
		log.Printf("Error generating JWT: %s", err)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Server Error, Please try again"))
		return 
	}
	newjwt := NewJWT{Token: val}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type","application/json")
	dst, err := json.Marshal(newjwt)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("There was an error marshalling the final request"))
		log.Printf("Error marshalling %v: %s", dst, err)
		return 
	}
	w.Write([]byte(dst))

	
}

func (cfg *apiConfig) revokeRefreshToken(w http.ResponseWriter, r *http.Request) {
	tokenHeader, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token\n")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("There was a server error, please try again."))
		return		
	}
	userFromToken, err := cfg.db.GetRefreshToken(r.Context(),tokenHeader)
	if err != nil {
		log.Printf("Error getting user information from the database: %s",err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error getting user information, please try again."))
		return 
	}
	var emptyRefeshStruct database.RefreshToken
	if userFromToken == emptyRefeshStruct {
		log.Printf("User not found in the database: %v", userFromToken)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Bad Request to revoke this user."))
		return 
	}
	err = cfg.db.SetRevokedDate(r.Context(), userFromToken.Token)
	if err != nil {
		log.Printf("There was an error removing %s from the database", userFromToken.UserID)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Erorr revoking users refresh token."))
		return 
	}
	w.WriteHeader(http.StatusNoContent)
	

}

func cleanChirp(chirp string) string {
	var final_string = strings.Split(chirp," ")
	for idx,val := range final_string {
		if strings.ToLower(val) == "kerfuffle" || strings.ToLower(val) == "sharbert" || strings.ToLower(val) == "fornax"{
			final_string[idx] = "****"
		}

	}
	return strings.Join(final_string," ")

}

func respondWithError(w http.ResponseWriter, code int, msg string){	
	w.WriteHeader(code)
	w.Write([]byte(msg))
	return 
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}){
	w.Header().Set("Content-Type","application/json")
	w.WriteHeader(code)
	fmt.Println(payload)

}

func main(){
godotenv.Load(".env")
dbURL := os.Getenv("DB_URL")
secretKey := os.Getenv("SECRET")


db, err := sql.Open("postgres", dbURL)
if err != nil {
	log.Fatal(err)
}
dbQueries := database.New(db)

rootDir := "."
httpPort := 8080

apiConfig := apiConfig{fileserverHits: atomic.Int32{}, db: dbQueries, secret: secretKey}

mux := http.NewServeMux()

server := &http.Server{
	Addr: ":8080",
	Handler: mux,
}

fs := http.FileServer(http.Dir(rootDir))
mux.Handle("/app/", apiConfig.middlewareMetricsInc(http.StripPrefix("/app",apiConfig.addHeaders(fs))))


mux.HandleFunc("GET /admin/metrics",apiConfig.getMetrics)
mux.HandleFunc("POST /admin/reset", apiConfig.reset)

mux.HandleFunc("GET /api/healthz", handleHealth)
mux.HandleFunc("GET /api/chirps", apiConfig.getChirps)
mux.HandleFunc("GET /api/chirps/{chirpID}", apiConfig.getOneChirp)
mux.HandleFunc("POST /api/chirps", apiConfig.createChirp)
mux.HandleFunc("POST /api/users", apiConfig.createUsers)
mux.HandleFunc("POST /api/login", apiConfig.chirpLogin)
mux.HandleFunc("POST /api/refresh", apiConfig.getRefreshToken)
mux.HandleFunc("POST /api/revoke", apiConfig.revokeRefreshToken)


log.Printf("Server files from %s on port: %d",rootDir, httpPort)
log.Fatal(server.ListenAndServe())


}