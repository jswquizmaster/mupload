package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"

	"github.com/schollz/progressbar/v3"
)

// uploadResponse mirrors the JSON the server returns on a successful upload.
type uploadResponse struct {
	Filename string `json:"filename"`
	Dir      string `json:"dir"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	tmdbID := flag.String("tmdb", "", "TMDB id (optional; if omitted the file is looked up via /search)")
	dirname := flag.String("dirname", "", "optional target directory name override")
	server := flag.String("server", "http://localhost:8080", "mupload server base URL")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [--tmdb <id>] [--dirname <name>] [--server <url>] <file>\n\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		return fmt.Errorf("exactly one file argument is required")
	}
	path := flag.Arg(0)

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", path)
	}

	name := filepath.Base(path)

	// When no TMDB id is given, look the file up via /search, let the user
	// pick the matching movie, and prompt for the target directory name.
	resolvedID := *tmdbID
	resolvedDirname := *dirname
	if resolvedID == "" {
		resolvedID, resolvedDirname, err = resolveViaSearch(*server, name, *dirname)
		if err != nil {
			return err
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	q := url.Values{"tmdb_id": {resolvedID}}
	if resolvedDirname != "" {
		q.Set("dirname", resolvedDirname)
	}
	uploadURL, err := endpointURL(*server, "upload", q)
	if err != nil {
		return err
	}

	bar := progressbar.DefaultBytes(info.Size(), "uploading "+name)

	// Stream the multipart body through a pipe so the file is never fully
	// buffered in memory. Pipe writes block until the HTTP client consumes
	// them, so the bar tracks the real network send rate.
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		part, err := mw.CreateFormFile("file", name)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, io.TeeReader(file, bar)); err != nil {
			pw.CloseWithError(err)
			return
		}
		if err := mw.Close(); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	req, err := http.NewRequest(http.MethodPost, uploadURL, pr)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %s: %s", resp.Status, string(body))
	}

	var out uploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("could not decode server response: %w", err)
	}

	fmt.Printf("\nuploaded %s (%d bytes)\n  dir:  %s\n  path: %s\n",
		out.Filename, out.Size, out.Dir, out.Path)
	return nil
}

// endpointURL joins an endpoint path onto the server base URL and attaches the
// given query parameters.
func endpointURL(server, endpoint string, q url.Values) (string, error) {
	base, err := url.Parse(server)
	if err != nil {
		return "", fmt.Errorf("invalid --server URL: %w", err)
	}
	base.Path = pathpkg.Join(base.Path, endpoint)
	base.RawQuery = q.Encode()
	return base.String(), nil
}
