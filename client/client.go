package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	server   *string
	port     *int
	username *string
	password *string
	verbose  *bool
	years    yearFlags
)

// Custom flag type to handle multiple year arguments
type yearFlags []int

func (y *yearFlags) String() string {
	return fmt.Sprintf("%v", *y)
}

func (y *yearFlags) Set(value string) error {
	year, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	*y = append(*y, year)
	return nil
}

func printVerbose(format string, v ...interface{}) {
	if *verbose {
		log.Printf(format, v...)
	}
}

// AuthRequest mirrors the JSON structure for the authentication request.
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse mirrors the JSON structure for the authentication response.
type AuthResponse struct {
	Bearer string `json:"bearer"`
}

// authenticate handles logging into the server to get a bearer token.
func authenticate(server string, port int, user, pass string) (string, error) {
	printVerbose("Connecting to server at %s:%d with username %s", server, port, user)
	authURL := fmt.Sprintf("http://%s:%d/api/auth", server, port)
	reqBody, err := json.Marshal(AuthRequest{Username: user, Password: pass})
	if err != nil {
		return "", fmt.Errorf("failed to create auth request body: %w", err)
	}

	resp, err := http.Post(authURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed! Status Code: %d", resp.StatusCode)
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", fmt.Errorf("failed to decode auth response: %w", err)
	}

	if authResp.Bearer == "" {
		return "", fmt.Errorf("authentication failed, bearer token not received")
	}

	printVerbose("Login successful!")
	return authResp.Bearer, nil
}

// fetchMoviesInPage fetches movies for a single page and returns the count and status code.
func fetchMoviesInPage(ctx context.Context, server string, port, year, page int, bearer string) (int, int, error) {
	pageURL := fmt.Sprintf("http://%s:%d/api/movies/%d/%d", server, port, year, page)
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create request for page %d: %w", page, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Don't return an error if the context was canceled, as it's expected.
		if ctx.Err() != nil {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("failed to fetch page %d: %w", page, err)
	}
	defer resp.Body.Close()

	printVerbose("Fetching page %d for year %d: Status Code %d", page, year, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return 0, resp.StatusCode, nil
	}

	var movies []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&movies); err != nil {
		return 0, resp.StatusCode, fmt.Errorf("failed to decode movies response for page %d: %w", page, err)
	}

	return len(movies), resp.StatusCode, nil
}

// fetchMoviesByYear orchestrates fetching all movies for a given year.
// It first determines the number of movies on page 1, and if that is zero, returns zero.
// It then finds the last page with movies. With the number of pages, it can calculate the total number of movies.
func fetchMoviesByYear(server string, port, year int, bearer, user, pass string) (int, error) {
	// Get movies on page 1 to find out movies per page.
	moviesOnPage1, status, err := fetchMoviesInPage(context.Background(), server, port, year, 1, bearer)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch page 1 for year %d: %w", year, err)
	}
	if status == http.StatusUnauthorized {
		printVerbose("Session expired, re-authenticating...")
		bearer, err = authenticate(server, port, user, pass)
		if err != nil {
			return 0, fmt.Errorf("failed to re-authenticate: %w", err)
		}
		moviesOnPage1, status, err = fetchMoviesInPage(context.Background(), server, port, year, 1, bearer)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch page 1 for year %d after re-auth: %w", year, err)
		}
	}

	if status != http.StatusOK {
		return 0, fmt.Errorf("failed to fetch page 1 for year %d. Status: %d", year, status)
	}

	if moviesOnPage1 == 0 {
		return 0, nil
	}
	moviesPerPage := moviesOnPage1

	// Find the last page
	lastPage := 1
	// Exponential search for an upper bound
	for {
		count, status, err := fetchMoviesInPage(context.Background(), server, port, year, lastPage*2, bearer)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch page %d for year %d: %w", lastPage*2, year, err)
		}
		if status != http.StatusOK || count == 0 {
			break
		}
		lastPage *= 2
	}

	// Binary search for the last page
	low, high := lastPage, lastPage*2
	for low <= high {
		mid := (low + high) / 2
		if mid == 0 { // Should not happen with our logic
			break
		}
		count, status, err := fetchMoviesInPage(context.Background(), server, port, year, mid, bearer)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch page %d for year %d: %w", mid, year, err)
		}
		if status == http.StatusOK && count > 0 {
			lastPage = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	// Get the number of movies on the last page
	moviesOnLastPage, status, err := fetchMoviesInPage(context.Background(), server, port, year, lastPage, bearer)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch last page %d for year %d: %w", lastPage, year, err)
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("failed to fetch last page %d for year %d. Status: %d", lastPage, year, status)
	}

	totalMovies := (lastPage-1)*moviesPerPage + moviesOnLastPage
	return totalMovies, nil
}

func main() {
	server = flag.String("s", "", "Server IP address for authentication (required)")
	port = flag.Int("P", 8080, "Server port for authentication (default: 8080)")
	username = flag.String("u", "", "Username for authentication (required)")
	password = flag.String("p", "", "Password for authentication (required)")
	flag.Var(&years, "Y", "Filter movie database by year (required, can be repeated)")
	verbose = flag.Bool("v", false, "Enable verbose output")

	// Custom parsing to allow single-dash long options like Python's argparse
	err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Println("Error parsing flags:", err)
		flag.Usage()
		os.Exit(1)
	}

	if *server == "" || *username == "" || *password == "" || len(years) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	bearer, err := authenticate(*server, *port, *username, *password)
	if err != nil {
		log.Fatalf("Authentication failed: %v", err)
	}

	printVerbose("Filtering movies by year(s): %s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(years)), ", "), "[]"))

	var wg sync.WaitGroup
	results := make(map[int]int)
	var mu sync.Mutex

	for _, year := range years {
		wg.Add(1)
		go func(y int) {
			defer wg.Done()
			count, err := fetchMoviesByYear(*server, *port, y, bearer, *username, *password)
			if err != nil {
				log.Printf("Failed to fetch movies for year %d: %v", y, err)
				return
			}
			mu.Lock()
			results[y] = count
			mu.Unlock()
		}(year)
	}

	wg.Wait()

	for _, year := range years {
		if count, ok := results[year]; ok {
			suffix := "s"
			if count == 1 {
				suffix = ""
			}
			fmt.Printf("Year %d: %d movie%s\n", year, count, suffix)
		}
	}
}

// parseFlags provides basic support for long-form flags with a single dash.
func parseFlags(args []string) error {
	var newArgs []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 2 {
			// Potentially a single-dash long option
			name := strings.Split(arg, "=")[0]
			if flag.Lookup(name[1:]) != nil {
				newArgs = append(newArgs, "-"+arg)
				continue
			}
		}
		newArgs = append(newArgs, arg)
	}
	return flag.CommandLine.Parse(newArgs)
}
