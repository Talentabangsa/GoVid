package api

import (
	"context"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"govid/internal/ffmpeg"
	"govid/internal/models"
	"govid/pkg/config"
	"govid/pkg/downloader"
	"govid/pkg/logger"
	"govid/pkg/storage"
	"govid/pkg/webhook"
)

// Handler contains dependencies for API handlers
type Handler struct {
	executor   *ffmpeg.Executor
	jobStore   *models.JobStore
	cfg        *config.Config
	s3Uploader *storage.S3Uploader
	downloader *downloader.VideoDownloader
	webhook    *webhook.Client
}

// NewHandler creates a new API handler
func NewHandler(executor *ffmpeg.Executor, jobStore *models.JobStore, cfg *config.Config) *Handler {
	// Initialize S3 uploader
	s3Uploader, err := storage.NewS3Uploader(storage.S3Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Bucket:    cfg.S3Bucket,
		Region:    cfg.S3Region,
		UseSSL:    cfg.S3UseSSL,
	})
	if err != nil {
		logger.Error("Failed to initialize S3 uploader: %v", err)
	}

	return &Handler{
		executor:   executor,
		jobStore:   jobStore,
		cfg:        cfg,
		s3Uploader: s3Uploader,
		downloader: downloader.NewVideoDownloader(cfg.TempDir),
		webhook:    webhook.NewClient(),
	}
}

// HealthCheck godoc
// @Summary Health check endpoint
// @Description Check if the service is running
// @Tags Health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Router /api/v1/health [get]
func (h *Handler) HealthCheck(c fiber.Ctx) error {
	return c.JSON(models.HealthResponse{
		Status:  "ok",
		Version: "1.0.0",
	})
}

// MergeVideos godoc
// @Summary Merge multiple videos with timeframes
// @Description Merge multiple video segments. Supports both JSON (with file paths) and multipart/form-data (direct upload, max 10 files)
// @Tags Video
// @Security ApiKeyAuth
// @Accept json,multipart/form-data
// @Produce json
// @Param request body models.MergeVideoRequest false "Video merge request (JSON)"
// @Param videos formData file false "Video files to upload (multipart, 2-10 files)"
// @Success 202 {object} models.JobResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/video/merge [post]
func (h *Handler) MergeVideos(c fiber.Ctx) error {
	contentType := string(c.Request().Header.ContentType())

	var req models.MergeVideoRequest

	// Handle multipart/form-data
	if len(contentType) >= len(fiber.MIMEMultipartForm) && contentType[:len(fiber.MIMEMultipartForm)] == fiber.MIMEMultipartForm {
		form, err := c.MultipartForm()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid multipart form",
				Message: err.Error(),
			})
		}

		files := form.File["videos"]
		if len(files) < 2 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid request",
				Message: "At least 2 video files required",
			})
		}

		if len(files) > 10 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Too many files",
				Message: "Maximum 10 videos allowed per merge",
			})
		}

		// Save uploaded files and build segments
		segments := make([]models.VideoSegment, 0, len(files))
		for _, file := range files {
			ext := filepath.Ext(file.Filename)
			filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
			savePath := filepath.Join(h.cfg.UploadDir, filename)

			if err := c.SaveFile(file, savePath); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
					Error:   "Failed to save uploaded file",
					Message: err.Error(),
				})
			}

			segments = append(segments, models.VideoSegment{
				FilePath:  savePath,
				StartTime: 0,
				EndTime:   0, // 0 means use full video
			})
		}

		req.Segments = segments
	} else {
		// Handle JSON
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid request body",
				Message: err.Error(),
			})
		}
	}

	// Validate request
	if len(req.Segments) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid request",
			Message: "At least 2 video segments required",
		})
	}

	job, response := h.createAndStartJob()
	go h.processMergeJob(job, req)

	return c.Status(fiber.StatusAccepted).JSON(response)
}

// AddImageOverlay godoc
// @Summary Add image overlay to video
// @Description Add an image overlay. Supports both JSON (with file paths) and multipart/form-data (direct upload)
// @Tags Video
// @Security ApiKeyAuth
// @Accept json,multipart/form-data
// @Produce json
// @Param request body models.OverlayRequest false "Overlay request (JSON)"
// @Param video formData file false "Video file (multipart)"
// @Param image formData file false "Image file for overlay (multipart)"
// @Param overlay_config formData string false "JSON string of overlay configuration (multipart)"
// @Success 202 {object} models.JobResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/video/overlay [post]
func (h *Handler) AddImageOverlay(c fiber.Ctx) error {
	contentType := string(c.Request().Header.ContentType())

	var req models.OverlayRequest

	// Handle multipart/form-data
	if len(contentType) >= len(fiber.MIMEMultipartForm) && contentType[:len(fiber.MIMEMultipartForm)] == fiber.MIMEMultipartForm {
		form, err := c.MultipartForm()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid multipart form",
				Message: err.Error(),
			})
		}

		// Get video file
		videoFiles := form.File["video"]
		if len(videoFiles) != 1 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid request",
				Message: "Exactly one video file required",
			})
		}

		// Get image file
		imageFiles := form.File["image"]
		if len(imageFiles) != 1 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid request",
				Message: "Exactly one image file required",
			})
		}

		// Save video file
		videoFile := videoFiles[0]
		videoExt := filepath.Ext(videoFile.Filename)
		videoFilename := fmt.Sprintf("%s%s", uuid.New().String(), videoExt)
		videoPath := filepath.Join(h.cfg.UploadDir, videoFilename)
		if err := c.SaveFile(videoFile, videoPath); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "Failed to save video file",
				Message: err.Error(),
			})
		}

		// Save image file
		imageFile := imageFiles[0]
		imageExt := filepath.Ext(imageFile.Filename)
		imageFilename := fmt.Sprintf("%s%s", uuid.New().String(), imageExt)
		imagePath := filepath.Join(h.cfg.UploadDir, imageFilename)
		if err := c.SaveFile(imageFile, imagePath); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "Failed to save image file",
				Message: err.Error(),
			})
		}

		// Build request with default overlay settings
		req.VideoPath = videoPath
		req.Overlay = models.ImageOverlay{
			FilePath: imagePath,
			Position: models.PositionTopRight,
		}
	} else {
		// Handle JSON
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid request body",
				Message: err.Error(),
			})
		}
	}

	job, response := h.createAndStartJob()
	go h.processOverlayJob(job, req)

	return c.Status(fiber.StatusAccepted).JSON(response)
}

// AddBackgroundMusic godoc
// @Summary Add background music to video
// @Description Add background music. Supports both JSON (with file paths) and multipart/form-data (direct upload)
// @Tags Video
// @Security ApiKeyAuth
// @Accept json,multipart/form-data
// @Produce json
// @Param request body models.AudioRequest false "Audio request (JSON)"
// @Param video formData file false "Video file (multipart)"
// @Param audio formData file false "Audio file (multipart)"
// @Param audio_config formData string false "JSON string of audio configuration (multipart)"
// @Success 202 {object} models.JobResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/video/audio [post]
func (h *Handler) AddBackgroundMusic(c fiber.Ctx) error {
	contentType := string(c.Request().Header.ContentType())

	var req models.AudioRequest

	// Handle multipart/form-data
	if len(contentType) >= len(fiber.MIMEMultipartForm) && contentType[:len(fiber.MIMEMultipartForm)] == fiber.MIMEMultipartForm {
		form, err := c.MultipartForm()
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid multipart form",
				Message: err.Error(),
			})
		}

		// Get video file
		videoFiles := form.File["video"]
		if len(videoFiles) != 1 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid request",
				Message: "Exactly one video file required",
			})
		}

		// Get audio file
		audioFiles := form.File["audio"]
		if len(audioFiles) != 1 {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid request",
				Message: "Exactly one audio file required",
			})
		}

		// Save video file
		videoFile := videoFiles[0]
		videoExt := filepath.Ext(videoFile.Filename)
		videoFilename := fmt.Sprintf("%s%s", uuid.New().String(), videoExt)
		videoPath := filepath.Join(h.cfg.UploadDir, videoFilename)
		if err := c.SaveFile(videoFile, videoPath); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "Failed to save video file",
				Message: err.Error(),
			})
		}

		// Save audio file
		audioFile := audioFiles[0]
		audioExt := filepath.Ext(audioFile.Filename)
		audioFilename := fmt.Sprintf("%s%s", uuid.New().String(), audioExt)
		audioPath := filepath.Join(h.cfg.UploadDir, audioFilename)
		if err := c.SaveFile(audioFile, audioPath); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "Failed to save audio file",
				Message: err.Error(),
			})
		}

		// Build request with default audio settings
		req.VideoPath = videoPath
		req.Audio = models.AudioConfig{
			FilePath: audioPath,
			Volume:   0.3,
		}
	} else {
		// Handle JSON
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error:   "Invalid request body",
				Message: err.Error(),
			})
		}
	}

	job, response := h.createAndStartJob()
	go h.processAudioJob(job, req)

	return c.Status(fiber.StatusAccepted).JSON(response)
}

// ProcessComplete godoc
// @Summary Complete video processing
// @Description Process video with merge, overlay, and audio in one operation
// @Tags Video
// @Security ApiKeyAuth
// @Accept json
// @Produce json
// @Param request body models.CompleteProcessRequest true "Complete process request"
// @Success 202 {object} models.JobResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/video/process [post]
func (h *Handler) ProcessComplete(c fiber.Ctx) error {
	var req models.CompleteProcessRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid request body",
			Message: err.Error(),
		})
	}

	// Validate request
	if len(req.Segments) < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid request",
			Message: "At least 1 video segment required",
		})
	}

	job, response := h.createAndStartJob()
	go h.processCompleteJob(job, req)

	return c.Status(fiber.StatusAccepted).JSON(response)
}

// GetJobStatus godoc
// @Summary Get job status
// @Description Get the status of a video processing job
// @Tags Jobs
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} models.JobStatusResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/jobs/{id} [get]
func (h *Handler) GetJobStatus(c fiber.Ctx) error {
	jobID := c.Params("id")

	job, exists := h.jobStore.Get(jobID)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "Job not found",
			Message: fmt.Sprintf("Job with ID %s does not exist", jobID),
		})
	}

	return c.JSON(job.GetStatus())
}

// DownloadOutput godoc
// @Summary Download completed job output
// @Description Download the output file from a completed processing job
// @Tags Jobs
// @Produce octet-stream
// @Param id path string true "Job ID"
// @Success 200 {file} string
// @Failure 404 {object} models.ErrorResponse "Job not found"
// @Failure 202 {object} models.ErrorResponse "Job not yet completed"
// @Failure 500 {object} models.ErrorResponse "File not accessible"
// @Router /api/v1/jobs/{id}/download [get]
// @Security ApiKeyAuth
func (h *Handler) DownloadOutput(c fiber.Ctx) error {
	jobID := c.Params("id")

	job, exists := h.jobStore.Get(jobID)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error:   "Job not found",
			Message: fmt.Sprintf("Job with ID %s does not exist", jobID),
		})
	}

	status := job.GetStatus()

	// Check if job is completed
	if status.Status != models.JobStatusCompleted {
		return c.Status(fiber.StatusAccepted).JSON(models.ErrorResponse{
			Error:   "Job not completed",
			Message: fmt.Sprintf("Job is currently %s. Please wait for it to complete.", status.Status),
		})
	}

	// Check if output path is set
	if status.OutputPath == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "No output file",
			Message: "Job completed but no output file was generated",
		})
	}

	// Verify file exists
	if _, err := os.Stat(status.OutputPath); os.IsNotExist(err) {
		logger.Error("Output file not found for job %s: %s", jobID, status.OutputPath)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "File not found",
			Message: "The output file no longer exists on the server",
		})
	}

	// Get filename from path
	filename := filepath.Base(status.OutputPath)

	// Set download headers
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Set("Content-Type", "application/octet-stream")

	logger.Info("Downloading output for job %s: %s", jobID, status.OutputPath)

	// Send the file
	return c.SendFile(status.OutputPath)
}

// createAndStartJob is a helper to create a job and return response
func (h *Handler) createAndStartJob() (*models.Job, models.JobResponse) {
	jobID := uuid.New().String()
	job := models.NewJob(jobID)
	h.jobStore.Add(job)

	response := models.JobResponse{
		JobID:     jobID,
		Status:    models.JobStatusPending,
		Message:   "Job created successfully",
		CreatedAt: job.CreatedAt,
	}

	return job, response
}

// processJobCommon handles common job processing logic
func (h *Handler) processJobCommon(job *models.Job, jobType string, processFn func(context.Context, string) error) {
	job.UpdateStatus(models.JobStatusProcessing)
	job.UpdateProgress(10)
	_ = h.jobStore.Update(job)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(h.cfg.JobTimeout)*time.Second)
	defer cancel()

	outputPath := filepath.Join(h.cfg.OutputDir, fmt.Sprintf("%s.mp4", job.ID))

	logger.Info("Starting %s job %s", jobType, job.ID)
	job.UpdateProgress(30)
	_ = h.jobStore.Update(job)

	if err := processFn(ctx, outputPath); err != nil {
		logger.Error("%s job %s failed: %v", jobType, job.ID, err)
		job.SetError(err.Error())
		_ = h.jobStore.Update(job)
		return
	}

	job.UpdateProgress(100)
	job.SetOutput(outputPath)
	job.UpdateStatus(models.JobStatusCompleted)
	_ = h.jobStore.Update(job)
	logger.Info("%s job %s completed successfully", jobType, job.ID)
}

// processMergeJob processes a video merge job
func (h *Handler) processMergeJob(job *models.Job, req models.MergeVideoRequest) {
	h.processJobCommon(job, "merge", func(ctx context.Context, outputPath string) error {
		return h.executor.MergeVideos(ctx, req.Segments, outputPath)
	})
}

// processOverlayJob processes an image overlay job
func (h *Handler) processOverlayJob(job *models.Job, req models.OverlayRequest) {
	h.processJobCommon(job, "overlay", func(ctx context.Context, outputPath string) error {
		return h.executor.AddImageOverlay(ctx, req.VideoPath, req.Overlay, outputPath)
	})
}

// processAudioJob processes a background music job
func (h *Handler) processAudioJob(job *models.Job, req models.AudioRequest) {
	h.processJobCommon(job, "audio", func(ctx context.Context, outputPath string) error {
		return h.executor.AddBackgroundMusic(ctx, req.VideoPath, req.Audio, outputPath)
	})
}

// processCompleteJob processes a complete video processing job
func (h *Handler) processCompleteJob(job *models.Job, req models.CompleteProcessRequest) {
	h.processJobCommon(job, "complete process", func(ctx context.Context, outputPath string) error {
		return h.executor.CompleteProcess(ctx, req, outputPath)
	})
}

// UploadFile godoc
// @Summary Upload a single file
// @Description Upload a video, image, or audio file
// @Tags Upload
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "File to upload"
// @Success 200 {object} models.UploadResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/upload [post]
func (h *Handler) UploadFile(c fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid file upload",
			Message: "No file provided or invalid file",
		})
	}

	// Generate unique filename
	ext := filepath.Ext(file.Filename)
	filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	savePath := filepath.Join(h.cfg.UploadDir, filename)

	// Save file
	if err := c.SaveFile(file, savePath); err != nil {
		logger.Error("Failed to save uploaded file: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "Failed to save file",
			Message: err.Error(),
		})
	}

	logger.Info("File uploaded successfully: %s (%d bytes)", filename, file.Size)

	return c.JSON(models.UploadResponse{
		FileName: filename,
		FilePath: savePath,
		FileSize: file.Size,
	})
}

// UploadMultipleFiles godoc
// @Summary Upload multiple files
// @Description Upload multiple video, image, or audio files
// @Tags Upload
// @Security ApiKeyAuth
// @Accept multipart/form-data
// @Produce json
// @Param files formData file true "Files to upload (multiple)"
// @Success 200 {object} models.MultiUploadResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/upload/multiple [post]
func (h *Handler) UploadMultipleFiles(c fiber.Ctx) error {
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid multipart form",
			Message: err.Error(),
		})
	}

	files := form.File["files"]
	if len(files) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "No files provided",
			Message: "At least one file is required",
		})
	}

	uploadedFiles := make([]models.UploadResponse, 0, len(files))

	for _, file := range files {
		// Generate unique filename
		ext := filepath.Ext(file.Filename)
		filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
		savePath := filepath.Join(h.cfg.UploadDir, filename)

		// Save file
		if err := c.SaveFile(file, savePath); err != nil {
			logger.Error("Failed to save uploaded file %s: %v", file.Filename, err)
			continue
		}

		logger.Info("File uploaded successfully: %s (%d bytes)", filename, file.Size)

		uploadedFiles = append(uploadedFiles, models.UploadResponse{
			FileName: filename,
			FilePath: savePath,
			FileSize: file.Size,
		})
	}

	if len(uploadedFiles) == 0 {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "Failed to save files",
			Message: "All file uploads failed",
		})
	}

	return c.JSON(models.MultiUploadResponse{
		Files: uploadedFiles,
	})
}

// CombineVideos godoc
// @Summary Combine videos from URLs or file uploads and upload to S3
// @Description Accepts either JSON with video URLs or multipart/form-data with video files, combines them in order, and uploads to S3
// @Tags Video
// @Security ApiKeyAuth
// @Accept json,multipart/form-data
// @Produce json
// @Param request body models.CombineVideosRequest false "Video URLs to combine (JSON mode)"
// @Param videos formData []file false "Video files to combine (multipart mode)"
// @Param webhook_url formData string false "Webhook URL for job completion notification (multipart mode)"
// @Success 200 {object} models.JobResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /api/v1/video/combine [post]
func (h *Handler) CombineVideos(c fiber.Ctx) error {
	// Check if S3 uploader is available
	if h.s3Uploader == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "S3 uploader not configured",
			Message: "S3 configuration is missing or invalid",
		})
	}

	// Try to parse as multipart form first
	if form, err := c.MultipartForm(); err == nil {
		return h.handleCombineVideosMultipart(c, form)
	}

	// Otherwise, handle as JSON (URL mode)
	return h.handleCombineVideosJSON(c)
}

// handleCombineVideosJSON handles JSON request with video URLs
func (h *Handler) handleCombineVideosJSON(c fiber.Ctx) error {
	var req models.CombineVideosRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid request body",
			Message: err.Error(),
		})
	}

	// Validate minimum videos
	if len(req.Videos) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid request",
			Message: "At least 2 video URLs are required",
		})
	}

	// Create job
	job, response := h.createAndStartJob()

	// Set webhook URL if provided
	if req.WebhookURL != "" {
		job.WebhookURL = req.WebhookURL
		_ = h.jobStore.Update(job)
	}

	// Start async processing from URLs
	go h.processCombineJobFromURLs(job, req.Videos)

	logger.Info("Created combine videos job %s with %d URLs", job.ID, len(req.Videos))

	return c.JSON(response)
}

// handleCombineVideosMultipart handles multipart/form-data request with file uploads
func (h *Handler) handleCombineVideosMultipart(c fiber.Ctx, form *multipart.Form) error {
	// Get uploaded files
	files := form.File["videos"]
	if len(files) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid request",
			Message: "At least 2 video files are required",
		})
	}

	// Limit maximum files
	if len(files) > 10 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Too many files",
			Message: "Maximum 10 video files allowed",
		})
	}

	// Save uploaded files to temp directory in order
	uploadedPaths := make([]string, 0, len(files))
	for i, file := range files {
		filename := fmt.Sprintf("%s_%d_%s", uuid.New().String(), i, filepath.Base(file.Filename))
		savePath := filepath.Join(h.cfg.TempDir, filename)

		if err := c.SaveFile(file, savePath); err != nil {
			// Clean up already saved files
			for _, path := range uploadedPaths {
				os.Remove(path)
			}
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error:   "Failed to save file",
				Message: err.Error(),
			})
		}

		uploadedPaths = append(uploadedPaths, savePath)
		logger.Info("Saved uploaded file %d: %s", i, savePath)
	}

	// Get optional webhook URL from form
	webhookURL := ""
	if webhookValues, ok := form.Value["webhook_url"]; ok && len(webhookValues) > 0 {
		webhookURL = webhookValues[0]
	}

	// Create job
	job, response := h.createAndStartJob()

	// Set webhook URL if provided
	if webhookURL != "" {
		job.WebhookURL = webhookURL
		_ = h.jobStore.Update(job)
	}

	// Start async processing from uploaded files
	go h.processCombineJobFromFiles(job, uploadedPaths)

	logger.Info("Created combine videos job %s with %d uploaded files", job.ID, len(uploadedPaths))

	return c.JSON(response)
}

// processCombineJobFromURLs processes a video combine job from URLs
func (h *Handler) processCombineJobFromURLs(job *models.Job, videoURLs []string) {
	job.UpdateStatus(models.JobStatusProcessing)
	job.UpdateProgress(10)
	_ = h.jobStore.Update(job)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(h.cfg.JobTimeout)*time.Second)
	defer cancel()

	logger.Info("Starting combine videos job %s from URLs", job.ID)

	// Download videos in order
	logger.Info("Downloading %d videos for job %s", len(videoURLs), job.ID)
	job.UpdateProgress(20)
	_ = h.jobStore.Update(job)

	downloadedFiles, err := h.downloader.DownloadVideosInOrder(videoURLs)
	if err != nil {
		logger.Error("Failed to download videos for job %s: %v", job.ID, err)
		job.SetError(fmt.Sprintf("Failed to download videos: %v", err))
		_ = h.jobStore.Update(job)
		h.sendWebhookIfConfigured(job)
		return
	}
	defer h.downloader.CleanupFiles(downloadedFiles)

	logger.Info("Downloaded %d videos for job %s", len(downloadedFiles), job.ID)
	job.UpdateProgress(40)
	_ = h.jobStore.Update(job)

	// Continue with common processing
	h.processCombineJobCommon(job, ctx, downloadedFiles, true)
}

// processCombineJobFromFiles processes a video combine job from uploaded files
func (h *Handler) processCombineJobFromFiles(job *models.Job, uploadedFiles []string) {
	job.UpdateStatus(models.JobStatusProcessing)
	job.UpdateProgress(10)
	_ = h.jobStore.Update(job)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(h.cfg.JobTimeout)*time.Second)
	defer cancel()

	logger.Info("Starting combine videos job %s from uploaded files", job.ID)

	// Files are already uploaded, skip to merge
	job.UpdateProgress(40)
	_ = h.jobStore.Update(job)

	// Continue with common processing
	h.processCombineJobCommon(job, ctx, uploadedFiles, true)
}

// processCombineJobCommon handles the common video merge and S3 upload logic
func (h *Handler) processCombineJobCommon(job *models.Job, ctx context.Context, inputFiles []string, cleanupFiles bool) {
	// Cleanup files at the end if requested
	if cleanupFiles {
		defer h.downloader.CleanupFiles(inputFiles)
	}

	// Merge videos
	outputPath := filepath.Join(h.cfg.OutputDir, fmt.Sprintf("%s.mp4", job.ID))
	logger.Info("Merging %d videos for job %s", len(inputFiles), job.ID)
	job.UpdateProgress(60)
	_ = h.jobStore.Update(job)

	if err := h.executor.MergeVideosSimple(ctx, inputFiles, outputPath); err != nil {
		logger.Error("Failed to merge videos for job %s: %v", job.ID, err)
		job.SetError(fmt.Sprintf("Failed to merge videos: %v", err))
		_ = h.jobStore.Update(job)
		h.sendWebhookIfConfigured(job)
		return
	}

	logger.Info("Videos merged successfully for job %s", job.ID)
	job.UpdateProgress(80)
	job.SetOutput(outputPath)
	_ = h.jobStore.Update(job)

	// Upload to S3
	logger.Info("Uploading to S3 for job %s", job.ID)
	objectName := storage.GetObjectName(job.ID, outputPath)
	s3URL, err := h.s3Uploader.Upload(ctx, outputPath, objectName)
	if err != nil {
		logger.Error("Failed to upload to S3 for job %s: %v", job.ID, err)
		job.SetError(fmt.Sprintf("Failed to upload to S3: %v", err))
		_ = h.jobStore.Update(job)
		h.sendWebhookIfConfigured(job)
		return
	}

	logger.Info("Uploaded to S3 for job %s: %s", job.ID, s3URL)
	job.SetS3URL(s3URL)
	job.UpdateProgress(90)
	_ = h.jobStore.Update(job)

	// Delete local file after successful upload
	if err := os.Remove(outputPath); err != nil {
		logger.Error("Failed to delete local file for job %s: %v", job.ID, err)
		// Don't fail the job, just log the error
	} else {
		logger.Info("Deleted local file for job %s", job.ID)
		// Clear output path since file is deleted
		job.SetOutput("")
	}

	// Mark job as completed
	job.UpdateProgress(100)
	job.UpdateStatus(models.JobStatusCompleted)
	_ = h.jobStore.Update(job)
	logger.Info("Combine videos job %s completed successfully", job.ID)

	// Send webhook notification
	h.sendWebhookIfConfigured(job)
}

// sendWebhookIfConfigured sends a webhook notification if webhook URL is configured
func (h *Handler) sendWebhookIfConfigured(job *models.Job) {
	if job.WebhookURL == "" {
		return
	}

	status := job.GetStatus()
	payload := webhook.JobCompletionPayload{
		JobID:  job.ID,
		Status: string(status.Status),
		S3URL:  status.S3URL,
		Error:  status.Error,
	}

	h.webhook.SendJobCompleteAsync(job.WebhookURL, payload)
}
