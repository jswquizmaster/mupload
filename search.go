package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

// qualityPattern matches common release tags that signal the start of non-title information.
var qualityPattern = regexp.MustCompile(
	`(?i)\b(1080p|720p|480p|4k|2160p|uhd|bluray|blu-ray|bdrip|brrip|web-dl|webrip|` +
		`hdrip|hdtv|dvdrip|dvdscr|cam|ts|hc|hdr|sdr|dv|remux|proper|repack|` +
		`x264|x265|h264|h265|hevc|xvid|divx|avc|` +
		`aac|dts|ac3|dd5|truehd|atmos|flac|mp3)\b`,
)

// yearPattern matches a standalone 4-digit year between 1900 and 2099.
var yearPattern = regexp.MustCompile(`\b(19|20)\d{2}\b`)

// cleanFilename extracts a search title and optional year from a movie filename.
func cleanFilename(name string) (title, year string) {
	// Remove file extension.
	name = strings.TrimSuffix(name, filepath.Ext(name))

	// Replace dots, underscores, and brackets with spaces.
	name = strings.NewReplacer(".", " ", "_", " ", "[", " ", "]", " ", "(", " ", ")", " ").Replace(name)

	// Find year before cutting quality tags (year might appear after title).
	if m := yearPattern.FindString(name); m != "" {
		year = m
	}

	// Cut everything from the first quality tag onward.
	if loc := qualityPattern.FindStringIndex(name); loc != nil {
		name = name[:loc[0]]
	}

	// If year is still in the remaining string, remove it from the title.
	if year != "" {
		name = strings.ReplaceAll(name, year, "")
	}

	title = strings.Join(strings.Fields(name), " ")
	return
}

func searchHandler(apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filename := r.URL.Query().Get("filename")
		if filename == "" {
			http.Error(w, "query parameter 'filename' is required", http.StatusBadRequest)
			return
		}

		title, year := cleanFilename(filename)
		if title == "" {
			http.Error(w, "could not extract title from filename", http.StatusBadRequest)
			return
		}

		results, err := searchMovies(title, year, apiKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		if results == nil {
			results = []json.RawMessage{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}
