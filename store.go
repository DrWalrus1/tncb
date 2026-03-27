package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
)

// Store writes disc records to CSV files and a SQLite database.
type Store struct {
	db           *sql.DB
	tvCSVPath    string
	movieCSVPath string
}

// NewStore creates a Store and ensures the database tables exist.
func NewStore(db *sql.DB, csvDir string) (*Store, error) {
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// migrate creates the tv_episodes and movies tables if they don't exist,
// and adds disc_name to existing databases that predate the column.
func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tv_episodes (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			disc_name       TEXT    NOT NULL DEFAULT '',
			season_number   INTEGER NOT NULL,
			episode_number  INTEGER NOT NULL,
			playlist_id     TEXT    NOT NULL,
			clip_id         TEXT    NOT NULL,
			duration        INTEGER NOT NULL,
			episode_id      INTEGER NOT NULL DEFAULT 0,
			series_name     TEXT    NOT NULL,
			series_id       INTEGER NOT NULL DEFAULT 0,
			extracted_title TEXT    NOT NULL,
			actual_title    TEXT    NOT NULL
		);
		CREATE TABLE IF NOT EXISTS movies (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			disc_name       TEXT    NOT NULL DEFAULT '',
			playlist_id     TEXT    NOT NULL,
			clip_id         TEXT    NOT NULL,
			duration        INTEGER NOT NULL,
			movie_id        INTEGER NOT NULL DEFAULT 0,
			extracted_title TEXT    NOT NULL,
			actual_title    TEXT    NOT NULL
		);
	`)
	return err
}

// Write persists a DiscResult to the appropriate CSV file and SQLite table.
func (s *Store) Write(result *DiscResult) error {
	if result.IsMovie && result.Movie != nil {
		if err := s.writeMovieCSV(result.Movie); err != nil {
			return fmt.Errorf("movie CSV: %w", err)
		}
		if err := s.insertMovie(result.Movie); err != nil {
			return fmt.Errorf("movie DB: %w", err)
		}
		fmt.Printf("Stored movie %q\n", result.Movie.ActualTitle)
		return nil
	}

	if len(result.Episodes) > 0 {
		if err := s.writeTVCSV(result.Episodes); err != nil {
			return fmt.Errorf("TV CSV: %w", err)
		}
		if err := s.insertEpisodes(result.Episodes); err != nil {
			return fmt.Errorf("TV DB: %w", err)
		}
		fmt.Printf("Stored %d episode(s) of %q season %d\n",
			len(result.Episodes), result.Episodes[0].SeriesName, result.Episodes[0].SeasonNumber)
	}
	return nil
}

// writeTVCSV appends TV episode rows, writing a header if the file is new.
func (s *Store) writeTVCSV(episodes []TVRecord) error {
	isNew := !fileExists(s.tvCSVPath)
	f, err := os.OpenFile(s.tvCSVPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if isNew {
		if err := w.Write([]string{
			"Disc Name", "Season Number", "Episode Number", "Playlist ID", "Clip ID",
			"Duration", "Episode ID", "Series Name", "Series ID",
			"Extracted Episode Title", "Actual Title",
		}); err != nil {
			return err
		}
	}
	for _, ep := range episodes {
		if err := w.Write([]string{
			ep.DiscName,
			strconv.Itoa(ep.SeasonNumber),
			strconv.Itoa(ep.EpisodeNumber),
			ep.PlaylistID,
			ep.ClipID,
			strconv.Itoa(ep.Duration),
			strconv.Itoa(ep.EpisodeID),
			ep.SeriesName,
			strconv.Itoa(ep.SeriesID),
			ep.ExtractedTitle,
			ep.ActualTitle,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// writeMovieCSV appends a movie row, writing a header if the file is new.
func (s *Store) writeMovieCSV(m *MovieRecord) error {
	isNew := !fileExists(s.movieCSVPath)
	f, err := os.OpenFile(s.movieCSVPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if isNew {
		if err := w.Write([]string{
			"Disc Name", "Playlist ID", "Clip ID", "Duration", "Movie ID",
			"Extracted Title", "Actual Title",
		}); err != nil {
			return err
		}
	}
	if err := w.Write([]string{
		m.DiscName,
		m.PlaylistID,
		m.ClipID,
		strconv.Itoa(m.Duration),
		strconv.Itoa(m.MovieID),
		m.ExtractedTitle,
		m.ActualTitle,
	}); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// insertEpisodes inserts TV episode records into the tv_episodes table.
func (s *Store) insertEpisodes(episodes []TVRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO tv_episodes
			(disc_name, season_number, episode_number, playlist_id, clip_id, duration,
			 episode_id, series_name, series_id, extracted_title, actual_title)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ep := range episodes {
		if _, err := stmt.Exec(
			ep.DiscName, ep.SeasonNumber, ep.EpisodeNumber, ep.PlaylistID, ep.ClipID, ep.Duration,
			ep.EpisodeID, ep.SeriesName, ep.SeriesID, ep.ExtractedTitle, ep.ActualTitle,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// insertMovie inserts a movie record into the movies table.
func (s *Store) insertMovie(m *MovieRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO movies (disc_name, playlist_id, clip_id, duration, movie_id, extracted_title, actual_title)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, m.DiscName, m.PlaylistID, m.ClipID, m.Duration, m.MovieID, m.ExtractedTitle, m.ActualTitle)
	return err
}

// CountEpisodesBySeason returns the number of episodes already stored for the
// given TMDB series ID and season number.
func (s *Store) CountEpisodesBySeason(seriesID, season int) int {
	var count int
	s.db.QueryRow(
		`SELECT COUNT(*) FROM tv_episodes WHERE series_id = ? AND season_number = ?`,
		seriesID, season,
	).Scan(&count)
	return count
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
