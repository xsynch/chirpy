package main

import (
	"log"
	"net/http"
)


func main(){
mux := http.NewServeMux()

server := &http.Server{
	Addr: ":8080",
	Handler: mux,
}
mux.Handle("/", http.FileServer(http.Dir(".")))
mux.Handle("/assets", http.FileServer(http.Dir("assets")))


log.Printf("Server files from '.' on port: 8080")
log.Fatal(server.ListenAndServe())


}