package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "modernc.org/sqlite"
)

func main() {
	// Load .env if present; ignore error if file doesn't exist.
	_ = godotenv.Load()

	tmdbKey := flag.String("tmdb-key", os.Getenv("TMDB_API_KEY"), "TMDB API bearer token (or TMDB_API_KEY in .env)")
	dbPath := flag.String("db", "tncb.db", "SQLite database file path")
	csvDir := flag.String("csv-dir", ".", "Directory to write CSV output files")
	bdmvPath := flag.String("bdmv", "", "Explicit BDMV root path (auto-detects if empty)")
	listFlag := flag.Bool("list", false, "Print all stored records and exit")
	flag.Parse()

	sqlDB, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("SQLite open %s: %v", *dbPath, err)
	}
	defer sqlDB.Close()

	store, err := NewStore(sqlDB, *csvDir)
	if err != nil {
		log.Fatalf("Store init: %v", err)
	}

	if *listFlag {
		if err := printDatabase(sqlDB); err != nil {
			log.Fatalf("List: %v", err)
		}
		return
	}

	if *tmdbKey == "" {
		log.Fatal("TMDB API key required: set TMDB_API_KEY in .env or use --tmdb-key")
	}

	session := time.Now().Format("20060102_150405")
	store.tvCSVPath = fmt.Sprintf("%s/tv_%s.csv", *csvDir, session)
	store.movieCSVPath = fmt.Sprintf("%s/movies_%s.csv", *csvDir, session)

	stdin := bufio.NewReader(os.Stdin)
	proc := &Processor{
		tmdbKey:  *tmdbKey,
		bdmvPath: *bdmvPath,
		stdin:    stdin,
		store:    store,
	}

	discNum := 1
	for {
		fmt.Printf("\n[m] Scan as movie  [t] Scan as TV  [Enter] Auto-detect  [p] Print database  [q] Quit\n> ")
		line, _ := stdin.ReadString('\n')
		choice := strings.TrimSpace(strings.ToLower(line))

		switch choice {
		case "q", "quit":
			fmt.Println("\nSession complete.")
			fmt.Printf("  TV CSV:    %s\n", store.tvCSVPath)
			fmt.Printf("  Movie CSV: %s\n", store.movieCSVPath)
			fmt.Printf("  Database:  %s\n", *dbPath)
			return

		case "p", "print":
			if err := printDatabase(sqlDB); err != nil {
				fmt.Fprintf(os.Stderr, "Print error: %v\n", err)
			}
			continue
		}

		// Determine forced type for m/t, nil for auto.
		var forceIsMovie *bool
		if choice == "m" || choice == "movie" {
			v := true
			forceIsMovie = &v
		} else if choice == "t" || choice == "tv" {
			v := false
			forceIsMovie = &v
		}

		fmt.Printf("\n=== Disc %d ===\n", discNum)
		result, bdmvRoot, err := proc.ProcessDisc(forceIsMovie)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}
		if err := store.Write(result); err != nil {
			fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
		}
		if err := ejectDisc(bdmvRoot); err != nil {
			fmt.Printf("Note: auto-eject failed: %v\n", err)
		} else {
			fmt.Println("Disc ejected.")
		}
		discNum++
	}
}
