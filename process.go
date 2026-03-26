package main

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/DrWalrus1/spindrift/bdmv"
	"github.com/DrWalrus1/spindrift/disc"
	"github.com/DrWalrus1/spindrift/tmdb"
)

// TVRecord holds metadata for one TV episode extracted from a Blu-ray disc.
type TVRecord struct {
	DiscName       string `bson:"disc_name"`
	SeasonNumber   int    `bson:"season_number"`
	EpisodeNumber  int    `bson:"episode_number"`
	PlaylistID     string `bson:"playlist_id"`
	ClipID         string `bson:"clip_id"`
	Duration       int    `bson:"duration_s"`
	EpisodeID      int    `bson:"episode_id"`
	SeriesName     string `bson:"series_name"`
	SeriesID       int    `bson:"series_id"`
	ExtractedTitle string `bson:"extracted_title"`
	ActualTitle    string `bson:"actual_title"`
}

// MovieRecord holds metadata for a movie extracted from a Blu-ray disc.
type MovieRecord struct {
	DiscName       string `bson:"disc_name"`
	PlaylistID     string `bson:"playlist_id"`
	ClipID         string `bson:"clip_id"`
	Duration       int    `bson:"duration_s"`
	MovieID        int    `bson:"movie_id"`
	ExtractedTitle string `bson:"extracted_title"`
	ActualTitle    string `bson:"actual_title"`
}

// DiscResult is the processed output from a single disc.
type DiscResult struct {
	IsMovie  bool
	Episodes []TVRecord
	Movie    *MovieRecord
}

// Processor scans discs and resolves metadata via TMDB and user prompts.
type Processor struct {
	tmdbKey  string
	bdmvPath string
	stdin    *bufio.Reader
}

// ProcessDisc scans the current disc and returns enriched metadata along with
// the BDMV root path (needed for ejection).
// forceIsMovie overrides auto-detection when non-nil.
func (p *Processor) ProcessDisc(forceIsMovie *bool) (*DiscResult, string, error) {
	bdmvRoot, err := disc.SelectBDMV(p.bdmvPath)
	if err != nil {
		return nil, "", fmt.Errorf("detecting disc: %w", err)
	}
	fmt.Printf("BDMV root: %s\n", bdmvRoot)

	d, err := disc.Open(bdmvRoot)
	if err != nil {
		return nil, bdmvRoot, fmt.Errorf("opening disc: %w", err)
	}
	discName, err := disc.ParseDiscTitle(bdmvRoot)
	if err != nil || discName == "" {
		discName = d.Info.ShowName
	}
	fmt.Printf("Disc: %q  (season %d, disc %d)\n", discName, d.Info.Season, d.Info.Disc)

	minDur, maxDur, clusterDur := disc.InferEpisodeBounds(bdmvRoot)
	playlists, err := disc.LoadEpisodePlaylists(bdmvRoot, minDur, maxDur, clusterDur)
	if err != nil {
		return nil, bdmvRoot, fmt.Errorf("loading playlists: %w", err)
	}
	if len(playlists) == 0 {
		return nil, bdmvRoot, fmt.Errorf("no content playlists found on disc")
	}
	fmt.Printf("Found %d content playlist(s)\n", len(playlists))

	d.Info.DetectMovie(len(playlists))
	client := tmdb.New(p.tmdbKey)

	// Use forced type if provided; otherwise auto-detect via TMDB.
	isMovie := d.Info.IsMovie
	if forceIsMovie != nil {
		isMovie = *forceIsMovie
	} else if !p.quickTMDBCheck(d.Info.ShowName, isMovie, client) {
		isMovie = p.promptConfirmType(d.Info.ShowName, isMovie)
	}
	d.Info.IsMovie = isMovie

	result := &DiscResult{IsMovie: isMovie}
	if isMovie {
		rec, err := p.processMovie(d.Info, playlists, client, bdmvRoot, discName)
		if err != nil {
			return nil, bdmvRoot, err
		}
		result.Movie = rec
	} else {
		eps, err := p.processTV(d.Info, playlists, client, bdmvRoot, discName)
		if err != nil {
			return nil, bdmvRoot, err
		}
		result.Episodes = eps
	}
	return result, bdmvRoot, nil
}

// quickTMDBCheck returns true if TMDB has any results for the title+type.
func (p *Processor) quickTMDBCheck(name string, isMovie bool, client *tmdb.Client) bool {
	if isMovie {
		movies, err := client.SearchMovie(name)
		return err == nil && len(movies) > 0
	}
	shows, err := client.SearchTV(name)
	return err == nil && len(shows) > 0
}

// promptConfirmType asks the user to confirm whether the disc is a movie or TV show.
func (p *Processor) promptConfirmType(name string, detectedMovie bool) bool {
	detected := "TV show"
	if detectedMovie {
		detected = "movie"
	}
	fmt.Printf("No TMDB match for %q (detected as %s).\n", name, detected)
	fmt.Print("Is this a [m]ovie or [t]v show? [m/t]: ")
	line, _ := p.stdin.ReadString('\n')
	input := strings.TrimSpace(strings.ToLower(line))
	return input == "" || input == "m" || input == "movie"
}

// processTV resolves TV episode metadata and builds records for each playlist.
func (p *Processor) processTV(info disc.DiscInfo, playlists []*bdmv.Playlist, client *tmdb.Client, bdmvRoot, discName string) ([]TVRecord, error) {
	seriesID, seriesName, tmdbEps := p.lookupTV(info, len(playlists), client)

	// Pad with empty entries so we always have one per playlist slot.
	for i := len(tmdbEps); i < len(playlists); i++ {
		tmdbEps = append(tmdbEps, tmdb.Episode{EpisodeNumber: i + 1})
	}

	records := make([]TVRecord, 0, len(playlists))
	for i, pl := range playlists {
		ep := tmdbEps[i]
		records = append(records, TVRecord{
			DiscName:       discName,
			SeasonNumber:   info.Season,
			EpisodeNumber:  ep.EpisodeNumber,
			PlaylistID:     pl.Name,
			ClipID:         pl.PrimaryClip(),
			Duration:       pl.EstimateDuration(bdmvRoot, disc.DefaultBitrate),
			EpisodeID:      ep.ID,
			SeriesName:     seriesName,
			SeriesID:       seriesID,
			ExtractedTitle: fmt.Sprintf("%s S%02dE%02d", seriesName, info.Season, ep.EpisodeNumber),
			ActualTitle:    ep.Name,
		})
	}
	return records, nil
}

// lookupTV searches TMDB for the series and fetches the right episode slice.
// Returns (seriesID, seriesName, episodes).
func (p *Processor) lookupTV(info disc.DiscInfo, wantCount int, client *tmdb.Client) (int, string, []tmdb.Episode) {
	shows, err := client.SearchTV(info.ShowName)
	if err == nil && len(shows) > 0 {
		show := shows[0]
		fmt.Printf("TMDB match: %q (ID %d)\n", show.Name, show.ID)
		eps := p.fetchSeasonEpisodes(client, show.ID, info, wantCount)
		return show.ID, show.Name, eps
	}
	// No TMDB hit — manual entry (type was already confirmed in ProcessDisc).
	return p.promptTVManual(info, wantCount, client)
}

// fetchSeasonEpisodes retrieves the correct episode slice for this disc.
func (p *Processor) fetchSeasonEpisodes(client *tmdb.Client, showID int, info disc.DiscInfo, wantCount int) []tmdb.Episode {
	season, err := client.GetSeason(showID, info.Season)
	if err != nil {
		fmt.Printf("Warning: could not fetch season %d: %v\n", info.Season, err)
		return nil
	}
	startEp := 0
	if info.Disc > 1 {
		fmt.Printf("Disc %d of season %d — first episode number on this disc (1–%d): ",
			info.Disc, info.Season, len(season.Episodes))
		line, _ := p.stdin.ReadString('\n')
		n := 0
		fmt.Sscanf(strings.TrimSpace(line), "%d", &n)
		if n >= 1 {
			startEp = n
		}
	}
	return tmdb.EpisodesForDisc(season, startEp, wantCount)
}

// promptTVManual handles fully manual title entry for an unmatched TV disc.
func (p *Processor) promptTVManual(info disc.DiscInfo, numEps int, client *tmdb.Client) (int, string, []tmdb.Episode) {
	fmt.Printf("Enter series name [%s]: ", info.ShowName)
	line, _ := p.stdin.ReadString('\n')
	name := strings.TrimSpace(line)
	if name == "" {
		name = info.ShowName
	}

	// Try TMDB once more with the user-supplied name.
	if shows, err := client.SearchTV(name); err == nil && len(shows) > 0 {
		show := shows[0]
		fmt.Printf("Found: %q (ID %d) — use this? [Y/n]: ", show.Name, show.ID)
		confirm, _ := p.stdin.ReadString('\n')
		if c := strings.TrimSpace(strings.ToLower(confirm)); c == "" || c == "y" {
			eps := p.fetchSeasonEpisodes(client, show.ID, info, numEps)
			return show.ID, show.Name, eps
		}
	}

	// Fully manual per-episode title entry.
	fmt.Printf("Enter titles for %d episode(s):\n", numEps)
	eps := make([]tmdb.Episode, 0, numEps)
	for i := 1; i <= numEps; i++ {
		fmt.Printf("  Episode %d title: ", i)
		title, _ := p.stdin.ReadString('\n')
		eps = append(eps, tmdb.Episode{EpisodeNumber: i, Name: strings.TrimSpace(title)})
	}
	return 0, name, eps
}

// processMovie resolves movie metadata and builds the record.
func (p *Processor) processMovie(info disc.DiscInfo, playlists []*bdmv.Playlist, client *tmdb.Client, bdmvRoot, discName string) (*MovieRecord, error) {
	pl := playlists[0]
	rec := &MovieRecord{
		DiscName:       discName,
		PlaylistID:     pl.Name,
		ClipID:         pl.PrimaryClip(),
		Duration:       pl.EstimateDuration(bdmvRoot, disc.DefaultBitrate),
		ExtractedTitle: info.ShowName,
	}

	movies, err := client.SearchMovie(info.ShowName)
	if err == nil && len(movies) > 0 {
		fmt.Printf("TMDB match: %q (ID %d)\n", movies[0].Title, movies[0].ID)
		rec.MovieID = movies[0].ID
		rec.ActualTitle = movies[0].Title
		return rec, nil
	}

	// Manual title entry (type was already confirmed in ProcessDisc).
	rec.MovieID, rec.ActualTitle = p.promptMovieManual(info.ShowName, client)
	return rec, nil
}

// promptMovieManual collects a movie title when TMDB search fails.
func (p *Processor) promptMovieManual(extractedName string, client *tmdb.Client) (int, string) {
	fmt.Printf("Enter movie title [%s]: ", extractedName)
	line, _ := p.stdin.ReadString('\n')
	title := strings.TrimSpace(line)
	if title == "" {
		title = extractedName
	}

	// Try TMDB with user-supplied title.
	if movies, err := client.SearchMovie(title); err == nil && len(movies) > 0 {
		fmt.Printf("Found: %q (ID %d) — use this? [Y/n]: ", movies[0].Title, movies[0].ID)
		confirm, _ := p.stdin.ReadString('\n')
		if c := strings.TrimSpace(strings.ToLower(confirm)); c == "" || c == "y" {
			return movies[0].ID, movies[0].Title
		}
	}
	return 0, title
}
