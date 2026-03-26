# Take Names, Chew Bubblegum

A CLI tool for cataloguing Blu-ray disc collections. Feed it discs one at a time and it will extract playlist/clip metadata using [spindrift](https://github.com/DrWalrus1/spindrift), cross-reference titles against TMDB, prompt for anything it can't identify automatically, eject the disc, and loop until you're done.

Results are written to a SQLite database and to per-session CSV files.

---

## Requirements

- Go 1.25+
- macOS or Linux
- A TMDB API Read Access Token ([get one here](https://www.themoviedb.org/settings/api))

---

## Installation

```sh
git clone https://github.com/DrWalrus1/tncb
cd tncb
go build -o tncb .
```

---

## Configuration

Create a `.env` file in the working directory:

```
TMDB_API_KEY=your_tmdb_read_access_token_here
```

All settings can also be passed as flags (flags take precedence over `.env`).

---

## Usage

### Scan discs

```sh
./tncb [flags]
```

Insert a disc, run the tool, and follow the prompts. After each disc is processed it is ejected automatically and the tool waits for you to confirm before scanning the next one. Type `q` at the prompt to exit.

**Flags**

| Flag | Default | Description |
|------|---------|-------------|
| `--tmdb-key` | `$TMDB_API_KEY` | TMDB API Read Access Token |
| `--db` | `tncb.db` | SQLite database file path |
| `--csv-dir` | `.` | Directory to write CSV output files |
| `--bdmv` | *(auto-detect)* | Explicit BDMV root path, e.g. `/Volumes/SHOW/BDMV` |

### Print the database

```sh
./tncb --list
./tncb --list --db /path/to/tncb.db
```

Prints all stored TV episodes (grouped by series and season) and movies, then exits. Does not require `TMDB_API_KEY`.

---

## How it works

1. **Disc detection** — spindrift locates the mounted BDMV directory and parses the disc title (e.g. `Breaking Bad Season 3 Disc 2`) to extract show name, season number, and disc number.

2. **Playlist analysis** — content playlists are filtered by estimated duration to identify individual episode streams, including multi-episode streams which are split into individual entries.

3. **TMDB lookup** — the extracted title is searched on TMDB. If a match is found the episode/movie metadata is fetched automatically.

4. **Fallback prompts** — if TMDB returns no match the tool will ask:
   - Whether the disc is a movie or TV show
   - The correct title (which triggers a second TMDB search)
   - If still unmatched: episode titles are entered manually one by one
   - For multi-disc TV sets on disc 2+: which episode number the disc starts from

5. **Output** — records are appended to a SQLite database and a CSV file for the current session. The disc is then ejected.

---

## Output

### SQLite tables

**`tv_episodes`**

| Column | Type | Description |
|--------|------|-------------|
| `disc_name` | TEXT | Raw disc title as read from the disc label |
| `season_number` | INTEGER | Season number parsed from disc |
| `episode_number` | INTEGER | Episode number from TMDB or manual entry |
| `playlist_id` | TEXT | BDMV playlist filename (e.g. `00800`) |
| `clip_id` | TEXT | Primary `.m2ts` clip name (e.g. `00801`) |
| `duration` | INTEGER | Estimated duration in seconds |
| `episode_id` | INTEGER | TMDB episode ID (0 if not found) |
| `series_name` | TEXT | Series name from TMDB or user input |
| `series_id` | INTEGER | TMDB series ID (0 if not found) |
| `extracted_title` | TEXT | Title constructed from disc label, e.g. `Breaking Bad S03E01` |
| `actual_title` | TEXT | Episode title from TMDB or user input |

**`movies`**

| Column | Type | Description |
|--------|------|-------------|
| `disc_name` | TEXT | Raw disc title as read from the disc label |
| `playlist_id` | TEXT | BDMV playlist filename |
| `clip_id` | TEXT | Primary `.m2ts` clip name |
| `duration` | INTEGER | Estimated duration in seconds |
| `movie_id` | INTEGER | TMDB movie ID (0 if not found) |
| `extracted_title` | TEXT | Title parsed from disc label |
| `actual_title` | TEXT | Movie title from TMDB or user input |

### CSV files

Two files are created in `--csv-dir` per session, named `tv_YYYYMMDD_HHMMSS.csv` and `movies_YYYYMMDD_HHMMSS.csv`. They contain the same columns as the tables above.
