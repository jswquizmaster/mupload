package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	apiKey := os.Getenv("TMDB_API_KEY")
	if apiKey == "" {
		log.Fatal("TMDB_API_KEY environment variable is required")
	}

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatalf("failed to create upload directory: %v", err)
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	jellyfinURL := os.Getenv("JELLYFIN_URL")
	jellyfinKey := os.Getenv("JELLYFIN_API_KEY")
	if jellyfinURL == "" {
		log.Println("JELLYFIN_URL not set — library refresh disabled")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /upload", uploadHandler(uploadDir, apiKey, jellyfinURL, jellyfinKey))
	mux.HandleFunc("GET /search", searchHandler(apiKey))
	mux.HandleFunc("GET /validate", validateHandler())

	fmt.Printf("mupload listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
