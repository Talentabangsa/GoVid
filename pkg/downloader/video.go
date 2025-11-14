package downloader

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
)

// VideoDownloader handles downloading videos from URLs
type VideoDownloader struct {
	tempDir string
}

// NewVideoDownloader creates a new video downloader
func NewVideoDownloader(tempDir string) *VideoDownloader {
	return &VideoDownloader{
		tempDir: tempDir,
	}
}

// DownloadResult contains the result of a download operation
type DownloadResult struct {
	Index    int
	FilePath string
	Error    error
}

// DownloadVideosInOrder downloads videos from URLs while preserving order
func (d *VideoDownloader) DownloadVideosInOrder(urls []string) ([]string, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}

	// Create a results channel
	results := make(chan DownloadResult, len(urls))
	var wg sync.WaitGroup

	// Download videos concurrently
	for i, url := range urls {
		wg.Add(1)
		go func(index int, videoURL string) {
			defer wg.Done()

			filePath, err := d.downloadVideo(videoURL, index)
			results <- DownloadResult{
				Index:    index,
				FilePath: filePath,
				Error:    err,
			}
		}(i, url)
	}

	// Wait for all downloads to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results and maintain order
	downloadedFiles := make([]string, len(urls))
	for result := range results {
		if result.Error != nil {
			// Clean up already downloaded files
			for _, path := range downloadedFiles {
				if path != "" {
					os.Remove(path)
				}
			}
			return nil, fmt.Errorf("failed to download video %d: %w", result.Index, result.Error)
		}
		downloadedFiles[result.Index] = result.FilePath
	}

	return downloadedFiles, nil
}

// downloadVideo downloads a single video from a URL
func (d *VideoDownloader) downloadVideo(url string, index int) (string, error) {
	// Create HTTP request
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to download from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	// Generate unique filename
	filename := fmt.Sprintf("%s_%d.mp4", uuid.New().String(), index)
	filePath := filepath.Join(d.tempDir, filename)

	// Create the file
	out, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Write the response body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(filePath)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filePath, nil
}

// CleanupFiles removes downloaded files
func (d *VideoDownloader) CleanupFiles(filePaths []string) {
	for _, path := range filePaths {
		if path != "" {
			os.Remove(path)
		}
	}
}
