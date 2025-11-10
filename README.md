# GoVid - FFmpeg Video Processing API & MCP Server

A comprehensive video processing service that exposes FFmpeg capabilities through both HTTP API and Model Context Protocol (MCP) server.

## Features

### Video Processing Capabilities
- **Video Merging**: Merge multiple video segments with customizable timeframes per segment
- **Image Overlay**: Add image overlays with position, duration, and animations:
  - Fade in/out effects
  - Slide animations (from left, right, top, bottom)
  - Zoom effects
- **Background Music**: Add background music with:
  - Volume control (0.0-1.0)
  - Fade in/out effects
  - Timeframe selection (trim audio)

### Technical Features
- **Dual Interface**: Both HTTP REST API and MCP Server
- **Authentication**: Bearer token authentication for both interfaces
- **Async Processing**: Job-based processing with status tracking
- **Docker Support**: Containerized deployment with FFmpeg included
- **API Documentation**: OpenAPI/Swagger documentation with Scalar UI
- **High Performance**: Uses Sonic for fast JSON encoding/decoding

## Technology Stack

- **Language**: Go 1.25
- **Web Framework**: Fiber v3
- **MCP Library**: mcp-go (StreamableHTTP transport)
- **JSON Library**: Sonic (bytedance/sonic)
- **Video Processing**: FFmpeg
- **API Documentation**: Swag + Scalar

## Quick Start

### Prerequisites
- Docker & Docker Compose (recommended)
- OR Go 1.25+ and FFmpeg installed locally

### Using Docker Compose (Recommended)

1. Clone the repository
```bash
git clone <repository-url>
cd govid
```

2. Create environment file
```bash
cp .env.example .env
# Edit .env and set your API keys
```

3. Start the services
```bash
docker-compose up -d
```

4. Access the services
- HTTP API: http://localhost:4101
- API Documentation: http://localhost:4101/docs
- MCP Server: http://localhost:1106/mcp
- Health Check: http://localhost:4101/api/v1/health

### Local Development

1. Install dependencies
```bash
go mod download
```

2. Generate OpenAPI documentation
```bash
swag init -g cmd/main.go --outputTypes yaml --parseDependency --parseInternal
```

3. Set environment variables
```bash
export HTTP_API_KEY="your-http-api-key"
export MCP_API_KEY="your-mcp-api-key"
```

4. Run the application
```bash
go run cmd/main.go
```

## Configuration

Configuration is done via environment variables. See `.env.example` for all available options.

| Variable | Description | Default |
|----------|-------------|---------|
| `HTTP_PORT` | HTTP API server port | 4101 |
| `MCP_PORT` | MCP server port | 1106 |
| `HTTP_API_KEY` | API key for HTTP API | (required) |
| `MCP_API_KEY` | API key for MCP server | (required) |
| `FFMPEG_BINARY` | Path to FFmpeg binary | ffmpeg |
| `UPLOAD_DIR` | Directory for uploaded files | ./uploads |
| `OUTPUT_DIR` | Directory for output files | ./outputs |
| `TEMP_DIR` | Directory for temporary files | ./temp |
| `MAX_CONCURRENT_JOBS` | Max concurrent processing jobs | 3 |
| `JOB_TIMEOUT` | Job timeout in seconds | 3600 |

## HTTP API Usage

### Authentication

All API requests (except health check and documentation) require API key authentication via the `X-API-Key` header:

```bash
curl -H "X-API-Key: your-http-api-key" \
     http://localhost:4101/api/v1/video/merge
```

### File Upload

#### Upload Single File
```bash
POST /api/v1/upload
```

Upload a video, image, or audio file:
```bash
curl -X POST http://localhost:4101/api/v1/upload \
  -H "X-API-Key: your-api-key" \
  -F "file=@/path/to/video.mp4"
```

Response:
```json
{
  "file_name": "550e8400-e29b-41d4-a716-446655440000.mp4",
  "file_path": "/uploads/550e8400-e29b-41d4-a716-446655440000.mp4",
  "file_size": 1048576
}
```

#### Upload Multiple Files
```bash
POST /api/v1/upload/multiple
```

Upload multiple files at once:
```bash
curl -X POST http://localhost:4101/api/v1/upload/multiple \
  -H "X-API-Key: your-api-key" \
  -F "files=@/path/to/video1.mp4" \
  -F "files=@/path/to/video2.mp4"
```

### Video Processing Endpoints

All video processing endpoints support **two request formats**:
1. **JSON** - Reference previously uploaded files by path
2. **Multipart/form-data** - Upload and process files in one request (max 10 videos for merge)

#### Merge Videos
```bash
POST /api/v1/video/merge
```

**Option 1: JSON (with file paths)**
```bash
curl -X POST http://localhost:4101/api/v1/video/merge \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "segments": [
      {
        "file_path": "/uploads/video1.mp4",
        "start_time": 0,
        "end_time": 10.5
      },
      {
        "file_path": "/uploads/video2.mp4",
        "start_time": 5,
        "end_time": 15
      }
    ]
  }'
```

**Option 2: Multipart (direct upload, 2-10 files)**
```bash
curl -X POST http://localhost:4101/api/v1/video/merge \
  -H "X-API-Key: your-api-key" \
  -F "videos=@/path/to/video1.mp4" \
  -F "videos=@/path/to/video2.mp4" \
  -F "videos=@/path/to/video3.mp4"
```
*Note: Files uploaded via multipart are merged in full (no timeframe trimming)*

#### Add Image Overlay
```bash
POST /api/v1/video/overlay
```

**Option 1: JSON (with file paths)**
```bash
curl -X POST http://localhost:4101/api/v1/video/overlay \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "video_path": "/uploads/video.mp4",
    "overlay": {
      "file_path": "/uploads/logo.png",
      "position": "top-right",
      "start_time": 0,
      "end_time": 5,
      "animation": "fade",
      "fade_duration": 1.0
    }
  }'
```

**Option 2: Multipart (direct upload)**
```bash
curl -X POST http://localhost:4101/api/v1/video/overlay \
  -H "X-API-Key: your-api-key" \
  -F "video=@/path/to/video.mp4" \
  -F "image=@/path/to/logo.png"
```
*Note: Default overlay position is top-right*

Supported positions: `top-left`, `top-right`, `bottom-left`, `bottom-right`, `center`, `custom`
Supported animations: `fade`, `slide`, `zoom`, `none`

#### Add Background Music
```bash
POST /api/v1/video/audio
```

**Option 1: JSON (with file paths)**
```bash
curl -X POST http://localhost:4101/api/v1/video/audio \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "video_path": "/uploads/video.mp4",
    "audio": {
      "file_path": "/uploads/music.mp3",
      "volume": 0.3,
      "start_time": 0,
      "end_time": 30,
      "fade_in": 2,
      "fade_out": 2
    }
  }'
```

**Option 2: Multipart (direct upload)**
```bash
curl -X POST http://localhost:4101/api/v1/video/audio \
  -H "X-API-Key: your-api-key" \
  -F "video=@/path/to/video.mp4" \
  -F "audio=@/path/to/music.mp3"
```
*Note: Default volume is 0.3 (30%)*

#### Complete Video Processing
```bash
POST /api/v1/video/process
```

Request body:
```json
{
  "segments": [
    {
      "file_path": "/uploads/video1.mp4",
      "start_time": 0,
      "end_time": 10
    }
  ],
  "overlays": [
    {
      "file_path": "/uploads/logo.png",
      "position": "top-right",
      "start_time": 0,
      "end_time": 10,
      "animation": "fade",
      "fade_duration": 1.0
    }
  ],
  "audio": {
    "file_path": "/uploads/music.mp3",
    "volume": 0.3,
    "fade_in": 2,
    "fade_out": 2
  }
}
```

#### Get Job Status
```bash
GET /api/v1/jobs/{job_id}
```

Response:
```json
{
  "job_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "progress": 100,
  "output_path": "/outputs/550e8400-e29b-41d4-a716-446655440000.mp4",
  "error": "",
  "created_at": "2025-01-13T10:00:00Z",
  "updated_at": "2025-01-13T10:05:00Z"
}
```

Job statuses: `pending`, `processing`, `completed`, `failed`

## MCP Server Usage

### Authentication

MCP requests require Bearer token authentication via the Authorization header:

```
Authorization: Bearer your-mcp-api-key
```

### Available Tools

#### upload_file
Upload a single file (video, image, or audio) using base64 encoding.

Parameters:
- `filename` (string): Original filename with extension (e.g., video.mp4, logo.png, music.mp3)
- `content_base64` (string): Base64-encoded file content

Example:
```json
{
  "filename": "video.mp4",
  "content_base64": "SGVsbG8gV29ybGQh..."
}
```

Response:
```json
{
  "file_name": "550e8400-e29b-41d4-a716-446655440000.mp4",
  "file_path": "/uploads/550e8400-e29b-41d4-a716-446655440000.mp4",
  "file_size": 1048576,
  "message": "File uploaded successfully"
}
```

#### upload_multiple_files
Upload multiple files at once using base64 encoding.

Parameters:
- `files_json` (string): JSON array of objects with `filename` and `content_base64` fields

Example:
```json
{
  "files_json": "[{\"filename\":\"video1.mp4\",\"content_base64\":\"...\"},{\"filename\":\"video2.mp4\",\"content_base64\":\"...\"}]"
}
```

Response:
```json
{
  "files": [
    {
      "file_name": "550e8400-e29b-41d4-a716-446655440000.mp4",
      "file_path": "/uploads/550e8400-e29b-41d4-a716-446655440000.mp4",
      "file_size": 1048576
    }
  ],
  "message": "2 file(s) uploaded successfully"
}
```

#### merge_videos
Merge multiple video segments with customizable timeframes.

Parameters:
- `segments_json` (string): JSON array of video segments with file_path, start_time, and end_time

#### add_image_overlay
Add image overlay with animations.

Parameters:
- `video_path` (string): Path to input video
- `overlay_json` (string): JSON object with overlay configuration

#### add_background_music
Add background music with effects.

Parameters:
- `video_path` (string): Path to input video
- `audio_json` (string): JSON object with audio configuration

#### process_video_complete
Complete video processing in one operation.

Parameters:
- `request_json` (string): JSON object with complete processing request

#### get_job_status
Get status of a processing job.

Parameters:
- `job_id` (string): Job ID to check

### MCP Usage Workflow

**Option 1: Upload then Process**
```
1. Call upload_file or upload_multiple_files with base64-encoded content
2. Use returned file_path in processing tools (merge_videos, add_image_overlay, etc.)
3. Call get_job_status to check progress
```

**Option 2: Use Pre-uploaded Files**
```
1. Files already uploaded via HTTP API or manually placed in uploads directory
2. Reference file paths directly in MCP tool calls
3. Call get_job_status to check progress
```

**Example Flow:**
```python
# 1. Upload video files
upload_response = mcp_client.call_tool("upload_multiple_files", {
    "files_json": json.dumps([
        {"filename": "video1.mp4", "content_base64": base64_video1},
        {"filename": "video2.mp4", "content_base64": base64_video2}
    ])
})

# 2. Extract file paths from response
files = json.loads(upload_response)["files"]
file_paths = [f["file_path"] for f in files]

# 3. Merge videos
merge_response = mcp_client.call_tool("merge_videos", {
    "segments_json": json.dumps([
        {"file_path": file_paths[0], "start_time": 0, "end_time": 10},
        {"file_path": file_paths[1], "start_time": 0, "end_time": 10}
    ])
})

# 4. Get job ID and check status
job_id = json.loads(merge_response)["job_id"]
status = mcp_client.call_tool("get_job_status", {"job_id": job_id})
```

## API Documentation

Interactive API documentation is available at:
- Scalar UI: http://localhost:4101/docs
- OpenAPI Spec: http://localhost:4101/docs/swagger.yaml

## Deployment with Traefik

When deploying with Traefik reverse proxy, the application uses a single domain with path-based routing:
- Main domain (e.g., `govid.example.com`) routes to HTTP API (port 4101)
- Path `/mcp` (e.g., `govid.example.com/mcp`) routes to MCP Server (port 1106)

Configure the `DOMAIN` environment variable in your `.env` file for Traefik integration.

## Project Structure

```
govid/
├── cmd/
│   └── main.go              # Application entry point
├── internal/
│   ├── ffmpeg/              # FFmpeg operations
│   │   ├── executor.go      # Command executor
│   │   ├── video.go         # Video merging
│   │   ├── overlay.go       # Image overlays
│   │   └── audio.go         # Audio processing
│   ├── models/              # Data models
│   │   └── types.go         # Shared types
│   ├── api/                 # HTTP API
│   │   ├── handlers.go      # Request handlers
│   │   ├── middleware.go    # Middleware
│   │   └── routes.go        # Route definitions
│   └── mcp/                 # MCP server
│       ├── tools.go         # MCP tools
│       └── middleware.go    # MCP middleware
├── pkg/
│   ├── config/              # Configuration
│   ├── auth/                # Authentication
│   └── logger/              # Logging
├── docs/                    # Generated API docs
├── Dockerfile               # Container image
├── compose.yml              # Container orchestration
└── README.md                # This file
```

## Development

### Generate OpenAPI Documentation

```bash
swag init -g cmd/main.go --outputTypes yaml --parseDependency --parseInternal
```

### Build

```bash
go build -o govid ./cmd/main.go
```

### Run Tests

```bash
go test ./...
```

## Production Deployment

1. Generate strong API keys:
```bash
openssl rand -hex 32
```

2. Update environment variables in production

3. Use Docker Compose or your preferred orchestration tool

4. Set up reverse proxy (nginx, traefik, etc.) for HTTPS

5. Configure persistent volumes for uploads/outputs

6. Set up monitoring and logging

## Production Deployment with Traefik

The `compose.yml` includes Traefik labels for automatic HTTPS and path-based routing:

```yaml
# Environment variables needed:
DOMAIN=govid.example.com          # Your domain
CERT_RESOLVER=letsencrypt          # Traefik certificate resolver
HTTP_API_KEY=your-secure-key-here  # HTTP API authentication key
MCP_API_KEY=your-secure-key-here   # MCP authentication key
```

**Routing configuration:**
- `https://govid.example.com/` → HTTP API (port 4101)
- `https://govid.example.com/api/v1/*` → HTTP API endpoints
- `https://govid.example.com/docs` → API documentation
- `https://govid.example.com/mcp` → MCP Server (port 1106)

All HTTP requests are automatically redirected to HTTPS.

## Troubleshooting

### FFmpeg not found
Make sure FFmpeg is installed and accessible in PATH, or set `FFMPEG_BINARY` to the full path.

### Permission errors
Ensure the application has write permissions to `UPLOAD_DIR`, `OUTPUT_DIR`, and `TEMP_DIR`.

### Job timeouts
Increase `JOB_TIMEOUT` for large video files or complex processing.

