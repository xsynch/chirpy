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
		IsChirpyRd: user.IsChirpyRed,
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
		log.Printf("error inserting %v into db: %s", params, err)
		return 
	}
	// log.Printf("Successfully inserted %v into the db", newChirp)
	cs := chirpSuccess{
		ID: newChirp.ID,
		
		// ID: tokenUserID,
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
	
	authorid := r.URL.Query().Get("author_id")
	ascOrdesc := r.URL.Query().Get("sort")
	

	var allChirps []chirpSuccess
	var results  []database.Chirp
	var err error
	if authorid != ""{
		// log.Printf("Found aurhorid: %s", authorid)
		authParsed,err := uuid.Parse(authorid)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Error getting chirps"))
			log.Printf("Could not parse %s: %s", authorid, err)
			return 
		}
		if ascOrdesc != "desc"{
			results, err = cfg.db.GetChirpsByAuthor(r.Context(),authParsed)
		} else {
			results, err = cfg.db.GetChirpsByAuthorDesc(r.Context(),authParsed)
		}
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Error getting chirps"))
			log.Printf("There was an error getting chirps: %s",err)
			return
		}
		// log.Printf("results from authorid %s is %v",authorid, results)
		
	} else {
		if ascOrdesc != "desc"{
		results, err = cfg.db.GetAllChirps(r.Context())
		} else {
			results, err = cfg.db.GetAllChirpsDesc(r.Context())
		}
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Error getting chirps"))
			log.Printf("There was an error getting chirps: %s",err)
			return
		}
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
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Error finding chirp"))
		log.Printf("There was an error getting chirp %v: %s", chirpID,err)
		return 
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
		w.Write([]byte(fmt.Sprintf("Error Marshalling data %v: %s", dst, err)))
		return 
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
		log.Printf("Error decoding the request %v: %s",r.Body,err)
		return 
	}
	
	chirpUser.ExpiresInSeconds = int(time.Second) * 3600
	
	// log.Println("The user expiration duration is set to ", chirpUser.ExpiresInSeconds)
	userLookup, err := cfg.db.LookupUser(r.Context(), chirpUser.Email)
	if err != nil {
		log.Printf("Could not lookup user %s: %s",chirpUser.Email, err)
		return 
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
	_, err = cfg.db.InsertRefreshToken(r.Context(), refreshTokenParams)
	if err != nil {
		log.Printf("Error inserting the refresh token %v: %s",refreshTokenParams, err )
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error with the server, please try again."))
		return 
	}
	// log.Printf("Refresh token %v inserted successfully\n", rt)

	finalUser := createDBUserResponse{ ID: userLookup.ID, CreatedAt: userLookup.CreatedAt, UpdatedAt: userLookup.UpdatedAt, Email: userLookup.Email, Token: chirpUserToken, RefreshToken: chirpUserRefreshtoken, IsChirpyRd: userLookup.IsChirpyRed}
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

func (cfg *apiConfig) updateUsers(w http.ResponseWriter, r *http.Request){
	tokenHeader, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("There was an error getting the header token")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Server Error, please try again."))
		return 
	}
	_,err = auth.ValidateJWT(tokenHeader, cfg.secret) 
	if err != nil {
		log.Printf("Invalid token: %s", tokenHeader)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Invalid User"))
		return 
	}
	updateChirpUser := chirpUser{}
	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&updateChirpUser)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("There was an error logging in."))
		log.Fatalf("Error decoding the request %v: %s",r.Body,err)
	}
	password, err := auth.HashPassword(updateChirpUser.Password)
	if err != nil {
		log.Printf("Error hashing password %s: %s", updateChirpUser.Password, err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Server error, please try again."))
		return 
	}
	params := database.UpdatePasswordParams{Email: updateChirpUser.Email, HashedPassword: password}
	err = cfg.db.UpdatePassword(r.Context(), params)
	if err != nil {
		log.Printf("There was an error updating user %s, erorr: %s", updateChirpUser.Email, err)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Error updating the user"))
		return 
	}
	finalUser := chirpUser{Email: updateChirpUser.Email}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type","application/json")
	dst, err := json.Marshal(finalUser)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("There was an error marshalling the final request"))
		log.Printf("Error marshalling %v: %s", dst, err)
		return 
	}
	w.Write([]byte(dst))



}

func (cfg *apiConfig) deleteChirp(w http.ResponseWriter, r *http.Request){
	tokenHeader, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Error getting token")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Server Error, please try again"))
		return 
	}
	userID, err := auth.ValidateJWT(tokenHeader, cfg.secret)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Error validating user"))
		return 
	}
	
	chirpID,_ := uuid.Parse(r.PathValue("chirpID"))
	
	results, err := cfg.db.GetOneChirp(r.Context(), chirpID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Error finding chirp"))
		log.Printf("There was an error getting chirps: %s",err)
		return 
	}
	if userID == results.UserID {
	// log.Printf("The results from the lookup: %v", results)
	//log.Printf("This is in the header: %v", r.Header)	
		err = cfg.db.DeleteChirp(r.Context(), results.ID)
		if err != nil {
			log.Printf("There was an error deleting %v: %s", results.ID, err)
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Error Deleting the chirp"))
			return 
		}
		
		w.WriteHeader(http.StatusNoContent)
		return 
	} else {
		log.Printf("User %s not authorized to delete %s", userID, results.Body)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Not authorized to delete chirp."))
		return 
	}
	
	// chirp := chirpSuccess{
	// 		ID: results.ID,
	// 		CreatedAt: results.CreatedAt,
	// 		UpdatedAt: results.UpdatedAt,
	// 		Body: results.Body,
	// 		UserID: results.UserID,
	// }

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
polka_secret := os.Getenv("POLKA_KEY")


db, err := sql.Open("postgres", dbURL)
if err != nil {
	log.Fatal(err)
}
dbQueries := database.New(db)

rootDir := "."
httpPort := 8080

apiConfig := apiConfig{fileserverHits: atomic.Int32{}, db: dbQueries, secret: secretKey, polka_key: polka_secret}

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

mux.HandleFunc("PUT /api/users", apiConfig.updateUsers)
mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiConfig.deleteChirp)
mux.HandleFunc("POST /api/polka/webhooks", apiConfig.upgradeChirpyUser)


log.Printf("Server files from %s on port: %d",rootDir, httpPort)
log.Fatal(server.ListenAndServe())


}