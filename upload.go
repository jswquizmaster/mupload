package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// illegalChars matches characters that are forbidden in directory names on
// Linux, macOS, and Windows so the path stays portable.
var illegalChars = regexp.MustCompile(`[/\\:*?"<>|]`)

func sanitizeDirName(s string) string {
	s = illegalChars.ReplaceAllString(s, "_")
	s = strings.TrimSpace(s)
	return s
}

// releaseYear extracts the 4-digit year from a TMDB release_date ("2010-07-16").
func releaseYear(releaseDate string) string {
	if len(releaseDate) >= 4 {
		return releaseDate[:4]
	}
	return ""
}

type uploadResponse struct {
	Filename string `json:"filename"`
	Dir      string `json:"dir"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
}

func uploadHandler(uploadDir, apiKey, jellyfinURL, jellyfinKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmdbID := r.URL.Query().Get("tmdb_id")
		if tmdbID == "" {
			http.Error(w, "query parameter 'tmdb_id' is required", http.StatusBadRequest)
			return
		}

		details, err := getMovieDetails(tmdbID, apiKey)
		if err != nil {
			http.Error(w, fmt.Sprintf("tmdb lookup failed: %v", err), http.StatusBadGateway)
			return
		}
		if details.Title == "" {
			http.Error(w, "tmdb returned no title for this id", http.StatusBadGateway)
			return
		}

		year := releaseYear(details.ReleaseDate)
		var dirName string
		if year != "" {
			dirName = sanitizeDirName(fmt.Sprintf("%s (%s)", details.Title, year))
		} else {
			dirName = sanitizeDirName(details.Title)
		}

		targetDir := filepath.Join(uploadDir, dirName)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			http.Error(w, "failed to create movie directory", http.StatusInternalServerError)
			return
		}

		// Keep at most 32 KB in memory; everything else goes to OS temp files.
		if err := r.ParseMultipartForm(32 << 10); err != nil {
			http.Error(w, "invalid multipart form", http.StatusBadRequest)
			return
		}

		src, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "field 'file' missing", http.StatusBadRequest)
			return
		}
		defer src.Close()

		// Sanitize filename to prevent path traversal.
		filename := filepath.Base(header.Filename)
		if filename == "." || filename == "/" {
			http.Error(w, "invalid filename", http.StatusBadRequest)
			return
		}

		nfoPath := filepath.Join(targetDir, "movie.nfo")
		if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
			nfoContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<movie>
  <uniqueid type="tmdb" default="true">%s</uniqueid>
</movie>`, tmdbID)
			os.WriteFile(nfoPath, []byte(nfoContent), 0644)
		}

		dstPath := filepath.Join(targetDir, filename)
		dst, err := os.Create(dstPath)
		if err != nil {
			http.Error(w, "failed to create file", http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		written, err := io.Copy(dst, src)
		if err != nil {
			http.Error(w, "failed to write file", http.StatusInternalServerError)
			return
		}

		if jellyfinURL != "" && jellyfinKey != "" {
			if err := refreshJellyfinLibrary(jellyfinURL, jellyfinKey); err != nil {
				log.Printf("jellyfin refresh failed: %v", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(uploadResponse{
			Filename: filename,
			Dir:      dirName,
			Path:     dstPath,
			Size:     written,
		})
	}
}
