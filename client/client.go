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
	"sync/atomic"
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

// fetchMoviesByYear orchestrates fetching all movies for a given year, handling concurrency and re-authentication.
func fetchMoviesByYear(server string, port, year int, bearer, user, pass string) (int, error) {
	var totalMovieCount int64
	currentPage := 1
	currentBearer := bearer

	// Outer loop to handle re-authentication attempts.
	for {
		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pageChan := make(chan int, 100) // Buffer for pages to fetch

		var reauthRequired int32

		// Worker goroutines
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for page := range pageChan {
					if atomic.LoadInt32(&reauthRequired) > 0 {
						return
					}
					count, status, err := fetchMoviesInPage(ctx, server, port, year, page, currentBearer)
					if err != nil {
						log.Printf("Error fetching page %d for year %d: %v", page, year, err)
						cancel()
						return
					}

					if status == http.StatusUnauthorized {
						atomic.StoreInt32(&reauthRequired, 1)
						cancel() // Stop all workers
						return
					}

					if status != http.StatusOK || count == 0 {
						cancel() // Found the end or an error, stop all workers
						return
					}
					atomic.AddInt64(&totalMovieCount, int64(count))
				}
			}()
		}

		// Producer goroutine
		producerDone := make(chan struct{})
		go func() {
			defer close(producerDone)
			defer close(pageChan)
			page := currentPage
			for {
				select {
				case pageChan <- page:
					page++
				case <-ctx.Done():
					return
				}
			}
		}()

		<-ctx.Done() // Wait until cancellation is triggered

		// Ensure producer is done before waiting on workers
		<-producerDone
		wg.Wait()

		if atomic.LoadInt32(&reauthRequired) > 0 {
			printVerbose("Session expired, re-authenticating...")
			newBearer, err := authenticate(server, port, user, pass)
			if err != nil {
				return 0, fmt.Errorf("failed to re-authenticate: %w", err)
			}
			currentBearer = newBearer
			// The page that failed was not processed, so we can just continue.
			// A more robust implementation might restart from the failed page.
			continue
		}

		// If we are here, it means we finished successfully or with a non-auth error.
		cancel()
		break
	}

	return int(totalMovieCount), nil
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
