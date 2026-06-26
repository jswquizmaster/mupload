package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/peterh/liner"
	"golang.org/x/term"
)

// searchResult is one movie hit from the server's /search endpoint, which
// forwards raw TMDB movie objects.
type searchResult struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"` // "2010-07-16"
}

// label renders a result as "Title (Year)", falling back to just the title
// when no release date is known.
func (r searchResult) label() string {
	if len(r.ReleaseDate) >= 4 {
		return fmt.Sprintf("%s (%s)", r.Title, r.ReleaseDate[:4])
	}
	return r.Title
}

// resolveViaSearch searches the server for the given filename, lets the user
// pick the matching movie, and prompts for the target directory name. It
// returns the chosen movie's TMDB id and the directory name to upload to.
func resolveViaSearch(server, filename, dirnameFlag string) (id, dirname string, err error) {
	results, err := searchMovies(server, filename)
	if err != nil {
		return "", "", err
	}
	if len(results) == 0 {
		return "", "", fmt.Errorf("no movies found for %q", filename)
	}

	var chosen searchResult
	if len(results) == 1 {
		chosen = results[0]
		fmt.Fprintf(os.Stderr, "1 match: %s\n", chosen.label())
	} else {
		chosen, err = selectMovie(results)
		if err != nil {
			return "", "", err
		}
	}
	fmt.Fprintf(os.Stderr, "selected: %s [tmdb=%d]\n", chosen.label(), chosen.ID)

	// Pre-fill the prompt with an explicit --dirname when given, otherwise
	// with the server-sanitized "Title (Year)".
	start := dirnameFlag
	if start == "" {
		start, err = sanitizeDirname(server, chosen.label())
		if err != nil {
			return "", "", err
		}
	}
	dirname, err = promptDirname(start)
	if err != nil {
		return "", "", err
	}

	return strconv.Itoa(chosen.ID), dirname, nil
}

// validateResponse mirrors the server's /validate JSON.
type validateResponse struct {
	Dirname   string `json:"dirname"`
	Sanitized string `json:"sanitized"`
	Valid     bool   `json:"valid"`
}

// sanitizeDirname asks the server's /validate endpoint to sanitize a directory
// name and returns the cleaned value.
func sanitizeDirname(server, dirname string) (string, error) {
	u, err := endpointURL(server, "validate", url.Values{"dirname": {dirname}})
	if err != nil {
		return "", err
	}
	resp, err := http.Get(u)
	if err != nil {
		return "", fmt.Errorf("validate request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("validate returned %s: %s", resp.Status, string(body))
	}
	var vr validateResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return "", fmt.Errorf("could not decode validate response: %w", err)
	}
	return vr.Sanitized, nil
}

// searchMovies queries the server's /search endpoint.
func searchMovies(server, filename string) ([]searchResult, error) {
	searchURL, err := endpointURL(server, "search", url.Values{"filename": {filename}})
	if err != nil {
		return nil, err
	}

	resp, err := http.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search returned %s: %s", resp.Status, string(body))
	}

	var results []searchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("could not decode search response: %w", err)
	}
	return results, nil
}

// selectMovie shows an interactive arrow-key menu on the terminal. When stdin
// is not an interactive terminal it falls back to a numbered prompt.
func selectMovie(results []searchResult) (searchResult, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return selectMovieNumbered(results)
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return selectMovieNumbered(results)
	}
	defer term.Restore(fd, oldState)

	out := os.Stderr
	cur := 0

	// The menu is drawn on its own block of lines; redraws move the cursor
	// back to the top of that block and overwrite it in place.
	fmt.Fprint(out, "Select a movie (↑/↓ to move, Enter to confirm, q to cancel):\r\n")
	draw := func(first bool) {
		if !first {
			fmt.Fprintf(out, "\033[%dA", len(results)) // cursor up to block start
		}
		for i, r := range results {
			marker, line := "  ", r.label()
			if i == cur {
				marker, line = "> ", "\033[7m"+line+"\033[0m" // reverse video
			}
			fmt.Fprintf(out, "\r\033[K%s%s\r\n", marker, line)
		}
	}
	draw(true)

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return searchResult{}, err
		}

		switch {
		case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
			return results[cur], nil
		case n == 1 && (buf[0] == 'q' || buf[0] == 3): // q or Ctrl-C
			return searchResult{}, fmt.Errorf("selection cancelled")
		case n == 1 && buf[0] == 'k':
			cur--
		case n == 1 && buf[0] == 'j':
			cur++
		case n == 3 && buf[0] == 0x1b && buf[1] == '[': // arrow keys: ESC [ A/B
			switch buf[2] {
			case 'A':
				cur--
			case 'B':
				cur++
			}
		default:
			continue
		}

		if cur < 0 {
			cur = 0
		}
		if cur >= len(results) {
			cur = len(results) - 1
		}
		draw(false)
	}
}

// promptDirname asks for the target directory name on the terminal, pre-filled
// with def and fully editable (←/→, Home/End, word/line kill, history) via the
// liner readline library. When stdin is not a terminal it falls back to a
// default-on-empty line prompt.
func promptDirname(def string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return promptDirnameLine(def)
	}

	line := liner.NewLiner()
	defer line.Close()
	line.SetCtrlCAborts(true)

	name, err := line.PromptWithSuggestion("Directory name: ", def, len([]rune(def)))
	if err == liner.ErrPromptAborted {
		return "", fmt.Errorf("cancelled")
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(name), nil
}

// promptDirnameLine is the non-interactive fallback for promptDirname.
func promptDirnameLine(def string) (string, error) {
	fmt.Fprintf(os.Stderr, "Directory name [%s]: ", def)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return def, nil
	}
	if line := strings.TrimSpace(scanner.Text()); line != "" {
		return line, nil
	}
	return def, nil
}

// selectMovieNumbered is the non-interactive fallback: print a numbered list
// and read the chosen index from stdin.
func selectMovieNumbered(results []searchResult) (searchResult, error) {
	for i, r := range results {
		fmt.Fprintf(os.Stderr, "  [%d] %s\n", i+1, r.label())
	}
	fmt.Fprint(os.Stderr, "Enter number: ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return searchResult{}, fmt.Errorf("no selection made")
	}
	n, err := strconv.Atoi(scanner.Text())
	if err != nil || n < 1 || n > len(results) {
		return searchResult{}, fmt.Errorf("invalid selection %q", scanner.Text())
	}
	return results[n-1], nil
}
