package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type validateResponse struct {
	Dirname string `json:"dirname"`
	Valid   bool   `json:"valid"`
}

func validateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dirname := r.URL.Query().Get("dirname")

		valid := dirname != "" &&
			strings.TrimSpace(dirname) != "" &&
			!illegalChars.MatchString(dirname)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(validateResponse{
			Dirname: dirname,
			Valid:   valid,
		})
	}
}
