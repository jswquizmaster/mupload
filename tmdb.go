package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var tmdbClient = &http.Client{
	Timeout: 10 * time.Second,
}

const tmdbSearchURL = "https://api.themoviedb.org/3/search/movie"
const tmdbDetailURL = "https://api.themoviedb.org/3/movie"

type tmdbResponse struct {
	Results []json.RawMessage `json:"results"`
}

type movieDetails struct {
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"` // "2010-07-16"
}

func getMovieDetails(id, apiKey string) (movieDetails, error) {
	params := url.Values{}
	params.Set("api_key", apiKey)
	params.Set("language", "de-DE")

	resp, err := tmdbClient.Get(fmt.Sprintf("%s/%s?%s", tmdbDetailURL, id, params.Encode()))
	if err != nil {
		return movieDetails{}, fmt.Errorf("tmdb request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return movieDetails{}, fmt.Errorf("tmdb returned status %d", resp.StatusCode)
	}

	var details movieDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return movieDetails{}, fmt.Errorf("failed to decode tmdb response: %w", err)
	}

	return details, nil
}

func searchMovies(title, year, apiKey string) ([]json.RawMessage, error) {
	params := url.Values{}
	params.Set("api_key", apiKey)
	params.Set("query", title)
	params.Set("language", "de-DE")
	if year != "" {
		params.Set("year", year)
	}

	resp, err := tmdbClient.Get(fmt.Sprintf("%s?%s", tmdbSearchURL, params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("tmdb request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb returned status %d", resp.StatusCode)
	}

	var result tmdbResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode tmdb response: %w", err)
	}

	return result.Results, nil
}
