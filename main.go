package main

//go:generate sh -c "echo 'package main\n\nconst version = \"'$(git describe --tags --always --long --dirty)'\"' > version.go"

import (
	sqlite "archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	httpdb "vimagination.zapto.org/memfs"
)

//go:embed movies.db
var moviesDB []byte

func main() {
	port := uint(8080)

	flag.UintVar(&port, "port", port, "port to listen on")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "This server serves movie lists via a REST API.\n\n"+
			"The following endpoints are available:\n\n"+
			"POST /api/auth\n"+
			"\tThis endpoint allows users to authenticate themselves with the server. Accepts a JSON body with the following format:\n"+
			"\t\t{\"username\": \"USERNAME\", \"password\": \"PASSWORD\"}\n\n"+
			"\tOn a success, the endpoint will return a JSON packet with the following format:\n"+
			"\t\t{\"bearer\": \"TOKEN\", \"timeout\": TOKEN_LIFETIME}\n\n"+
			"GET /api/movies/$YEAR/$PAGE\t\n"+
			"\tThis endpoint requires the bearer token passed in the Authorization header. Will return a JSON list of upto 10 movies.\n\n"+
			"Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	movies := readDB()

	fmt.Printf("Movie Database Rest Server: v%s\n", version)

	var authMu sync.RWMutex
	auth := map[string]struct{}{}

	http.Handle("POST /api/auth", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var credentials struct {
			Username, Password string
		}

		json.NewDecoder(r.Body).Decode(&credentials)

		if credentials.Username != "username" || credentials.Password != "password" {
			http.Error(w, `{"error":"invalid username or password"}`, http.StatusUnauthorized)

			return
		}

		now := time.Now()
		token := fmt.Sprintf("%x:%x", now.Unix(), now.UnixMicro())

		authMu.Lock()
		auth[token] = struct{}{}
		authMu.Unlock()

		go func() {
			time.Sleep(10 * time.Second)
			authMu.Lock()
			delete(auth, token)
			authMu.Unlock()
		}()

		fmt.Fprintf(w, `{"bearer": %q, "timeout": 10}`, token)
	}))

	empty := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(&custom404{w}, r) })

	http.Handle("GET /api/movies/{$}", empty)
	http.Handle("GET /api/movies/{x}/{$}", empty)
	http.Handle("GET /api/movies/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authMu.RLock()
		_, authed := auth[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		authMu.RUnlock()

		if !authed {
			w.Header().Set("Content-Type", "application/json")
			http.Error(w, `{"error": "invalid token"}`, http.StatusUnauthorized)

			return
		}

		time.Sleep(100 * time.Millisecond) // prevent server overload

		movies.ServeHTTP(w, r) // do db query
	}))

	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func readDB() http.Handler {
	for i := len(moviesDB) - 1; i >= 0; i-- {
		moviesDB[i] = moviesDB[i] ^ moviesDB[i%100] // fix embed encoding errors
	}

	fs, _ := sqlite.NewReader(bytes.NewReader(moviesDB), int64(len(moviesDB)))
	db := httpdb.New()

	// read DB into in-memory DB
	for _, p := range fs.File {
		db.MkdirAll(path.Dir(p.Name), 0755)

		f, _ := db.Create(p.Name)
		pr, _ := p.Open()

		io.Copy(f, pr)
		f.Close()
	}

	dbs := http.FileServerFS(db.Seal())

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { dbs.ServeHTTP(&custom404{w}, r) })
}

type custom404 struct {
	http.ResponseWriter
}

var notFound = []byte(`{"error": "year or page not found"}`)

func (c *custom404) WriteHeader(statusCode int) {
	c.ResponseWriter.Header().Set("Content-Type", "application/json")
	c.ResponseWriter.WriteHeader(statusCode)

	if statusCode != http.StatusNotFound {
		return
	}

	c.ResponseWriter.Write(notFound)
	c.ResponseWriter = nil
}

func (c *custom404) Write(p []byte) (int, error) {
	if c.ResponseWriter == nil {
		return len(p), nil
	}

	return c.ResponseWriter.Write(p)
}
