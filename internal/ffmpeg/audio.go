package ffmpeg

import (
	"context"
	"fmt"

	"govid/internal/models"

	ffmpeg "github.com/u2takey/ffmpeg-go"
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

	// Load video and audio
	videoStream := ffmpeg.Input(videoPath)
	audioStream := ffmpeg.Input(audio.FilePath).Audio()

	// Apply audio filters
	audioStream = applyAudioFilters(audioStream, audio)

	// Mix with original video audio
	mixedAudio := ffmpeg.Filter(
		[]*ffmpeg.Stream{videoStream.Audio(), audioStream},
		"amix",
		ffmpeg.Args{},
		ffmpeg.KwArgs{
			"inputs":             2,
			"duration":           "first",
			"dropout_transition": 2,
		},
	)

	// Output with video and mixed audio
	output := ffmpeg.Output(
		[]*ffmpeg.Stream{videoStream.Video(), mixedAudio},
		outputPath,
		ffmpeg.KwArgs{
			"c:v": "copy",
			"c:a": "aac",
			"b:a": "192k",
		},
	).OverWriteOutput()

	return output.Run()
}

// applyAudioFilters applies trim, fade, and volume filters to audio stream
func applyAudioFilters(audioStream *ffmpeg.Stream, audio models.AudioConfig) *ffmpeg.Stream {
	// Apply trim filter if specified
	if audio.StartTime != nil || audio.EndTime != nil {
		trimKwArgs := ffmpeg.KwArgs{}
		if audio.StartTime != nil {
			trimKwArgs["start"] = *audio.StartTime
		}
		if audio.EndTime != nil {
			trimKwArgs["end"] = *audio.EndTime
		}
		audioStream = audioStream.Filter("atrim", ffmpeg.Args{}, trimKwArgs)
		audioStream = audioStream.Filter("asetpts", ffmpeg.Args{"PTS-STARTPTS"})
	}

	// Add fade in effect
	if audio.FadeIn != nil && *audio.FadeIn > 0 {
		audioStream = audioStream.Filter("afade", ffmpeg.Args{}, ffmpeg.KwArgs{
			"t":  "in",
			"st": 0,
			"d":  *audio.FadeIn,
		})
	}

	// Add fade out effect
	if audio.FadeOut != nil && *audio.FadeOut > 0 {
		fadeKwArgs := ffmpeg.KwArgs{
			"t": "out",
			"d": *audio.FadeOut,
		}

		// Calculate fade out start time if we have end time
		if audio.EndTime != nil && audio.StartTime != nil {
			fadeOutStart := *audio.EndTime - *audio.StartTime - *audio.FadeOut
			if fadeOutStart > 0 {
				fadeKwArgs["st"] = fadeOutStart
			}
		}

		audioStream = audioStream.Filter("afade", ffmpeg.Args{}, fadeKwArgs)
	}

	// Add volume control
	audioStream = audioStream.Filter("volume", ffmpeg.Args{fmt.Sprintf("%.2f", audio.Volume)})

	return audioStream
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

	// Load video and audio
	videoStream := ffmpeg.Input(videoPath).Video()
	audioStream := ffmpeg.Input(audio.FilePath).Audio()

	// Apply audio filters
	audioStream = applyAudioFilters(audioStream, audio)

	// Output with video and replacement audio
	output := ffmpeg.Output(
		[]*ffmpeg.Stream{videoStream, audioStream},
		outputPath,
		ffmpeg.KwArgs{
			"c:v":      "copy",
			"c:a":      "aac",
			"b:a":      "192k",
			"shortest": nil, // Use shortest input duration
		},
	).OverWriteOutput()

	return output.Run()
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
		output := ffmpeg.Input(currentVideo).Output(outputPath, ffmpeg.KwArgs{
			"c": "copy",
		}).OverWriteOutput()

		if err := output.Run(); err != nil {
			return fmt.Errorf("copy video: %w", err)
		}
	}

	return nil
}
