# reMarkable QMD Verifier

Web application and API for verifying QMD (QML Diff) files against multiple hashtables and QML trees to determine device and OS compatibility.

**Features:**
- Drag-and-drop web interface for QMD file uploads
- Tree validation - Apply QMD diffs to full QML trees for accurate validation
- Parallel verification against all available hashtables
- Compatibility matrix showing which OS/device combinations are supported
- REST API for programmatic access
- Real-time progress updates via WebSocket
- Background job queue for async processing
- Batch file upload and validation
- Automatic dependency tracking for QMD files
- Hot reload for hashtables and QML trees

## Quick Start

### Docker Deployment

```bash
# Clone the repository
git clone https://github.com/rmitchellscott/rm-qmd-verify.git
cd rm-qmd-verify

# Run
docker-compose up -d
```

Access the application at http://localhost:8080

### Running Locally

```bash
# Build and run
go build
./rm-qmd-verify
```

The server starts on port 8080 by default. Access the web interface at http://localhost:8080

### Configuration

Set environment variables in `.env` or pass them directly:

```bash
PORT=8080                              # Server port (default: 8080)
HASHTAB_DIR=./hashtables               # Hashtable directory path (default: ./hashtables)
QML_TREE_DIR=./qml-trees               # QML tree directory path (default: ./qml-trees)
QMLDIFF_BINARY=./qmldiff               # Path to qmldiff CLI binary (default: ./qmldiff)
MAX_CONCURRENT_VALIDATIONS=15          # Max parallel validations (default: 15)
```

## Development

### Backend

**Prerequisites:**
- Go 1.23+
- Rust/Cargo (for building qmldiff CLI binary)

```bash
# Install dependencies
go mod download

# Build qmldiff CLI binary
git clone --branch collect-hash-errors https://github.com/rmitchellscott/qmldiff
cd qmldiff
cargo build --release --bin qmldiff
cd ..

# Copy binary to project directory
cp qmldiff/target/release/qmldiff ./

# Build and run the server
go build
./rm-qmd-verify
```

### Frontend

**Prerequisites:** Node.js 20 LTS+

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

### GET /api/trees

List all available QML trees.

**Response:**
```json
{
  "trees": [
    {
      "name": "3.22.0.64-rmpp",
      "os_version": "3.22.0.64",
      "device": "rmpp",
      "path": "/app/qml-trees/3.22.0.64-rmpp"
    }
  ],
  "count": 1
}
```

### GET /api/validated-versions

List all OS versions that have available QML trees for validation.

**Response:**
```json
{
  "versions": [
    {
      "os_version": "3.22.0.64",
      "devices": ["rmpp", "rmppm"]
    }
  ]
}
```

### GET /api/results/{jobId}

Retrieve results for a validation job.

**Response:**
```json
{
  "jobId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "files_processed": 15,
  "files_modified": 3,
  "files_with_errors": 0,
  "has_hash_errors": false,
  "errors": [],
  "success": true
}
```

### GET /api/status/ws/{jobId}

WebSocket endpoint for real-time job status updates. Connect to receive live progress updates during validation.

**Connection:** `ws://localhost:8080/api/status/ws/{jobId}`

**Messages:**
```json
{
  "status": "processing",
  "progress": 50,
  "message": "Processing file 5 of 10"
}
```

### GET /api/version

Get application version information.

**Response:**
```json
{
  "version": "1.0.0",
  "commit": "abc123def",
  "buildTime": "2025-01-15T10:30:00Z"
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

## QML Trees

QML trees are device and OS-specific files representing the QML structure. 
Place QML trees in `QML_TREE_DIR` with names matching hashtable names. For example:
- `qml-trees/3.22.0.65-rmppm/` - Tree for OS 3.22.0.65, device rmppm
- `qml-trees/3.20.0.52-rm2/` - Tree for OS 3.20.0.52, device rm2

When validating, the system automatically matches hashtables to trees by name.

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
- **QMLDiff:** Integration via [qmldiff](https://github.com/rmitchellscott/qmldiff) CLI binary (Rust)
- **Validation:** Worker pool parallelization for efficient tree validation
- **Real-time Updates:** WebSocket connections for live job progress
- **Job Processing:** Background job queue with concurrent validation support

## License

MIT
