package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

type incomingChirp struct {
	Body string `json:"body"`
}
type chirpError struct {
	Error string `json:"error"`
}

type chirpSuccess struct{
	Cleaned_body string `json:"cleaned_body"`
}


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

func (cfg *apiConfig) validateChirp(w http.ResponseWriter, r *http.Request){

	decoder := json.NewDecoder(r.Body)
	chirps := incomingChirp{}
	err := decoder.Decode(&chirps)
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
	

	cs := chirpSuccess{
		Cleaned_body: cleanChirp(chirps.Body),
	}
	dst, err := json.Marshal(cs)
	if err != nil {
		log.Printf("Error marshalling json: %s",err)
		w.WriteHeader(500)
		return 
	}
	w.WriteHeader(200)
	w.Header().Set("Content-Type","application/json")
	w.Write(dst)	 

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

rootDir := "."
httpPort := 8080

apiConfig := apiConfig{fileserverHits: atomic.Int32{}}

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
mux.HandleFunc("POST /api/validate_chirp", apiConfig.validateChirp)


log.Printf("Server files from %s on port: %d",rootDir, httpPort)
log.Fatal(server.ListenAndServe())


}