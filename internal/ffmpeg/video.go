package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govid/internal/models"

	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// MergeVideos merges multiple video segments with custom timeframes
func (e *Executor) MergeVideos(ctx context.Context, segments []models.VideoSegment, outputPath string) error {
	if len(segments) < 2 {
		return fmt.Errorf("at least 2 video segments required for merging")
	}

	// Validate all input files
	for i, seg := range segments {
		if err := ValidateFile(seg.FilePath); err != nil {
			return fmt.Errorf("segment %d: %w", i, err)
		}
	}

	// Process each segment with trim and setpts
	streams := make([]*ffmpeg.Stream, 0, len(segments)*2)

	for _, seg := range segments {
		input := ffmpeg.Input(seg.FilePath)

		// Trim video stream
		var videoStream *ffmpeg.Stream
		if seg.EndTime > 0 {
			videoStream = input.Video().Trim(ffmpeg.KwArgs{
				"start": seg.StartTime,
				"end":   seg.EndTime,
			}).SetPts("PTS-STARTPTS").Stream("", "")
		} else {
			if seg.StartTime > 0 {
				videoStream = input.Video().Trim(ffmpeg.KwArgs{
					"start": seg.StartTime,
				}).SetPts("PTS-STARTPTS").Stream("", "")
			} else {
				videoStream = input.Video()
			}
		}

		// Trim audio stream
		var audioStream *ffmpeg.Stream
		if seg.EndTime > 0 {
			audioStream = input.Audio().Filter("atrim", ffmpeg.Args{}, ffmpeg.KwArgs{
				"start": seg.StartTime,
				"end":   seg.EndTime,
			}).Filter("asetpts", ffmpeg.Args{"PTS-STARTPTS"})
		} else {
			if seg.StartTime > 0 {
				audioStream = input.Audio().Filter("atrim", ffmpeg.Args{}, ffmpeg.KwArgs{
					"start": seg.StartTime,
				}).Filter("asetpts", ffmpeg.Args{"PTS-STARTPTS"})
			} else {
				audioStream = input.Audio()
			}
		}

		streams = append(streams, videoStream, audioStream)
	}

	// Concatenate all streams
	output := ffmpeg.Concat(streams, ffmpeg.KwArgs{
		"n": len(segments),
		"v": 1,
		"a": 1,
	}).Output(outputPath, ffmpeg.KwArgs{
		"c:v":    "libx264",
		"preset": "medium",
		"crf":    "23",
		"c:a":    "aac",
		"b:a":    "192k",
	}).OverWriteOutput()

	return output.Run()
}

// MergeVideosSimple merges videos without timeframe trimming (concatenation only)
func (e *Executor) MergeVideosSimple(ctx context.Context, inputPaths []string, outputPath string) error {
	if len(inputPaths) < 2 {
		return fmt.Errorf("at least 2 video files required for merging")
	}

	// Create temporary concat file list
	concatFile, err := os.CreateTemp("", "concat-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create concat file: %w", err)
	}
	defer os.Remove(concatFile.Name())
	defer concatFile.Close()

	// Write file list in concat demuxer format
	for _, path := range inputPaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for %s: %w", path, err)
		}
		// Escape single quotes in the path
		escapedPath := strings.ReplaceAll(absPath, "'", "'\\''")
		_, err = fmt.Fprintf(concatFile, "file '%s'\n", escapedPath)
		if err != nil {
			return fmt.Errorf("failed to write concat file: %w", err)
		}
	}
	concatFile.Close()

	// Use concat demuxer protocol
	output := ffmpeg.Input(concatFile.Name(), ffmpeg.KwArgs{
		"f":    "concat",
		"safe": "0",
	}).Output(outputPath, ffmpeg.KwArgs{
		"c:v":    "libx264",
		"preset": "medium",
		"crf":    "23",
		"c:a":    "aac",
		"b:a":    "192k",
	}).OverWriteOutput()

	return output.Run()
}
