package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"govid/internal/ffmpeg"
	"govid/internal/models"
	"govid/pkg/config"
	"govid/pkg/logger"
)

// MCPServer wraps MCP server with dependencies
type MCPServer struct {
	server   *server.MCPServer
	executor *ffmpeg.Executor
	jobStore *models.JobStore
	cfg      *config.Config
	jobWG    *sync.WaitGroup
}

// NewMCPServer creates a new MCP server with video processing tools
func NewMCPServer(executor *ffmpeg.Executor, jobStore *models.JobStore, cfg *config.Config, jobWG *sync.WaitGroup) *MCPServer {
	mcpServer := server.NewMCPServer(
		"govid-mcp-server",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	ms := &MCPServer{
		server:   mcpServer,
		executor: executor,
		jobStore: jobStore,
		cfg:      cfg,
		jobWG:    jobWG,
	}

	// Register tools
	ms.registerTools()

	return ms
}

// GetServer returns the underlying MCP server
func (ms *MCPServer) GetServer() *server.MCPServer {
	return ms.server
}

// registerTools registers all video processing tools
func (ms *MCPServer) registerTools() {
	// Merge videos tool
	mergeVideosTool := mcp.NewTool("merge_videos",
		mcp.WithDescription("Merge multiple video segments with customizable timeframes per segment"),
		mcp.WithString("segments_json",
			mcp.Required(),
			mcp.Description("JSON array of video segments with file_path, start_time, and end_time"),
		),
	)
	ms.server.AddTool(mergeVideosTool, ms.handleMergeVideos)

	// Add image overlay tool
	overlayTool := mcp.NewTool("add_image_overlay",
		mcp.WithDescription("Add image overlay to video with position, duration, and animations (fade, slide, zoom)"),
		mcp.WithString("video_path",
			mcp.Required(),
			mcp.Description("Path to the input video file"),
		),
		mcp.WithString("overlay_json",
			mcp.Required(),
			mcp.Description("JSON object with overlay configuration including file_path, position, start_time, end_time, and animation settings"),
		),
	)
	ms.server.AddTool(overlayTool, ms.handleAddImageOverlay)

	// Add background music tool
	audioTool := mcp.NewTool("add_background_music",
		mcp.WithDescription("Add background music with volume control, fade effects, and timeframe selection"),
		mcp.WithString("video_path",
			mcp.Required(),
			mcp.Description("Path to the input video file"),
		),
		mcp.WithString("audio_json",
			mcp.Required(),
			mcp.Description("JSON object with audio configuration including file_path, volume (0.0-1.0), start_time, end_time, fade_in, and fade_out"),
		),
	)
	ms.server.AddTool(audioTool, ms.handleAddBackgroundMusic)

	// Complete process tool
	completeTool := mcp.NewTool("process_video_complete",
		mcp.WithDescription("Complete video processing with merge, overlay, and audio in one operation"),
		mcp.WithString("request_json",
			mcp.Required(),
			mcp.Description("JSON object with segments array, optional overlays array, and optional audio object"),
		),
	)
	ms.server.AddTool(completeTool, ms.handleProcessComplete)

	// Get job status tool
	jobStatusTool := mcp.NewTool("get_job_status",
		mcp.WithDescription("Get the status of a video processing job"),
		mcp.WithString("job_id",
			mcp.Required(),
			mcp.Description("The job ID to check"),
		),
	)
	ms.server.AddTool(jobStatusTool, ms.handleGetJobStatus)

	// Upload file tool
	uploadFileTool := mcp.NewTool("upload_file",
		mcp.WithDescription("Upload a single file (video, image, or audio) using base64 encoding"),
		mcp.WithString("filename",
			mcp.Required(),
			mcp.Description("Original filename with extension (e.g., video.mp4, logo.png, music.mp3)"),
		),
		mcp.WithString("content_base64",
			mcp.Required(),
			mcp.Description("Base64-encoded file content"),
		),
	)
	ms.server.AddTool(uploadFileTool, ms.handleUploadFile)

	// Upload multiple files tool
	uploadMultipleFilesTool := mcp.NewTool("upload_multiple_files",
		mcp.WithDescription("Upload multiple files at once using base64 encoding"),
		mcp.WithString("files_json",
			mcp.Required(),
			mcp.Description("JSON array of objects with 'filename' and 'content_base64' fields"),
		),
	)
	ms.server.AddTool(uploadMultipleFilesTool, ms.handleUploadMultipleFiles)
}

// createJobResponse creates a standard job response
func (ms *MCPServer) createJobResponse() (*models.Job, string) {
	jobID := uuid.New().String()
	job := models.NewJob(jobID)
	ms.jobStore.Add(job)

	response := map[string]any{
		"job_id":  jobID,
		"status":  "pending",
		"message": "Job created successfully",
	}

	responseJSON, _ := sonic.MarshalString(response)
	return job, responseJSON
}

// handleVideoProcessingTool handles common video processing tool logic
func (ms *MCPServer) handleVideoProcessingTool(_ context.Context, request mcp.CallToolRequest, jsonKey string, unmarshalFn func(string) (any, error), processFn func(*models.Job, string, any)) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	videoPath, ok := args["video_path"].(string)
	if !ok {
		return mcp.NewToolResultError("video_path must be a string"), nil
	}

	jsonStr, ok := args[jsonKey].(string)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("%s must be a string", jsonKey)), nil
	}

	config, err := unmarshalFn(jsonStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse %s: %v", jsonKey, err)), nil
	}

	job, responseJSON := ms.createJobResponse()
	ms.jobWG.Add(1)
	go func() {
		defer ms.jobWG.Done()
		processFn(job, videoPath, config)
	}()

	return mcp.NewToolResultText(responseJSON), nil
}

// handleMergeVideos handles video merging requests
func (ms *MCPServer) handleMergeVideos(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	segmentsJSON, ok := args["segments_json"].(string)
	if !ok {
		return mcp.NewToolResultError("segments_json must be a string"), nil
	}

	var segments []models.VideoSegment
	if err := sonic.UnmarshalString(segmentsJSON, &segments); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse segments_json: %v", err)), nil
	}

	if len(segments) < 2 {
		return mcp.NewToolResultError("At least 2 video segments required"), nil
	}

	job, responseJSON := ms.createJobResponse()
	ms.jobWG.Add(1)
	go func() {
		defer ms.jobWG.Done()
		ms.processMergeJob(job, segments)
	}()

	return mcp.NewToolResultText(responseJSON), nil
}

// handleAddImageOverlay handles image overlay requests
func (ms *MCPServer) handleAddImageOverlay(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return ms.handleVideoProcessingTool(ctx, request, "overlay_json",
		func(jsonStr string) (any, error) {
			var overlay models.ImageOverlay
			err := sonic.UnmarshalString(jsonStr, &overlay)
			return overlay, err
		},
		func(job *models.Job, videoPath string, config any) {
			ms.processOverlayJob(job, videoPath, config.(models.ImageOverlay))
		})
}

// handleAddBackgroundMusic handles background music requests
func (ms *MCPServer) handleAddBackgroundMusic(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return ms.handleVideoProcessingTool(ctx, request, "audio_json",
		func(jsonStr string) (any, error) {
			var audio models.AudioConfig
			err := sonic.UnmarshalString(jsonStr, &audio)
			return audio, err
		},
		func(job *models.Job, videoPath string, config any) {
			ms.processAudioJob(job, videoPath, config.(models.AudioConfig))
		})
}

// handleProcessComplete handles complete processing requests
func (ms *MCPServer) handleProcessComplete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	requestJSON, ok := args["request_json"].(string)
	if !ok {
		return mcp.NewToolResultError("request_json must be a string"), nil
	}

	var req models.CompleteProcessRequest
	if err := sonic.UnmarshalString(requestJSON, &req); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse request_json: %v", err)), nil
	}

	if len(req.Segments) < 1 {
		return mcp.NewToolResultError("At least 1 video segment required"), nil
	}

	job, responseJSON := ms.createJobResponse()
	ms.jobWG.Add(1)
	go func() {
		defer ms.jobWG.Done()
		ms.processCompleteJob(job, req)
	}()

	return mcp.NewToolResultText(responseJSON), nil
}

// handleGetJobStatus handles job status requests
func (ms *MCPServer) handleGetJobStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	jobID, ok := args["job_id"].(string)
	if !ok {
		return mcp.NewToolResultError("job_id must be a string"), nil
	}

	job, exists := ms.jobStore.Get(jobID)
	if !exists {
		return mcp.NewToolResultError(fmt.Sprintf("Job with ID %s does not exist", jobID)), nil
	}

	status := job.GetStatus()
	responseJSON, _ := sonic.MarshalString(status)
	return mcp.NewToolResultText(responseJSON), nil
}

// Job processing methods (similar to API handlers)

// processJobCommon handles common job processing logic for MCP
func (ms *MCPServer) processJobCommon(job *models.Job, jobType string, processFn func(context.Context, string) error) {
	job.UpdateStatus(models.JobStatusProcessing)
	job.UpdateProgress(10)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ms.cfg.JobTimeout)*time.Second)
	defer cancel()

	outputPath := filepath.Join(ms.cfg.OutputDir, fmt.Sprintf("%s.mp4", job.ID))

	logger.Info("Starting %s job %s (MCP)", jobType, job.ID)
	job.UpdateProgress(30)

	if err := processFn(ctx, outputPath); err != nil {
		logger.Error("%s job %s failed: %v", jobType, job.ID, err)
		job.SetError(err.Error())
		return
	}

	job.UpdateProgress(100)
	job.SetOutput(outputPath)
	job.UpdateStatus(models.JobStatusCompleted)
	logger.Info("%s job %s completed successfully (MCP)", jobType, job.ID)
}

func (ms *MCPServer) processMergeJob(job *models.Job, segments []models.VideoSegment) {
	ms.processJobCommon(job, "merge", func(ctx context.Context, outputPath string) error {
		return ms.executor.MergeVideos(ctx, segments, outputPath)
	})
}

func (ms *MCPServer) processOverlayJob(job *models.Job, videoPath string, overlay models.ImageOverlay) {
	ms.processJobCommon(job, "overlay", func(ctx context.Context, outputPath string) error {
		return ms.executor.AddImageOverlay(ctx, videoPath, overlay, outputPath)
	})
}

func (ms *MCPServer) processAudioJob(job *models.Job, videoPath string, audio models.AudioConfig) {
	ms.processJobCommon(job, "audio", func(ctx context.Context, outputPath string) error {
		return ms.executor.AddBackgroundMusic(ctx, videoPath, audio, outputPath)
	})
}

func (ms *MCPServer) processCompleteJob(job *models.Job, req models.CompleteProcessRequest) {
	ms.processJobCommon(job, "complete process", func(ctx context.Context, outputPath string) error {
		return ms.executor.CompleteProcess(ctx, req, outputPath)
	})
}

// handleUploadFile handles single file upload
func (ms *MCPServer) handleUploadFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	filename, ok := args["filename"].(string)
	if !ok {
		return mcp.NewToolResultError("filename must be a string"), nil
	}

	contentBase64, ok := args["content_base64"].(string)
	if !ok {
		return mcp.NewToolResultError("content_base64 must be a string"), nil
	}

	// Decode base64 content
	content, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to decode base64: %v", err)), nil
	}

	// Generate unique filename
	ext := filepath.Ext(filename)
	uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	savePath := filepath.Join(ms.cfg.UploadDir, uniqueFilename)

	// Save file
	if err := os.WriteFile(savePath, content, 0644); err != nil {
		logger.Error("Failed to save uploaded file: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("Failed to save file: %v", err)), nil
	}

	logger.Info("File uploaded successfully via MCP: %s (%d bytes)", uniqueFilename, len(content))

	response := map[string]any{
		"file_name": uniqueFilename,
		"file_path": savePath,
		"file_size": len(content),
		"message":   "File uploaded successfully",
	}

	responseJSON, _ := sonic.MarshalString(response)
	return mcp.NewToolResultText(responseJSON), nil
}

// handleUploadMultipleFiles handles multiple file uploads
func (ms *MCPServer) handleUploadMultipleFiles(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]any)
	if !ok {
		return mcp.NewToolResultError("invalid arguments format"), nil
	}

	filesJSON, ok := args["files_json"].(string)
	if !ok {
		return mcp.NewToolResultError("files_json must be a string"), nil
	}

	// Parse files JSON
	type FileUpload struct {
		Filename      string `json:"filename"`
		ContentBase64 string `json:"content_base64"`
	}
	var files []FileUpload
	if err := sonic.UnmarshalString(filesJSON, &files); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse files_json: %v", err)), nil
	}

	if len(files) == 0 {
		return mcp.NewToolResultError("At least one file is required"), nil
	}

	uploadedFiles := make([]map[string]any, 0, len(files))

	for _, file := range files {
		// Decode base64 content
		content, err := base64.StdEncoding.DecodeString(file.ContentBase64)
		if err != nil {
			logger.Error("Failed to decode base64 for file %s: %v", file.Filename, err)
			continue
		}

		// Generate unique filename
		ext := filepath.Ext(file.Filename)
		uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
		savePath := filepath.Join(ms.cfg.UploadDir, uniqueFilename)

		// Save file
		if err := os.WriteFile(savePath, content, 0644); err != nil {
			logger.Error("Failed to save uploaded file %s: %v", file.Filename, err)
			continue
		}

		logger.Info("File uploaded successfully via MCP: %s (%d bytes)", uniqueFilename, len(content))

		uploadedFiles = append(uploadedFiles, map[string]any{
			"file_name": uniqueFilename,
			"file_path": savePath,
			"file_size": len(content),
		})
	}

	if len(uploadedFiles) == 0 {
		return mcp.NewToolResultError("All file uploads failed"), nil
	}

	response := map[string]any{
		"files":   uploadedFiles,
		"message": fmt.Sprintf("%d file(s) uploaded successfully", len(uploadedFiles)),
	}

	responseJSON, _ := sonic.MarshalString(response)
	return mcp.NewToolResultText(responseJSON), nil
}
