package main

import (
	"log"
	"net/http"
	"os"

	chathandler   "github.com/angrosist/demo/api/chat"
	healthhandler "github.com/angrosist/demo/api/health"
	leadshandler  "github.com/angrosist/demo/api/leads"
	detailhandler "github.com/angrosist/demo/api/leads/detail"
	httputil      "github.com/angrosist/demo/pkg/adapters/http"
)

func cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httputil.HandleOptions(w, r)
		next(w, r)
	}
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", healthhandler.Handler)
	mux.HandleFunc("/api/chat", chathandler.Handler)
	mux.HandleFunc("/api/leads/", detailhandler.Handler)
	mux.HandleFunc("/api/leads", leadshandler.Handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Backend running on http://localhost:" + port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
