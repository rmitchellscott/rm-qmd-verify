# reMarkable QMD Verifier

Web application and API for verifying QMD (QML Diff) files against multiple hashtables to determine device and OS compatibility.

**Features:**
- Drag-and-drop web interface for QMD file uploads
- **Tree validation mode** - Apply QMD diffs to full QML trees for accurate validation
- Parallel verification against all available hashtables
- Compatibility matrix showing which OS/device combinations are supported
- REST API for programmatic access
- Command-line tool with tree validation support

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

# Validate QMD against full QML tree (recommended)
./qmdverify validate-tree \
  --qmd patch.qmd \
  --hashtab ./hashtables/3.22.0.65-rmppm \
  --tree ./qml-trees/3.22.0.65-rmppm \
  --workers 4

# JSON output for scripting
./qmdverify validate-tree --qmd patch.qmd --hashtab ... --tree ... --json

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
QML_TREE_DIR=./qml-trees      # QML tree directory path (for tree validation)
```

**Tree Validation:**
Place QML trees in `QML_TREE_DIR` with names matching hashtable names. For example:
- `qml-trees/3.22.0.65-rmppm/` - Tree for OS 3.22.0.65, device rmppm
- `qml-trees/3.20.0.52-rm2/` - Tree for OS 3.20.0.52, device rm2

When validating, the system automatically matches hashtables to trees by name. If no tree is found for a hashtable, it gracefully falls back to simplified validation for that specific hashtable.

## Development

### Backend

**Prerequisites:**
- Go 1.23+
- Rust/Cargo (for building qmldiff static library)
- GCC or Clang (for CGO)

```bash
# Install dependencies
go mod download

# Build qmldiff static library
git clone --branch collect-hash-errors https://github.com/rmitchellscott/qmldiff
cd qmldiff
cargo build --release
cd ..

# Copy static library to project
mkdir -p lib
cp qmldiff/target/release/libqmldiff.a lib/

# Build and run the server (with CGO enabled)
CGO_ENABLED=1 go build
./rm-qmd-verify serve
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

**See [API.md](API.md) for complete API documentation.**

### POST /api/validate/tree

**Recommended:** Validate a QMD file by applying diffs to a full QML tree.

**Request:**
- Content-Type: `multipart/form-data`
- Fields:
  - `file` (QMD file)
  - `hashtab_path` (path to hashtab file on server)
  - `tree_path` (path to QML tree directory on server)
  - `workers` (optional, number of workers, default: 4)

**Response:**
```json
{
  "jobId": "550e8400-e29b-41d4-a716-446655440000"
}
```

Poll `/api/results/{jobId}` for completion:
```json
{
  "files_processed": 1,
  "files_modified": 1,
  "files_with_errors": 0,
  "has_hash_errors": false,
  "errors": [],
  "success": true
}
```

### POST /api/compare

**Primary endpoint:** Validates a QMD file against all available hashtables.

**Default:** Tree validation - automatically applies diffs to full QML trees for accurate validation
**Legacy:** Hash-only mode available via `?mode=hash` query parameter

**Request:**
- Content-Type: `multipart/form-data`
- Field: `file` (QMD file)
- Query parameter: `mode` (optional) - `tree` (default) or `hash` (legacy)

**Response (tree mode):**
```json
{
  "compatible": [
    {
      "hashtable": "3.22.0.64-rmpp",
      "os_version": "3.22.0.64",
      "device": "rmpp",
      "compatible": true,
      "validation_mode": "tree",
      "files_processed": 15,
      "files_modified": 3,
      "files_with_errors": 0,
      "tree_validation_used": true
    }
  ],
  "incompatible": [],
  "total_checked": 1,
  "mode": "tree"
}
```

Trees are automatically matched to hashtables by name. If no tree is available for a hashtable, it gracefully falls back to simplified validation for that specific hashtable.

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

- **Backend:** Go with Chi router + CGO
- **Frontend:** React + TypeScript + Vite + shadcn/ui
- **QMLDiff:** Integration via [qmldiff](https://github.com/rmitchellscott/qmldiff) static library (Rust + C FFI)
- **Validation:** Worker pool parallelization for efficient tree validation

## License

MIT
