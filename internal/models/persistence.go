package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"govid/pkg/logger"
)

// JobPersistence handles saving and loading jobs from disk
type JobPersistence struct {
	jobsDir string
	mu      sync.RWMutex
}

// NewJobPersistence creates a new job persistence layer
func NewJobPersistence(jobsDir string) *JobPersistence {
	return &JobPersistence{
		jobsDir: jobsDir,
	}
}

// jobData is the serializable representation of a job
type jobData struct {
	ID         string    `json:"id"`
	Status     JobStatus `json:"status"`
	Progress   int       `json:"progress"`
	OutputPath string    `json:"output_path"`
	Error      string    `json:"error"`
	CreatedAt  string    `json:"created_at"`
	UpdatedAt  string    `json:"updated_at"`
}

// SaveJob saves a job to disk
func (jp *JobPersistence) SaveJob(job *Job) error {
	jp.mu.Lock()
	defer jp.mu.Unlock()

	status := job.GetStatus()

	data := jobData{
		ID:         status.JobID,
		Status:     status.Status,
		Progress:   status.Progress,
		OutputPath: status.OutputPath,
		Error:      status.Error,
		CreatedAt:  status.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:  status.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	filePath := filepath.Join(jp.jobsDir, fmt.Sprintf("%s.json", status.JobID))

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		logger.Error("Failed to marshal job %s: %v", status.JobID, err)
		return err
	}

	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		logger.Error("Failed to save job %s to %s: %v", status.JobID, filePath, err)
		return err
	}

	logger.Debug("Job %s persisted to disk", status.JobID)
	return nil
}

// LoadJob loads a single job from disk
func (jp *JobPersistence) LoadJob(jobID string) (*Job, error) {
	jp.mu.RLock()
	defer jp.mu.RUnlock()

	filePath := filepath.Join(jp.jobsDir, fmt.Sprintf("%s.json", jobID))

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var data jobData
	if err := json.Unmarshal(content, &data); err != nil {
		logger.Error("Failed to unmarshal job %s: %v", jobID, err)
		return nil, err
	}

	// Reconstruct the job
	job := NewJob(data.ID)
	job.Status = data.Status
	job.Progress = data.Progress
	job.OutputPath = data.OutputPath
	job.Error = data.Error
	job.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z07:00", data.CreatedAt)
	job.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z07:00", data.UpdatedAt)

	return job, nil
}

// LoadAllJobs loads all jobs from disk
func (jp *JobPersistence) LoadAllJobs() map[string]*Job {
	jp.mu.RLock()
	defer jp.mu.RUnlock()

	jobs := make(map[string]*Job)

	entries, err := os.ReadDir(jp.jobsDir)
	if err != nil {
		logger.Error("Failed to read jobs directory: %v", err)
		return jobs
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		jobID := entry.Name()[:len(entry.Name())-5] // remove .json

		filePath := filepath.Join(jp.jobsDir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("Failed to read job file %s: %v", filePath, err)
			continue
		}

		var data jobData
		if err := json.Unmarshal(content, &data); err != nil {
			logger.Error("Failed to unmarshal job %s: %v", jobID, err)
			continue
		}

		job := NewJob(data.ID)
		job.Status = data.Status
		job.Progress = data.Progress
		job.OutputPath = data.OutputPath
		job.Error = data.Error
		job.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z07:00", data.CreatedAt)
		job.UpdatedAt, _ = time.Parse("2006-01-02T15:04:05Z07:00", data.UpdatedAt)

		jobs[job.ID] = job
		logger.Debug("Loaded job from disk: %s", job.ID)
	}

	logger.Info("Loaded %d jobs from disk", len(jobs))
	return jobs
}

// DeleteJob deletes a job file from disk
func (jp *JobPersistence) DeleteJob(jobID string) error {
	jp.mu.Lock()
	defer jp.mu.Unlock()

	filePath := filepath.Join(jp.jobsDir, fmt.Sprintf("%s.json", jobID))

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		logger.Error("Failed to delete job file %s: %v", filePath, err)
		return err
	}

	logger.Debug("Job %s deleted from disk", jobID)
	return nil
}
