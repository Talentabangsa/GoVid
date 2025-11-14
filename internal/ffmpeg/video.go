package ffmpeg

import (
	"context"
	"fmt"

	ffmpeg "github.com/u2takey/ffmpeg-go"
	"govid/internal/models"
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
	var streams []*ffmpeg.Stream

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
		"n":  len(segments),
		"v":  1,
		"a":  1,
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

	// Prepare streams for concatenation
	var streams []*ffmpeg.Stream
	for _, path := range inputPaths {
		input := ffmpeg.Input(path)
		streams = append(streams, input.Video(), input.Audio())
	}

	// Concatenate and output
	output := ffmpeg.Concat(streams, ffmpeg.KwArgs{
		"n": len(inputPaths),
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
