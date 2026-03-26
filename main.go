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
	flag.Parse()

	if *tmdbKey == "" {
		log.Fatal("TMDB API key required: set TMDB_API_KEY in .env or use --tmdb-key")
	}

	sqlDB, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("SQLite open %s: %v", *dbPath, err)
	}
	defer sqlDB.Close()

	store, err := NewStore(sqlDB, *csvDir)
	if err != nil {
		log.Fatalf("Store init: %v", err)
	}

	session := time.Now().Format("20060102_150405")
	store.tvCSVPath = fmt.Sprintf("%s/tv_%s.csv", *csvDir, session)
	store.movieCSVPath = fmt.Sprintf("%s/movies_%s.csv", *csvDir, session)

	stdin := bufio.NewReader(os.Stdin)
	proc := &Processor{
		tmdbKey:  *tmdbKey,
		bdmvPath: *bdmvPath,
		stdin:    stdin,
	}

	for discNum := 1; ; discNum++ {
		fmt.Printf("\n=== Disc %d ===\n", discNum)

		result, bdmvRoot, err := proc.ProcessDisc()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		} else {
			if err := store.Write(result); err != nil {
				fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
			}
			if err := ejectDisc(bdmvRoot); err != nil {
				fmt.Printf("Note: auto-eject failed: %v\n", err)
			} else {
				fmt.Println("Disc ejected.")
			}
		}

		fmt.Print("\nInsert next disc and press Enter to continue (or 'q' to quit): ")
		line, _ := stdin.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) == "q" {
			break
		}
	}

	fmt.Println("\nSession complete.")
	fmt.Printf("  TV CSV:    %s\n", store.tvCSVPath)
	fmt.Printf("  Movie CSV: %s\n", store.movieCSVPath)
	fmt.Printf("  Database:  %s\n", *dbPath)
}
