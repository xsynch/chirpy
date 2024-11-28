package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
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
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(fmt.Sprintf("Hits: %d", currentHits)))
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

mux.HandleFunc("/healthz", handleHealth)
mux.HandleFunc("/metrics",apiConfig.getMetrics)
mux.HandleFunc("/reset", apiConfig.reset)


log.Printf("Server files from %s on port: %d",rootDir, httpPort)
log.Fatal(server.ListenAndServe())


}