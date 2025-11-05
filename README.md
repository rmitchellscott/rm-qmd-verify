# reMarkable QMD Verifier

Web application and API for verifying QMD (QML Diff) files against multiple hashtables to determine device and OS compatibility.

**Features:**
- Drag-and-drop web interface for QMD file uploads
- Parallel verification against all available hashtables
- Compatibility matrix showing which OS/device combinations are supported
- REST API for programmatic access
- [Command-line tool](https://github.com/rmitchellscott/rm-qmd-verify-cli)

## Quick Start

### Docker Deployment

```bash
# Clone the repository (includes hashtables)
git clone https://github.com/rmitchellscott/rm-qmd-verify.git
cd rm-qmd-verify

# Run
docker-compose up -d
```

Access the application at http://localhost:8080

**Note:** Docker builds exclude hashtables by design. Mount the hashtables directory when running containers.

### CLI Commands

```bash
# Start the web server (default command)
./qmdverify
./qmdverify serve

# Sync hashtables from GitHub
./qmdverify sync
./qmdverify sync --dir ./custom-dir
./qmdverify sync --branch main --repo owner/repo

# Show version
./qmdverify --version

# Show help
./qmdverify --help
```

### Configuration

Set environment variables in `.env` or pass them directly:

```bash
PORT=8080                     # Server port
HASHTAB_DIR=./hashtables      # Hashtable directory path
```

## Development

### Backend

**Prerequisites:** Go 1.23+, Rust/Cargo (for building qmldiff)

```bash
# Install dependencies
go mod download

# Build qmldiff from source
git clone https://github.com/asivery/qmldiff
cd qmldiff
cargo build --release
mkdir -p /path/to/rm-qmd-verify/bin
cp target/release/qmldiff /path/to/rm-qmd-verify/bin/

# Run the server
go run main.go
```

### Frontend

**Prerequisites:** Node.js 24+

```bash
cd ui
npm install
npm run dev          # Development server
npm run build        # Production build
```

## API Reference

### POST /api/compare

Upload a QMD file for compatibility verification.

**Request:**
- Content-Type: `multipart/form-data`
- Field: `file` (QMD file)

**Response:**
```json
{
  "compatible": [
    {
      "hashtable": "3.22.0.64-rmpp",
      "os_version": "3.22.0.64",
      "device": "rmpp",
      "compatible": true
    }
  ],
  "incompatible": [
    {
      "hashtable": "3.20.0.52-rm2",
      "os_version": "3.20.0.52",
      "device": "rm2",
      "compatible": false,
      "error_detail": "3.20.0.52 (rm2): Cannot resolve hash 1121852971369147487"
    }
  ],
  "total_checked": 2
}
```

### GET /api/hashtables

List all loaded hashtables.

**Response:**
```json
{
  "hashtables": [
    {
      "name": "3.22.0.64-rmpp",
      "os_version": "3.22.0.64",
      "device": "rmpp",
      "entry_count": 1234
    }
  ],
  "count": 1
}
```

## Hashtables

Hashtables are device and OS-specific reference files used to verify QMD file compatibility. They are organized in device-specific directories:

```
hashtables/
├── rm1/
├── rm2/
├── rmpp/
└── rmppm/
```

### Getting Hashtables

Hashtables are stored in this repository and can be obtained in several ways:

**1. Clone the repository** (hashtables included):
```bash
git clone https://github.com/rmitchellscott/rm-qmd-verify.git
cd rm-qmd-verify
# Hashtables are already in hashtables/
```

**2. Sync via Docker/Docker Compose** (downloads latest from repository):
```bash
# Using docker-compose
docker-compose exec qmd-check /app/rm-qmd-verify sync --dir /app/hashtables

# Using docker directly
docker exec rm-qmd-verify /app/rm-qmd-verify sync --dir /app/hashtables

# Sync from different branch
docker-compose exec qmd-check /app/rm-qmd-verify sync --branch develop
```

**3. Manual download** from [GitHub](https://github.com/rmitchellscott/rm-qmd-verify/tree/main/hashtables)

### File Format

Name hashtable files using the format: `{os_version}-{device}`

**Supported devices:**
- `rm1` - reMarkable 1
- `rm2` - reMarkable 2
- `rmpp` - reMarkable Paper Pro
- `rmppm` - reMarkable Paper Pro Move

**Examples:**
- `3.22.0.64-rmpp` - OS 3.22.0.64 for Paper Pro
- `3.20.0.52-rm2` - OS 3.20.0.52 for reMarkable 2
- `3.15.2.1-rm1` - OS 3.15.2.1 for reMarkable 1

## Production Build

```bash
# Build image
docker build -t rm-qmd-verify:latest \
  --build-arg VERSION=1.0.0 \
  --build-arg COMMIT=$(git rev-parse HEAD) \
  --build-arg BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  .

# Run container
docker run -d -p 8080:8080 \
  -v $(pwd)/hashtables:/app/hashtables:ro \
  rm-qmd-verify:latest
```

## Architecture

- **Backend:** Go with Chi router
- **Frontend:** React + TypeScript + Vite + shadcn/ui
- **QMLDiff:** Integration via [qmldiff](https://github.com/asivery/qmldiff) binary

## License

MIT
