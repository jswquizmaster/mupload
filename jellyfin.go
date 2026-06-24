package main

import (
	"fmt"
	"net/http"
	"strings"
)

func refreshJellyfinLibrary(jellyfinURL, apiKey string) error {
	url := strings.TrimRight(jellyfinURL, "/") + "/Library/Refresh"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, apiKey))

	resp, err := tmdbClient.Do(req)
	if err != nil {
		return fmt.Errorf("jellyfin request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("jellyfin returned status %d", resp.StatusCode)
	}
	return nil
}
