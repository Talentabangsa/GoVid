package ffmpeg

import (
	"context"
	"fmt"
	"strings"

	"govid/internal/models"
)

// AddBackgroundMusic adds background music to a video with volume control and fade effects
func (e *Executor) AddBackgroundMusic(ctx context.Context, videoPath string, audio models.AudioConfig, outputPath string) error {
	// Validate files
	if err := ValidateFile(videoPath); err != nil {
		return fmt.Errorf("video file: %w", err)
	}
	if err := ValidateFile(audio.FilePath); err != nil {
		return fmt.Errorf("audio file: %w", err)
	}

	// Build FFmpeg command
	args := []string{"-y", "-i", videoPath, "-i", audio.FilePath}

	// Build audio filter chain
	audioFilter := buildAudioFilter(audio)

	args = append(args, "-filter_complex", audioFilter)
	args = append(args, "-map", "0:v", "-map", "[aout]")
	args = append(args, "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", outputPath)

	return e.Execute(ctx, args)
}

// buildAudioFilter builds the audio filter string with trimming, fade, and volume control
func buildAudioFilter(audio models.AudioConfig) string {
	var filters []string

	// Start with audio input
	audioStream := "[1:a]"

	// Add trim filter if specified
	if audio.StartTime != nil || audio.EndTime != nil {
		var trimFilter string
		switch {
		case audio.StartTime != nil && audio.EndTime != nil:
			trimFilter = fmt.Sprintf("atrim=start=%.2f:end=%.2f", *audio.StartTime, *audio.EndTime)
		case audio.StartTime != nil:
			trimFilter = fmt.Sprintf("atrim=start=%.2f", *audio.StartTime)
		case audio.EndTime != nil:
			trimFilter = fmt.Sprintf("atrim=end=%.2f", *audio.EndTime)
		}
		audioStream += trimFilter + ","
	}

	// Reset timestamps after trim
	if audio.StartTime != nil || audio.EndTime != nil {
		audioStream += "asetpts=PTS-STARTPTS,"
	}

	// Add fade in effect
	if audio.FadeIn != nil && *audio.FadeIn > 0 {
		fadeIn := fmt.Sprintf("afade=t=in:st=0:d=%.2f", *audio.FadeIn)
		audioStream += fadeIn + ","
	}

	// Add fade out effect
	if audio.FadeOut != nil && *audio.FadeOut > 0 {
		// Calculate fade out start time
		var fadeOutStart float64
		switch {
		case audio.EndTime != nil && audio.StartTime != nil:
			fadeOutStart = *audio.EndTime - *audio.StartTime - *audio.FadeOut
		case audio.EndTime != nil:
			fadeOutStart = *audio.EndTime - *audio.FadeOut
		default:
			// Use a default duration if not specified (will be calculated by FFmpeg)
			fadeOutStart = 0 // FFmpeg will handle this
		}

		if fadeOutStart > 0 {
			fadeOut := fmt.Sprintf("afade=t=out:st=%.2f:d=%.2f", fadeOutStart, *audio.FadeOut)
			audioStream += fadeOut + ","
		} else {
			fadeOut := fmt.Sprintf("afade=t=out:d=%.2f", *audio.FadeOut)
			audioStream += fadeOut + ","
		}
	}

	// Add volume control
	volumeFilter := fmt.Sprintf("volume=%.2f", audio.Volume)
	audioStream += volumeFilter

	// Finalize audio stream label
	audioStream += "[music]"
	filters = append(filters, audioStream)

	// Mix with original video audio
	mixFilter := "[0:a][music]amix=inputs=2:duration=first:dropout_transition=2[aout]"
	filters = append(filters, mixFilter)

	return strings.Join(filters, ";")
}

// ReplaceAudio replaces video audio completely with background music (no mixing)
func (e *Executor) ReplaceAudio(ctx context.Context, videoPath string, audio models.AudioConfig, outputPath string) error {
	// Validate files
	if err := ValidateFile(videoPath); err != nil {
		return fmt.Errorf("video file: %w", err)
	}
	if err := ValidateFile(audio.FilePath); err != nil {
		return fmt.Errorf("audio file: %w", err)
	}

	// Build FFmpeg command
	args := []string{"-y", "-i", videoPath, "-i", audio.FilePath}

	// Build audio filter (without mixing)
	audioFilter := buildAudioFilterNoMix(audio)

	args = append(args, "-filter_complex", audioFilter)
	args = append(args, "-map", "0:v", "-map", "[aout]")
	args = append(args, "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", outputPath)

	return e.Execute(ctx, args)
}

// buildAudioFilterNoMix builds audio filter without mixing with original audio
func buildAudioFilterNoMix(audio models.AudioConfig) string {
	audioStream := "[1:a]"

	// Add trim filter if specified
	if audio.StartTime != nil || audio.EndTime != nil {
		var trimFilter string
		switch {
		case audio.StartTime != nil && audio.EndTime != nil:
			trimFilter = fmt.Sprintf("atrim=start=%.2f:end=%.2f", *audio.StartTime, *audio.EndTime)
		case audio.StartTime != nil:
			trimFilter = fmt.Sprintf("atrim=start=%.2f", *audio.StartTime)
		case audio.EndTime != nil:
			trimFilter = fmt.Sprintf("atrim=end=%.2f", *audio.EndTime)
		}
		audioStream += trimFilter + ","
	}

	// Reset timestamps after trim
	if audio.StartTime != nil || audio.EndTime != nil {
		audioStream += "asetpts=PTS-STARTPTS,"
	}

	// Add fade in
	if audio.FadeIn != nil && *audio.FadeIn > 0 {
		audioStream += fmt.Sprintf("afade=t=in:st=0:d=%.2f,", *audio.FadeIn)
	}

	// Add fade out
	if audio.FadeOut != nil && *audio.FadeOut > 0 {
		var fadeOutStart float64
		if audio.EndTime != nil && audio.StartTime != nil {
			fadeOutStart = *audio.EndTime - *audio.StartTime - *audio.FadeOut
		} else {
			fadeOutStart = 0
		}

		if fadeOutStart > 0 {
			audioStream += fmt.Sprintf("afade=t=out:st=%.2f:d=%.2f,", fadeOutStart, *audio.FadeOut)
		} else {
			audioStream += fmt.Sprintf("afade=t=out:d=%.2f,", *audio.FadeOut)
		}
	}

	// Add volume
	audioStream += fmt.Sprintf("volume=%.2f[aout]", audio.Volume)

	return audioStream
}

// CompleteProcess performs complete video processing with merge, overlay, and audio
func (e *Executor) CompleteProcess(ctx context.Context, req models.CompleteProcessRequest, outputPath string) error {
	// For simplicity, we'll process in stages using temp files
	// In production, you might want to combine everything into one filter_complex

	// Stage 1: Merge videos if multiple segments
	var currentVideo string
	switch {
	case len(req.Segments) > 1:
		tempMerged := outputPath + ".merged.mp4"
		if err := e.MergeVideos(ctx, req.Segments, tempMerged); err != nil {
			return fmt.Errorf("merge videos: %w", err)
		}
		currentVideo = tempMerged
	case len(req.Segments) == 1:
		currentVideo = req.Segments[0].FilePath
	default:
		return fmt.Errorf("at least one video segment required")
	}

	// Stage 2: Add overlays if specified
	if len(req.Overlays) > 0 {
		tempOverlay := outputPath + ".overlay.mp4"
		if err := e.AddMultipleOverlays(ctx, currentVideo, req.Overlays, tempOverlay); err != nil {
			return fmt.Errorf("add overlays: %w", err)
		}
		currentVideo = tempOverlay
	}

	// Stage 3: Add audio if specified
	if req.Audio != nil {
		if err := e.AddBackgroundMusic(ctx, currentVideo, *req.Audio, outputPath); err != nil {
			return fmt.Errorf("add audio: %w", err)
		}
	} else {
		// Just copy the current video to output
		args := []string{"-y", "-i", currentVideo, "-c", "copy", outputPath}
		if err := e.Execute(ctx, args); err != nil {
			return fmt.Errorf("copy video: %w", err)
		}
	}

	return nil
}
