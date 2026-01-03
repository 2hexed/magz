package main

import (
	"embed"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"archive/zip"

	"golang.org/x/image/draw"
	_ "modernc.org/sqlite"

	"github.com/nwaples/rardecode"
)

//go:embed frontend/*
var staticFiles embed.FS

// Config represents application configuration
type Config struct {
	Port                int      `json:"Port"`
	AutoRefreshInterval int      `json:"AutoRefreshInterval"`
	LibraryPaths        []string `json:"LibraryPaths"`
	CacheDB             string   `json:"CacheDB"`
	MaxThumbnailSize    int      `json:"MaxThumbnailSize"`
	LogLevel            string   `json:"LogLevel"`
}

// LibraryItem represents a magazine/book entry
type LibraryItem struct {
	ID        int      `json:"id"`
	Category  string   `json:"category"`
	Title     string   `json:"title"`
	Path      string   `json:"path"`
	Cover     string   `json:"cover"`
	CoverData string   `json:"coverData"`
	LastMod   string   `json:"lastModified"`
	Pages     []string `json:"pages,omitempty"`
}

// Logger provides structured logging
type Logger struct {
	level string
}

func (l *Logger) Info(msg string, args ...interface{}) {
	log.Printf("[INFO] "+msg, args...)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	log.Printf("[ERROR] "+msg, args...)
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	if l.level == "debug" {
		log.Printf("[DEBUG] "+msg, args...)
	}
}

var (
	config Config
	db     *sql.DB
	logger *Logger
	// Rate limiter for thumbnail generation
	thumbSemaphore chan struct{}
)

// validateConfig checks if the configuration is valid
func validateConfig(cfg *Config) error {
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", cfg.Port)
	}
	if cfg.AutoRefreshInterval < 1 {
		return fmt.Errorf("invalid refresh interval: %d", cfg.AutoRefreshInterval)
	}
	if len(cfg.LibraryPaths) == 0 {
		return fmt.Errorf("no library paths specified")
	}
	for _, path := range cfg.LibraryPaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("library path does not exist: %s", path)
		}
	}
	if cfg.CacheDB == "" {
		cfg.CacheDB = "magz_cache.db"
	}
	if cfg.MaxThumbnailSize == 0 {
		cfg.MaxThumbnailSize = 400
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	return nil
}

// loadConfig reads and validates configuration
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// initDatabase sets up the database schema
func initDatabase(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool parameters
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	schema := `
		CREATE TABLE IF NOT EXISTS library (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			category TEXT,
			title TEXT,
			path TEXT UNIQUE,
			cover TEXT,
			coverData TEXT,
			lastModified TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_category ON library(category);
		CREATE INDEX IF NOT EXISTS idx_title ON library(title);
	`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return db, nil
}

// isPathAllowed checks if the path is within allowed library paths
func isPathAllowed(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, base := range config.LibraryPaths {
		cleanBase := filepath.Clean(base)
		if strings.HasPrefix(cleanPath, cleanBase) {
			return true
		}
	}
	return false
}

// getImagesFromCBR extracts image list from CBR archive
func getImagesFromCBR(cbrPath string) ([]string, error) {
	f, err := os.Open(cbrPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CBR: %w", err)
	}
	defer f.Close()

	rr, err := rardecode.NewReader(f, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create RAR reader: %w", err)
	}

	var pages []string
	for {
		h, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading RAR entry: %w", err)
		}
		name := strings.ToLower(h.Name)
		if isImageFile(name) && !strings.HasPrefix(filepath.Base(name), ".") {
			pages = append(pages, h.Name)
		}
	}

	sort.Slice(pages, func(i, j int) bool { return naturalLess(pages[i], pages[j]) })
	return pages, nil
}

// readImageFromCBR reads a specific image from CBR archive
func readImageFromCBR(cbrPath, imgName string) (image.Image, error) {
	f, err := os.Open(cbrPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rr, err := rardecode.NewReader(f, "")
	if err != nil {
		return nil, err
	}

	for {
		h, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Name == imgName {
			img, _, err := image.Decode(rr)
			if err != nil {
				return nil, fmt.Errorf("failed to decode image: %w", err)
			}
			return img, nil
		}
	}

	return nil, fmt.Errorf("image not found: %s", imgName)
}

// getImagesFromCBZ extracts image list from CBZ archive
func getImagesFromCBZ(cbzPath string) ([]string, error) {
	r, err := zip.OpenReader(cbzPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CBZ: %w", err)
	}
	defer r.Close()

	var pages []string
	for _, f := range r.File {
		name := strings.ToLower(f.Name)
		if isImageFile(name) && !strings.HasPrefix(filepath.Base(name), ".") {
			pages = append(pages, f.Name)
		}
	}

	sort.Slice(pages, func(i, j int) bool { return naturalLess(pages[i], pages[j]) })
	return pages, nil
}

// readImageFromCBZ reads a specific image from CBZ archive
func readImageFromCBZ(cbzPath, imgName string) (image.Image, error) {
	r, err := zip.OpenReader(cbzPath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name == imgName {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			img, _, err := image.Decode(rc)
			if err != nil {
				return nil, fmt.Errorf("failed to decode image: %w", err)
			}
			return img, nil
		}
	}
	return nil, fmt.Errorf("image not found: %s", imgName)
}

// isImageFile checks if the file is a supported image
func isImageFile(name string) bool {
	return strings.HasSuffix(name, ".jpg") ||
		strings.HasSuffix(name, ".jpeg") ||
		strings.HasSuffix(name, ".png") ||
		strings.HasSuffix(name, ".webp") ||
		strings.HasSuffix(name, ".avif") ||
		strings.HasSuffix(name, ".gif")
}

// generateThumbnailBase64 creates a thumbnail from file path
func generateThumbnailBase64(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}

	return imageToThumbnailBase64(src, config.MaxThumbnailSize)
}

// imageToThumbnailBase64 converts image to base64 thumbnail
func imageToThumbnailBase64(src image.Image, maxDim int) (string, error) {
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()

	if w == 0 || h == 0 {
		return "", fmt.Errorf("invalid image dimensions")
	}

	var targetW, targetH int
	if w >= h {
		targetH = maxDim
		targetW = int(float64(w) * (float64(maxDim) / float64(h)))
	} else {
		targetW = maxDim
		targetH = int(float64(h) * (float64(maxDim) / float64(w)))
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return "", err
	}

	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// naturalLess compares strings with natural number ordering
func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		if isDigit(a[ai]) && isDigit(b[bi]) {
			startA, startB := ai, bi
			for ai < len(a) && isDigit(a[ai]) {
				ai++
			}
			for bi < len(b) && isDigit(b[bi]) {
				bi++
			}
			na, _ := strconv.Atoi(a[startA:ai])
			nb, _ := strconv.Atoi(b[startB:bi])
			if na != nb {
				return na < nb
			}
		} else {
			if a[ai] != b[bi] {
				return a[ai] < b[bi]
			}
			ai++
			bi++
		}
	}
	return len(a) < len(b)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// selectCoverImage finds the best cover image from page list
func selectCoverImage(pages []string) string {
	// Look for files with "cover" in the name
	for _, p := range pages {
		lower := strings.ToLower(filepath.Base(p))
		if strings.Contains(lower, "cover") {
			return p
		}
	}
	// Look for files starting with "00" or "01"
	for _, p := range pages {
		base := filepath.Base(p)
		if strings.HasPrefix(base, "00") || strings.HasPrefix(base, "01") {
			return p
		}
	}
	// Return first page
	if len(pages) > 0 {
		return pages[0]
	}
	return ""
}

// processCBZ handles CBZ file scanning
func processCBZ(path string, existing map[string]string, seen map[string]bool, newCount, updatedCount int) (int, int) {
	info, err := os.Stat(path)
	if err != nil {
		logger.Error("Failed to stat CBZ: %v", err)
		return newCount, updatedCount
	}

	lastMod := info.ModTime().Format(time.RFC3339)
	prevMod, exists := existing[path]
	seen[path] = true

	category := filepath.Base(filepath.Dir(path))
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	var coverData string
	if !exists || prevMod != lastMod {
		pages, err := getImagesFromCBZ(path)
		if err != nil {
			logger.Error("Failed to read CBZ pages: %v", err)
		} else if len(pages) > 0 {
			cover := selectCoverImage(pages)
			img, err := readImageFromCBZ(path, cover)
			if err == nil {
				coverData, _ = imageToThumbnailBase64(img, config.MaxThumbnailSize)
			}
		}
	}

	if exists {
		if prevMod != lastMod {
			_, err := db.Exec(`UPDATE library SET category=?, title=?, cover=?, coverData=?, lastModified=?, updated_at=CURRENT_TIMESTAMP WHERE path=?`,
				category, title, "(cbz internal)", coverData, lastMod, path)
			if err != nil {
				logger.Error("Failed to update CBZ entry: %v", err)
			} else {
				updatedCount++
			}
		}
	} else {
		_, err := db.Exec(`INSERT INTO library (category, title, path, cover, coverData, lastModified)
			VALUES (?, ?, ?, ?, ?, ?)`,
			category, title, path, "(cbz internal)", coverData, lastMod)
		if err != nil {
			logger.Error("Failed to insert CBZ entry: %v", err)
		} else {
			newCount++
		}
	}

	return newCount, updatedCount
}

// processCBR handles CBR file scanning
func processCBR(path string, existing map[string]string, seen map[string]bool, newCount, updatedCount int) (int, int) {
	info, err := os.Stat(path)
	if err != nil {
		logger.Error("Failed to stat CBR: %v", err)
		return newCount, updatedCount
	}

	lastMod := info.ModTime().Format(time.RFC3339)
	prevMod, exists := existing[path]
	seen[path] = true

	category := filepath.Base(filepath.Dir(path))
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	var coverData string
	if !exists || prevMod != lastMod {
		pages, err := getImagesFromCBR(path)
		if err != nil {
			logger.Error("Failed to read CBR pages: %v", err)
		} else if len(pages) > 0 {
			cover := selectCoverImage(pages)
			img, err := readImageFromCBR(path, cover)
			if err == nil {
				coverData, _ = imageToThumbnailBase64(img, config.MaxThumbnailSize)
			}
		}
	}

	if exists {
		if prevMod != lastMod {
			_, err := db.Exec(`UPDATE library SET category=?, title=?, cover=?, coverData=?, lastModified=?, updated_at=CURRENT_TIMESTAMP WHERE path=?`,
				category, title, "(cbr internal)", coverData, lastMod, path)
			if err != nil {
				logger.Error("Failed to update CBR entry: %v", err)
			} else {
				updatedCount++
			}
		}
	} else {
		_, err := db.Exec(`INSERT INTO library (category, title, path, cover, coverData, lastModified)
			VALUES (?, ?, ?, ?, ?, ?)`,
			category, title, path, "(cbr internal)", coverData, lastMod)
		if err != nil {
			logger.Error("Failed to insert CBR entry: %v", err)
		} else {
			newCount++
		}
	}

	return newCount, updatedCount
}

// buildCache scans library directories and updates cache
func buildCache() {
	logger.Info("🔄 Scanning libraries...")
	startTime := time.Now()

	existing := make(map[string]string)
	rows, err := db.Query("SELECT path, lastModified FROM library")
	if err != nil {
		logger.Error("Failed to query existing entries: %v", err)
		return
	}
	for rows.Next() {
		var path, mod string
		rows.Scan(&path, &mod)
		existing[path] = mod
	}
	rows.Close()

	newCount := 0
	updatedCount := 0
	seen := make(map[string]bool)
	mu := sync.Mutex{}

	// Use worker pool for parallel processing
	var wg sync.WaitGroup
	workChan := make(chan string, 100)

	// Start workers
	numWorkers := 4
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range workChan {
				processPath(path, existing, seen, &newCount, &updatedCount, &mu)
			}
		}()
	}

	// Walk directories and send to workers
	for _, base := range config.LibraryPaths {
		filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			workChan <- path
			return nil
		})
	}

	close(workChan)
	wg.Wait()

	// Remove deleted entries
	deletedCount := 0
	for path := range existing {
		if !seen[path] {
			_, err := db.Exec("DELETE FROM library WHERE path=?", path)
			if err != nil {
				logger.Error("Failed to delete entry: %v", err)
			} else {
				deletedCount++
			}
		}
	}

	duration := time.Since(startTime)
	logger.Info("✅ Cache updated in %v — %d new, %d updated, %d removed", duration, newCount, updatedCount, deletedCount)
}

// processPath handles individual path processing
func processPath(path string, existing map[string]string, seen map[string]bool, newCount, updatedCount *int, mu *sync.Mutex) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	lower := strings.ToLower(info.Name())

	// Handle CBZ files
	if strings.HasSuffix(lower, ".cbz") {
		mu.Lock()
		n, u := processCBZ(path, existing, seen, *newCount, *updatedCount)
		*newCount = n
		*updatedCount = u
		mu.Unlock()
		return
	}

	// Handle CBR files
	if strings.HasSuffix(lower, ".cbr") {
		mu.Lock()
		n, u := processCBR(path, existing, seen, *newCount, *updatedCount)
		*newCount = n
		*updatedCount = u
		mu.Unlock()
		return
	}

	// Handle directories with images
	if !info.IsDir() {
		return
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}

	var pages []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if isImageFile(name) && !strings.HasPrefix(e.Name(), ".") {
			pages = append(pages, e.Name())
		}
	}

	if len(pages) == 0 {
		return
	}

	sort.Slice(pages, func(i, j int) bool { return naturalLess(pages[i], pages[j]) })

	cover := selectCoverImage(pages)
	coverPath := filepath.Join(path, cover)
	lastMod := info.ModTime().Format(time.RFC3339)

	mu.Lock()
	prevMod, exists := existing[path]
	seen[path] = true
	mu.Unlock()

	category := filepath.Base(filepath.Dir(path))
	title := filepath.Base(path)

	coverData := ""
	if !exists || prevMod != lastMod {
		// Use semaphore to limit concurrent thumbnail generation
		thumbSemaphore <- struct{}{}
		if data, err := generateThumbnailBase64(coverPath); err == nil {
			coverData = data
		} else {
			logger.Debug("Failed to generate thumbnail for %s: %v", coverPath, err)
		}
		<-thumbSemaphore
	}

	mu.Lock()
	defer mu.Unlock()

	if exists {
		if prevMod != lastMod {
			_, err := db.Exec(`UPDATE library SET category=?, title=?, cover=?, coverData=?, lastModified=?, updated_at=CURRENT_TIMESTAMP WHERE path=?`,
				category, title, cover, coverData, lastMod, path)
			if err != nil {
				logger.Error("Failed to update directory entry: %v", err)
			} else {
				*updatedCount++
			}
		}
	} else {
		_, err := db.Exec(`INSERT INTO library (category, title, path, cover, coverData, lastModified)
			VALUES (?, ?, ?, ?, ?, ?)`,
			category, title, path, cover, coverData, lastMod)
		if err != nil {
			logger.Error("Failed to insert directory entry: %v", err)
		} else {
			*newCount++
		}
	}
}

// HTTP Handlers

// handleMedia serves media files with security checks
func handleMedia(w http.ResponseWriter, r *http.Request) {
	cbrPath := r.URL.Query().Get("cbr")
	cbzPath := r.URL.Query().Get("cbz")
	pageName := r.URL.Query().Get("page")

	// Serve CBZ pages
	if cbzPath != "" && pageName != "" {
		if !isPathAllowed(cbzPath) {
			logger.Error("Unauthorized CBZ access attempt: %s", cbzPath)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		serveCBZPage(w, cbzPath, pageName)
		return
	}

	// Serve CBR pages
	if cbrPath != "" && pageName != "" {
		if !isPathAllowed(cbrPath) {
			logger.Error("Unauthorized CBR access attempt: %s", cbrPath)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		serveCBRPage(w, cbrPath, pageName)
		return
	}

	// Serve normal filesystem file
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}

	// Security: ensure file is inside library dirs
	if !isPathAllowed(path) {
		logger.Error("Unauthorized path access attempt: %s", path)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, path)
}

// serveCBZPage serves a single page from CBZ archive
func serveCBZPage(w http.ResponseWriter, cbzPath, pageName string) {
	rzip, err := zip.OpenReader(cbzPath)
	if err != nil {
		logger.Error("Cannot open CBZ: %v", err)
		http.Error(w, "cannot open cbz", http.StatusInternalServerError)
		return
	}
	defer rzip.Close()

	for _, f := range rzip.File {
		if f.Name == pageName {
			rc, err := f.Open()
			if err != nil {
				logger.Error("Cannot read page: %v", err)
				http.Error(w, "cannot read page", http.StatusInternalServerError)
				return
			}
			defer rc.Close()

			setImageContentType(w, f.Name)
			io.Copy(w, rc)
			return
		}
	}

	http.Error(w, "page not found", http.StatusNotFound)
}

// serveCBRPage serves a single page from CBR archive
func serveCBRPage(w http.ResponseWriter, cbrPath, pageName string) {
	f, err := os.Open(cbrPath)
	if err != nil {
		logger.Error("Cannot open CBR: %v", err)
		http.Error(w, "cannot open cbr", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	rr, err := rardecode.NewReader(f, "")
	if err != nil {
		logger.Error("Cannot read CBR: %v", err)
		http.Error(w, "cannot read cbr", http.StatusInternalServerError)
		return
	}

	for {
		h, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Error("Error reading CBR: %v", err)
			http.Error(w, "error reading cbr", http.StatusInternalServerError)
			return
		}
		if h.Name == pageName {
			img, _, err := image.Decode(rr)
			if err != nil {
				logger.Error("Cannot decode image: %v", err)
				http.Error(w, "cannot decode image", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "public, max-age=86400")
			jpeg.Encode(w, img, &jpeg.Options{Quality: 90})
			return
		}
	}

	http.Error(w, "page not found", http.StatusNotFound)
}

// setImageContentType sets appropriate content type for images
func setImageContentType(w http.ResponseWriter, filename string) {
	ext := strings.ToLower(filepath.Ext(filename))
	contentType := "image/jpeg"
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".webp":
		contentType = "image/webp"
	case ".gif":
		contentType = "image/gif"
	case ".avif":
		contentType = "image/avif"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
}

// handleLibrary returns all library items
func handleLibrary(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, category, title, path, cover, coverData, lastModified FROM library ORDER BY title")
	if err != nil {
		logger.Error("Query failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var items []LibraryItem

	for rows.Next() {
		var item LibraryItem
		err := rows.Scan(&item.ID, &item.Category, &item.Title, &item.Path, &item.Cover, &item.CoverData, &item.LastMod)
		if err != nil {
			logger.Error("Scan error: %v", err)
			continue
		}

		// If coverData is missing, try to generate on demand
		if item.CoverData == "" && item.Cover != "" && item.Cover != "(cbz internal)" && item.Cover != "(cbr internal)" {
			coverPath := filepath.Join(item.Path, item.Cover)
			if data, err := generateThumbnailBase64(coverPath); err == nil {
				item.CoverData = data
				// Update database asynchronously
				go db.Exec("UPDATE library SET coverData=? WHERE id=?", data, item.ID)
			} else {
				logger.Debug("Failed to generate cover for %s: %v", coverPath, err)
			}
		}

		items = append(items, item)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(items)
}

// handlePages returns pages for a specific item
func handlePages(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var path string
	err := db.QueryRow("SELECT path FROM library WHERE id=?", id).Scan(&path)
	if err != nil {
		logger.Error("Failed to find library item: %v", err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	lower := strings.ToLower(path)

	if strings.HasSuffix(lower, ".cbz") {
		handleCBZPages(w, path)
		return
	}

	if strings.HasSuffix(lower, ".cbr") {
		handleCBRPages(w, path)
		return
	}

	handleDirectoryPages(w, path)
}

// handleCBZPages returns page URLs for CBZ file
func handleCBZPages(w http.ResponseWriter, path string) {
	pages, err := getImagesFromCBZ(path)
	if err != nil {
		logger.Error("Cannot read CBZ: %v", err)
		http.Error(w, "cannot read cbz", http.StatusInternalServerError)
		return
	}

	var urls []string
	for _, p := range pages {
		urls = append(urls, fmt.Sprintf("/media?cbz=%s&page=%s",
			url.QueryEscape(path), url.QueryEscape(p)))
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(urls)
}

// handleCBRPages returns page URLs for CBR file
func handleCBRPages(w http.ResponseWriter, path string) {
	pages, err := getImagesFromCBR(path)
	if err != nil {
		logger.Error("Cannot read CBR: %v", err)
		http.Error(w, "cannot read cbr", http.StatusInternalServerError)
		return
	}

	var urls []string
	for _, p := range pages {
		urls = append(urls, fmt.Sprintf("/media?cbr=%s&page=%s",
			url.QueryEscape(path), url.QueryEscape(p)))
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(urls)
}

// handleDirectoryPages returns page URLs for directory
func handleDirectoryPages(w http.ResponseWriter, path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		logger.Error("Cannot read directory: %v", err)
		http.Error(w, "cannot read directory", http.StatusInternalServerError)
		return
	}

	var pages []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if isImageFile(name) && !strings.HasPrefix(e.Name(), ".") {
			pages = append(pages, filepath.Join(path, e.Name()))
		}
	}

	sort.Slice(pages, func(i, j int) bool {
		return naturalLess(filepath.Base(pages[i]), filepath.Base(pages[j]))
	})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(pages)
}

// handleHealth provides health check endpoint
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": "stable",
		"uptime":  time.Since(startTime).String(),
	})
}

var startTime time.Time

func main() {
	startTime = time.Now()

	// Load configuration
	cfg, err := loadConfig("magz.config.json")
	if err != nil {
		fmt.Printf("❌ Configuration error: %v\n", err)
		fmt.Println("💡 Tip: Copy magz.config.example.json to magz.config.json and edit it")
		os.Exit(1)
	}
	config = *cfg

	// Initialize logger
	logger = &Logger{level: config.LogLevel}
	logger.Info("Starting Magz")

	// Initialize database
	db, err = initDatabase(config.CacheDB)
	if err != nil {
		logger.Error("Database error: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	// Initialize thumbnail generation semaphore
	thumbSemaphore = make(chan struct{}, 4)

	// Initial cache build
	buildCache()

	// Start background cache refresh
	go func() {
		ticker := time.NewTicker(time.Duration(config.AutoRefreshInterval) * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			buildCache()
		}
	}()

	// Setup HTTP routes
	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("frontend"))
	mux.Handle("/", fs)

	// API endpoints
	mux.HandleFunc("/api/library", handleLibrary)
	mux.HandleFunc("/api/pages", handlePages)
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/media", handleMedia)

	// Create server with timeouts
	addr := fmt.Sprintf(":%d", config.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("🚀 Magz running at http://localhost%s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	<-shutdown
	logger.Info("Shutting down gracefully...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Shutdown error: %v", err)
	}

	logger.Info("Server stopped")
}
