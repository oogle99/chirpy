package main

import (
	"log"
	"net/http"
)

func handlerReadiness(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func main() {
	const port = "8080"
	const filepathRoot = "."

	mux := &http.ServeMux{}
	mux.Handle("/app/", http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot))))

	mux.HandleFunc("/healthz", handlerReadiness)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Serving files from %s on port: %s\n", filepathRoot, port)
	log.Fatal(srv.ListenAndServe())
}
