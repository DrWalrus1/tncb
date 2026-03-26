package main

import (
	"database/sql"
	"fmt"
	"strings"
)

// printDatabase prints all TV episodes and movies stored in the database.
func printDatabase(db *sql.DB) error {
	if err := printTVEpisodes(db); err != nil {
		return fmt.Errorf("tv_episodes: %w", err)
	}
	if err := printMovies(db); err != nil {
		return fmt.Errorf("movies: %w", err)
	}
	return nil
}

func printTVEpisodes(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT series_name, series_id, season_number, episode_number,
		       episode_id, playlist_id, clip_id, duration,
		       extracted_title, actual_title
		FROM tv_episodes
		ORDER BY series_name, season_number, episode_number
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type row struct {
		seriesName, playlistID, clipID, extractedTitle, actualTitle string
		seriesID, season, episode, episodeID, duration              int
	}

	var records []row
	for rows.Next() {
		var r row
		if err := rows.Scan(
			&r.seriesName, &r.seriesID, &r.season, &r.episode,
			&r.episodeID, &r.playlistID, &r.clipID, &r.duration,
			&r.extractedTitle, &r.actualTitle,
		); err != nil {
			return err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	fmt.Printf("\n%s\n", header("TV Episodes", 80))
	if len(records) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	currentSeries := ""
	currentSeason := 0
	for _, r := range records {
		if r.seriesName != currentSeries {
			currentSeries = r.seriesName
			currentSeason = 0
			seriesLabel := r.seriesName
			if r.seriesID > 0 {
				seriesLabel = fmt.Sprintf("%s (TMDB ID %d)", r.seriesName, r.seriesID)
			}
			fmt.Printf("\n  %s\n", seriesLabel)
		}
		if r.season != currentSeason {
			currentSeason = r.season
			fmt.Printf("    Season %d\n", r.season)
		}
		title := r.actualTitle
		if title == "" {
			title = r.extractedTitle
		}
		epID := ""
		if r.episodeID > 0 {
			epID = fmt.Sprintf(" [ep %d]", r.episodeID)
		}
		fmt.Printf("      E%02d  %-45s  playlist=%-8s clip=%-8s  %s%s\n",
			r.episode, truncate(title, 45), r.playlistID, r.clipID,
			formatDuration(r.duration), epID)
	}
	fmt.Printf("\n  Total: %d episode(s)\n", len(records))
	return nil
}

func printMovies(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT playlist_id, clip_id, duration, movie_id, extracted_title, actual_title
		FROM movies
		ORDER BY actual_title
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type row struct {
		playlistID, clipID, extractedTitle, actualTitle string
		movieID, duration                               int
	}

	var records []row
	for rows.Next() {
		var r row
		if err := rows.Scan(
			&r.playlistID, &r.clipID, &r.duration,
			&r.movieID, &r.extractedTitle, &r.actualTitle,
		); err != nil {
			return err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	fmt.Printf("\n%s\n", header("Movies", 80))
	if len(records) == 0 {
		fmt.Println("  (none)")
		return nil
	}

	for _, r := range records {
		title := r.actualTitle
		if title == "" {
			title = r.extractedTitle
		}
		tmdbID := ""
		if r.movieID > 0 {
			tmdbID = fmt.Sprintf(" [TMDB %d]", r.movieID)
		}
		fmt.Printf("  %-50s  playlist=%-8s clip=%-8s  %s%s\n",
			truncate(title, 50), r.playlistID, r.clipID,
			formatDuration(r.duration), tmdbID)
	}
	fmt.Printf("\n  Total: %d movie(s)\n", len(records))
	return nil
}

func header(title string, width int) string {
	line := strings.Repeat("─", width)
	return fmt.Sprintf("%s\n  %s\n%s", line, title, line)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func formatDuration(secs int) string {
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
