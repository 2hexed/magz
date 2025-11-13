# Magz

**Not your usual local magazine/book/comic reader.**

![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-%20%20GNU%20GPLv3%20-green)
![Status](https://img.shields.io/badge/Status-Beta-orange)
![Nix](https://img.shields.io/badge/Nix-Enabled-blue?logo=nixos)

## 📖 Overview

**Magz** makes reading **local magazines, books, or comics** effortless — no internet required.
It runs a lightweight Go-based local server that scans your library directories, caches metadata using SQLite,
and serves a simple web app UI for browsing and reading right in your browser.

## 🚀 Features

- 📚 Auto-detects and catalogs your local magazine/book folders
- 🖼️ Displays pages directly in a browser-based reader
- 🔁 Auto-refreshes your library every few minutes
- ⚡ Fast and portable — just a single Go binary
- 💿 Uses SQLite for local caching
- 🧩 Nix shell for easy development and reproducibility

## How to organize

```bash
/home/n/Books/ < (Books is a new Directory in my case)
├── Death to Pachuco < (Magazine/Comic Category ie., Directory name)
│   └── Death to Pachuco 001.cbz < (Magazine/Comic Title ie., Exact filename - Death to Pachuco 001)
├── Misc < (Magazine/Comic Category)
│   └── My Own Comic < (Magazine/Comic Title ie., Exact filename)
│       ├── 001.png
│       ├── 002.png
│       └── *.png
├── Playboy < (Magazine/Comic Category ie., Directory name)
│   └── Best Ones So far < (Magazine/Comic Title ie., Exact filename)
│       ├── 183887_001.jpg
│       ├── 183887_002.jpg
│       ├── 183887_003.jpg
│       └──*.jpg
└── Batman < (Magazine/Comic Category ie., Directory name)
    └── Batman #1 (1940-2011).cbr < (Magazine/Comic Title ie., Exact filename)
```

## 🧰 Requirements

- Go **1.22+**
- Any system Go runs on (Linux, macOS, Windows, etc.)
- (Optional) **Nix** for reproducible development environments

## ⚙️ Installation

### Option 1: Standard Go Setup

```bash
git clone https://github.com/2hexed/magz.git
cd magz

# Build the binary
go build -o magz

# Create your config file
cp magz.config.example.json magz.config.json
```

Then, edit `magz.config.json` to match your setup.

### Option 2: Installation with Nix (Shell Environment)

If you use **Nix**, you can instantly enter a ready-to-go environment:

```bash
nix-shell
```

Inside the shell, you’ll see a welcome message and have access to these helper commands:

| Command              | Description                                         |
| -------------------- | --------------------------------------------------- |
| `first-time-running` | Initializes Go module, installs dependencies        |
| `buildbin`           | Builds the `magz` binary                            |
| `fmtfiles`           | Formats Go, Nix, JSON, HTML, JS, and Markdown files |
| `prjcleanup`         | ⚠️ Deletes temporary and build artifacts            |

---

### Option 3: Directly from Releases

> TODO

## 🔒 Security

Magz implements several security measures:

1. **Path Validation**: All file access is validated against configured library paths
1. **Query Parameter Sanitization**: URL parameters are properly escaped
1. **No Directory Listing**: Only explicitly cataloged content is accessible
1. **Read-Only Access**: The application only reads files, never writes or modifies them
1. **Connection Timeouts**: HTTP server has configured timeouts to prevent resource exhaustion

### Best Practices

1. Run Magz on localhost only (don't expose to the internet without proper authentication)
1. Use specific library paths rather than root directories
1. Keep your Go version updated for security patches
1. Regularly review your library paths configuration

## 🧾 Configuration

Magz uses a single config file named `magz.config.json` in the project root. (Example file - `magz.config.example.json` is included in the project for users)

**Configuration Parameters:**

| Key                   | Type    | Description                                            |
| --------------------- | ------- | ------------------------------------------------------ |
| `Port`                | integer | Port for the local server                              |
| `AutoRefreshInterval` | integer | Minutes between automatic rescans                      |
| `LibraryPaths`        | array   | List of library directories containing magazines/books |
| `CacheDB`             | string  | SQLite cache database file name                        |
| `MaxThumbnailSize`    | int     | Maximum dimension for thumbnails in pixels             |
| `LogLevel`            | string  | Logging verbosity - "info" or "debug"                  |

## 🖥️ Usage

### Environment Variables

After building, You can override config values with environment variables:

```bash
export MAGZ_PORT=8090
export MAGZ_LOG_LEVEL=debug
./magz
```

You’ll see something like:

```
🚀 Magz running at http://localhost:8082
```

Then open your browser and visit that address.

Magz will:

- Scan your library paths for readable folders
- Cache metadata (title, category, cover, etc.)
- Serve the `public/` web app for browsing and reading

🧦 The web app is static (no build needed) and lives in `public/`.
It provides a minimal, clean reader interface.

## 📝 Keyboard Shortcuts

### Library Page

- `Ctrl/Cmd + K` - Focus search box
- `Escape` - Clear search
- `Enter` - Open selected magazine

### Viewer Page

- `Arrow Left` / `Arrow Right` - Navigate pages
- `Escape` - Return to library
- Touch/swipe gestures on mobile devices

## 🐛 Troubleshooting

### Library Not Updating

If your library doesn't show new content:

1. Check the log output for scan errors
2. Verify file permissions on library directories
3. Manually trigger a rescan by restarting the server
4. Check that file extensions are supported (.jpg, .jpeg, .png, .webp, .avif, .gif)

### Performance Issues

If the application is slow:

1. Reduce `MaxThumbnailSize` in config (try 300 or 250)
2. Increase `AutoRefreshInterval` to scan less frequently
3. Check database size - consider deleting and rebuilding cache
4. Ensure library paths are on fast storage (SSD preferred)

### Thumbnails Not Showing

If covers aren't displaying:

1. Check that cover images are valid formats
2. Look for error messages in logs (run with LogLevel: "debug")
3. Try deleting the cache database to force regeneration
4. Verify image file permissions

## 📊 Performance Tips

- **SSD Storage**: Use SSD for both library and cache database
- **Thumbnail Size**: Smaller thumbnails = faster loading (but lower quality)
- **Library Organization**: Organize files in subdirectories by category
- **Archive Format**: CBZ (zip) is faster than CBR (rar) for extraction
- **Concurrent Access**: The app handles multiple users but performance may degrade with many simultaneous readers

## 🔧 Development

### Running Tests

```bash
go test -v ./...
```

### Building for Different Platforms

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o magz-linux

# macOS
GOOS=darwin GOARCH=amd64 go build -o magz-macos

# Windows
GOOS=windows GOARCH=amd64 go build -o magz.exe
```

### Hot Reload During Development

Use `air` for hot reloading:

```bash
go install github.com/cosmtrek/air@latest
air
```

## 🔌 API Reference

Magz exposes a small set of REST endpoints used by the web app,
which can also be accessed manually or via other tools.

### Health Check Endpoint

```bash
curl http://localhost:8082/api/health
```

**Example Response:**

```json
{
  "status": "ok",
  "version": "1.0.0",
  "uptime": "2h30m15s"
}
```

### `GET /api/library`

Returns all cached library entries.

**Example Response:**

```json
[
  {
    "id": 1,
    "category": "Comics",
    "title": "Spiderverse Vol 1",
    "path": "/home/n/Books/Comics/Spiderverse Vol 1",
    "cover": "COVER TYPE",
    "coverData": "data:image/jpeg;base64,/9j/2..",
    "lastModified": "2025-11-12T14:03:22Z"
  }
]
```

---

### `GET /api/pages?id=<id>`

Returns all image pages for a specific library item.

**Example:**

```
GET /api/pages?id=1
```

**Response:**

```json
[
  "/home/n/Books/Comics/Spiderverse Vol 1/page1.jpg",
  "/home/n/Books/Comics/Spiderverse Vol 1/page2.jpg"
]
```

---

### `GET /media?path=<absolute-file-path>`

Serves a specific image file directly from disk.

**Example:**

```
GET /media?path=/home/n/Books/Comics/Spiderverse Vol 1/page1.jpg
```

## 🧱 Built With

- [Go](https://go.dev/)
- [Nix](https://nixos.org/)

## 🤝 Contributing Guidelines

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Format your code using `fmtfiles` (in Nix shell)
4. Commit your changes with descriptive messages
5. Push to your branch
6. Open a Pull Request

Please include:

- Clear description of changes
- Any new dependencies
- Updated documentation if needed
- Tests for new features

## 🌍 Roadmap / Future Ideas

| Status | Feature                               |
| :----: | ------------------------------------- |
|   OK   | Interval-based library refresh        |
|  TODO  | In-browser reading progress tracking  |
|   OK   | UI themes and dark mode               |
|  TODO  | Optional metadata editing and tagging |
|   OK   | Comic archive support                 |
|  TODO  | Systemd unit and Docker file          |
