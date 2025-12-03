package cleanup

import (
	"os"
	"path/filepath"
	"time"

	"govid/internal/models"
	"govid/pkg/logger"
)

// Scheduler handles periodic cleanup of old files and jobs
type Scheduler struct {
	outputDir     string
	uploadDir     string
	tempDir       string
	jobStore      *models.JobStore
	retentionDays int
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

// NewScheduler creates a new cleanup scheduler
func NewScheduler(outputDir, uploadDir, tempDir string, jobStore *models.JobStore, retentionDays int) *Scheduler {
	return &Scheduler{
		outputDir:     outputDir,
		uploadDir:     uploadDir,
		tempDir:       tempDir,
		jobStore:      jobStore,
		retentionDays: retentionDays,
		stopChan:      make(chan struct{}),
	}
}

// Start begins the cleanup scheduler
func (s *Scheduler) Start() {
	logger.Info("Starting cleanup scheduler (retention: %d days)", s.retentionDays)

	// Run cleanup immediately on start
	go s.runCleanup()

	// Schedule cleanup every 24 hours
	s.cleanupTicker = time.NewTicker(24 * time.Hour)

	go func() {
		for {
			select {
			case <-s.cleanupTicker.C:
				s.runCleanup()
			case <-s.stopChan:
				s.cleanupTicker.Stop()
				return
			}
		}
	}()
}

// Stop stops the cleanup scheduler
func (s *Scheduler) Stop() {
	logger.Info("Stopping cleanup scheduler")
	close(s.stopChan)
}

// runCleanup performs the cleanup operation
func (s *Scheduler) runCleanup() {
	logger.Info("Running scheduled cleanup...")
	startTime := time.Now()

	cutoffTime := time.Now().AddDate(0, 0, -s.retentionDays)
	logger.Info("Cleaning up files and jobs older than %s", cutoffTime.Format(time.RFC3339))

	totalFilesDeleted := 0
	totalJobsDeleted := 0

	// Clean outputs directory
	filesDeleted := s.cleanDirectory(s.outputDir, cutoffTime)
	totalFilesDeleted += filesDeleted
	logger.Info("Cleaned %d files from outputs directory", filesDeleted)

	// Clean uploads directory
	filesDeleted = s.cleanDirectory(s.uploadDir, cutoffTime)
	totalFilesDeleted += filesDeleted
	logger.Info("Cleaned %d files from uploads directory", filesDeleted)

	// Clean temp directory (always clean all files older than cutoff)
	filesDeleted = s.cleanDirectory(s.tempDir, cutoffTime)
	totalFilesDeleted += filesDeleted
	logger.Info("Cleaned %d files from temp directory", filesDeleted)

	// Clean old jobs
	totalJobsDeleted = s.cleanOldJobs(cutoffTime)
	logger.Info("Cleaned %d old jobs", totalJobsDeleted)

	duration := time.Since(startTime)
	logger.Info("Cleanup completed in %s (deleted %d files, %d jobs)", duration, totalFilesDeleted, totalJobsDeleted)
}

// cleanDirectory removes files older than cutoffTime from a directory
func (s *Scheduler) cleanDirectory(dir string, cutoffTime time.Time) int {
	filesDeleted := 0

	entries, err := os.ReadDir(dir)
	if err != nil {
		logger.Error("Failed to read directory %s: %v", dir, err)
		return 0
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			logger.Error("Failed to get file info for %s: %v", filePath, err)
			continue
		}

		// Check if file is older than cutoff time
		if info.ModTime().Before(cutoffTime) {
			if err := os.Remove(filePath); err != nil {
				logger.Error("Failed to delete file %s: %v", filePath, err)
			} else {
				logger.Debug("Deleted old file: %s (modified: %s)", filePath, info.ModTime().Format(time.RFC3339))
				filesDeleted++
			}
		}
	}

	return filesDeleted
}

// cleanOldJobs removes jobs older than cutoffTime
func (s *Scheduler) cleanOldJobs(cutoffTime time.Time) int {
	jobsDeleted := 0

	// Get all job IDs (we need to implement a method to list all jobs)
	// For now, we'll read from the jobs directory directly
	jobsDir := filepath.Dir(s.jobStore.GetJobsDir())
	if jobsDir == "" {
		// If we can't get jobs dir, skip job cleanup
		return 0
	}

	entries, err := os.ReadDir(jobsDir)
	if err != nil {
		logger.Error("Failed to read jobs directory: %v", err)
		return 0
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		jobFilePath := filepath.Join(jobsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			logger.Error("Failed to get job file info for %s: %v", jobFilePath, err)
			continue
		}

		// Check if job file is older than cutoff time
		if info.ModTime().Before(cutoffTime) {
			// Extract job ID from filename (remove .json extension)
			jobID := entry.Name()[:len(entry.Name())-5]

			// Delete from job store
			s.jobStore.Delete(jobID)
			jobsDeleted++
			logger.Debug("Deleted old job: %s (modified: %s)", jobID, info.ModTime().Format(time.RFC3339))
		}
	}

	return jobsDeleted
}
