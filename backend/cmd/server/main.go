package main

import (
	"log"
	"net/http"
	"os"

	"github.com/angrosist/demo/internal/app"
)

func main() {
	handler := Router(app.GetContainer())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Backend running on http://localhost:" + port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}
