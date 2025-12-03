package models

import (
	"sync"
	"time"
)

// JobStatus represents the status of a job
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

// VideoSegment represents a video segment with timeframe
type VideoSegment struct {
	FilePath  string  `json:"file_path" example:"/uploads/video1.mp4"`
	StartTime float64 `json:"start_time" example:"0"`  // in seconds
	EndTime   float64 `json:"end_time" example:"10.5"` // in seconds, 0 means end of video
}

// OverlayPosition represents predefined positions
type OverlayPosition string

const (
	PositionTopLeft     OverlayPosition = "top-left"
	PositionTopRight    OverlayPosition = "top-right"
	PositionBottomLeft  OverlayPosition = "bottom-left"
	PositionBottomRight OverlayPosition = "bottom-right"
	PositionCenter      OverlayPosition = "center"
	PositionCustom      OverlayPosition = "custom"
)

// AnimationType represents animation types
type AnimationType string

const (
	AnimationFade  AnimationType = "fade"
	AnimationSlide AnimationType = "slide"
	AnimationZoom  AnimationType = "zoom"
	AnimationNone  AnimationType = "none"
)

// SlideDirection represents slide direction
type SlideDirection string

const (
	SlideFromLeft   SlideDirection = "left"
	SlideFromRight  SlideDirection = "right"
	SlideFromTop    SlideDirection = "top"
	SlideFromBottom SlideDirection = "bottom"
)

// ImageOverlay represents image overlay configuration
type ImageOverlay struct {
	FilePath  string          `json:"file_path" example:"/uploads/logo.png"`
	Position  OverlayPosition `json:"position" example:"top-left"`
	X         *int            `json:"x,omitempty" example:"10"` // custom x position (only if position is "custom")
	Y         *int            `json:"y,omitempty" example:"10"` // custom y position (only if position is "custom")
	StartTime float64         `json:"start_time" example:"0"`   // when overlay appears (seconds)
	EndTime   float64         `json:"end_time" example:"5"`     // when overlay disappears (seconds)
	Animation AnimationType   `json:"animation" example:"fade"`
	// Animation specific options
	FadeDuration   *float64        `json:"fade_duration,omitempty" example:"1.0"` // fade in/out duration
	SlideDirection *SlideDirection `json:"slide_direction,omitempty" example:"left"`
	SlideDuration  *float64        `json:"slide_duration,omitempty" example:"1.0"`
	ZoomFrom       *float64        `json:"zoom_from,omitempty" example:"0.5"` // initial zoom level
	ZoomTo         *float64        `json:"zoom_to,omitempty" example:"1.5"`   // final zoom level
}

// AudioConfig represents background music configuration
type AudioConfig struct {
	FilePath  string   `json:"file_path" example:"/uploads/music.mp3"`
	Volume    float64  `json:"volume" example:"0.3"`             // 0.0 to 1.0
	StartTime *float64 `json:"start_time,omitempty" example:"0"` // trim audio start (seconds)
	EndTime   *float64 `json:"end_time,omitempty" example:"30"`  // trim audio end (seconds)
	FadeIn    *float64 `json:"fade_in,omitempty" example:"2"`    // fade in duration
	FadeOut   *float64 `json:"fade_out,omitempty" example:"2"`   // fade out duration
}

// MergeVideoRequest represents video merge request
type MergeVideoRequest struct {
	Segments []VideoSegment `json:"segments" binding:"required,min=2"`
}

// OverlayRequest represents image overlay request
type OverlayRequest struct {
	VideoPath string       `json:"video_path" binding:"required"`
	Overlay   ImageOverlay `json:"overlay" binding:"required"`
}

// AudioRequest represents background music request
type AudioRequest struct {
	VideoPath string      `json:"video_path" binding:"required"`
	Audio     AudioConfig `json:"audio" binding:"required"`
}

// CompleteProcessRequest represents complete video processing request
type CompleteProcessRequest struct {
	Segments []VideoSegment `json:"segments" binding:"required,min=1"`
	Overlays []ImageOverlay `json:"overlays,omitempty"`
	Audio    *AudioConfig   `json:"audio,omitempty"`
}

// WebhookHeader represents a custom header for webhook requests
type WebhookHeader struct {
	Key   string `json:"key" example:"x-api-key"`
	Value string `json:"value" example:"loremIPSUM"`
}

// CombineVideosRequest represents request to combine videos from URLs
type CombineVideosRequest struct {
	Videos        []string       `json:"videos" binding:"required,min=2"`
	WebhookURL    string         `json:"webhook_url,omitempty"`
	WebhookHeader *WebhookHeader `json:"webhook_header,omitempty"`
}

// JobResponse represents a job response
type JobResponse struct {
	JobID     string    `json:"job_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Status    JobStatus `json:"status" example:"pending"`
	Message   string    `json:"message" example:"Job created successfully"`
	CreatedAt time.Time `json:"created_at" example:"2025-01-13T10:00:00Z"`
}

// JobStatusResponse represents job status response
type JobStatusResponse struct {
	JobID      string    `json:"job_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	Status     JobStatus `json:"status" example:"processing"`
	Progress   int       `json:"progress" example:"50"` // 0-100
	OutputPath string    `json:"output_path,omitempty" example:"/outputs/result.mp4"`
	S3URL      string    `json:"s3_url,omitempty" example:"https://s3.amazonaws.com/bucket/video.mp4"`
	Error      string    `json:"error,omitempty" example:""`
	CreatedAt  time.Time `json:"created_at" example:"2025-01-13T10:00:00Z"`
	UpdatedAt  time.Time `json:"updated_at" example:"2025-01-13T10:05:00Z"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error" example:"Invalid request"`
	Message string `json:"message,omitempty" example:"Detailed error message"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status  string `json:"status" example:"ok"`
	Version string `json:"version" example:"1.0.0"`
}

// Job represents a processing job
type Job struct {
	ID            string
	Status        JobStatus
	Progress      int
	OutputPath    string
	S3URL         string
	WebhookURL    string
	WebhookHeader *WebhookHeader
	Error         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	mu            sync.RWMutex
}

// NewJob creates a new job
func NewJob(id string) *Job {
	now := time.Now()
	return &Job{
		ID:        id,
		Status:    JobStatusPending,
		Progress:  0,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// UpdateStatus updates job status
func (j *Job) UpdateStatus(status JobStatus) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = status
	j.UpdatedAt = time.Now()
}

// UpdateProgress updates job progress
func (j *Job) UpdateProgress(progress int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Progress = progress
	j.UpdatedAt = time.Now()
}

// SetOutput sets job output path
func (j *Job) SetOutput(path string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.OutputPath = path
	j.UpdatedAt = time.Now()
}

// SetS3URL sets job S3 URL
func (j *Job) SetS3URL(url string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.S3URL = url
	j.UpdatedAt = time.Now()
}

// SetError sets job error
func (j *Job) SetError(err string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Error = err
	j.Status = JobStatusFailed
	j.UpdatedAt = time.Now()
}

// GetStatus returns current job status
func (j *Job) GetStatus() JobStatusResponse {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return JobStatusResponse{
		JobID:      j.ID,
		Status:     j.Status,
		Progress:   j.Progress,
		OutputPath: j.OutputPath,
		S3URL:      j.S3URL,
		Error:      j.Error,
		CreatedAt:  j.CreatedAt,
		UpdatedAt:  j.UpdatedAt,
	}
}

// JobStore manages jobs
type JobStore struct {
	jobs        map[string]*Job
	mu          sync.RWMutex
	persistence *JobPersistence
}

// NewJobStore creates a new job store
func NewJobStore() *JobStore {
	return &JobStore{
		jobs: make(map[string]*Job),
	}
}

// NewJobStoreWithPersistence creates a new job store with persistence
func NewJobStoreWithPersistence(jobsDir string) *JobStore {
	store := &JobStore{
		jobs:        make(map[string]*Job),
		persistence: NewJobPersistence(jobsDir),
	}
	// Load existing jobs from disk
	store.jobs = store.persistence.LoadAllJobs()
	return store
}

// Add adds a job to the store
func (s *JobStore) Add(job *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	// Persist to disk if persistence is enabled
	if s.persistence != nil {
		_ = s.persistence.SaveJob(job)
	}
}

// Get retrieves a job by ID
func (s *JobStore) Get(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

// Update updates an existing job and persists changes
func (s *JobStore) Update(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	// Persist to disk if persistence is enabled
	if s.persistence != nil {
		return s.persistence.SaveJob(job)
	}
	return nil
}

// Delete removes a job from the store
func (s *JobStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	// Delete from disk if persistence is enabled
	if s.persistence != nil {
		_ = s.persistence.DeleteJob(id)
	}
}

// GetJobsDir returns the jobs directory path
func (s *JobStore) GetJobsDir() string {
	if s.persistence == nil {
		return ""
	}
	return s.persistence.GetJobsDir()
}

// UploadResponse represents file upload response
type UploadResponse struct {
	FileName string `json:"file_name" example:"video.mp4"`
	FilePath string `json:"file_path" example:"/uploads/video.mp4"`
	FileSize int64  `json:"file_size" example:"1048576"`
} // @name UploadResponse

// MultiUploadResponse represents multiple file upload response
type MultiUploadResponse struct {
	Files []UploadResponse `json:"files"`
} // @name MultiUploadResponse
