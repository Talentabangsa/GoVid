package ffmpeg

import (
	"context"
	"fmt"
	"strings"

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

	// Build FFmpeg command
	args := []string{"-y"} // Overwrite output file

	// Add input files
	for _, seg := range segments {
		args = append(args, "-i", seg.FilePath)
	}

	// Build filter_complex for trimming and concatenation
	filterParts := make([]string, 0, len(segments)*2+1)
	concatInputs := make([]string, 0, len(segments))

	for i, seg := range segments {
		// Build trim filter for video
		var trimFilter string
		if seg.EndTime > 0 {
			trimFilter = fmt.Sprintf("[%d:v]trim=start=%.2f:end=%.2f,setpts=PTS-STARTPTS[v%d]",
				i, seg.StartTime, seg.EndTime, i)
		} else {
			trimFilter = fmt.Sprintf("[%d:v]trim=start=%.2f,setpts=PTS-STARTPTS[v%d]",
				i, seg.StartTime, i)
		}
		filterParts = append(filterParts, trimFilter)

		// Build trim filter for audio
		var atrimFilter string
		if seg.EndTime > 0 {
			atrimFilter = fmt.Sprintf("[%d:a]atrim=start=%.2f:end=%.2f,asetpts=PTS-STARTPTS[a%d]",
				i, seg.StartTime, seg.EndTime, i)
		} else {
			atrimFilter = fmt.Sprintf("[%d:a]atrim=start=%.2f,asetpts=PTS-STARTPTS[a%d]",
				i, seg.StartTime, i)
		}
		filterParts = append(filterParts, atrimFilter)

		// Add to concat inputs
		concatInputs = append(concatInputs, fmt.Sprintf("[v%d][a%d]", i, i))
	}

	// Build concat filter
	concatFilter := fmt.Sprintf("%sconcat=n=%d:v=1:a=1[outv][outa]",
		strings.Join(concatInputs, ""), len(segments))
	filterParts = append(filterParts, concatFilter)

	// Combine all filters
	filterComplex := BuildFilterComplex(filterParts)
	args = append(args, "-filter_complex", filterComplex)

	// Map output
	args = append(args, "-map", "[outv]", "-map", "[outa]")

	// Output settings
	args = append(args,
		"-c:v", "libx264",    // Video codec
		"-preset", "medium",   // Encoding speed
		"-crf", "23",         // Quality (lower is better, 23 is default)
		"-c:a", "aac",        // Audio codec
		"-b:a", "192k",       // Audio bitrate
		outputPath,
	)

	return e.Execute(ctx, args)
}

// MergeVideosSimple merges videos without timeframe trimming (concatenation only)
func (e *Executor) MergeVideosSimple(ctx context.Context, inputPaths []string, outputPath string) error {
	if len(inputPaths) < 2 {
		return fmt.Errorf("at least 2 video files required for merging")
	}

	// Build FFmpeg command
	args := []string{"-y"} // Overwrite output file

	// Add input files
	for _, path := range inputPaths {
		args = append(args, "-i", path)
	}

	// Build filter_complex for concatenation
	concatInputs := make([]string, 0, len(inputPaths))
	for i := range inputPaths {
		concatInputs = append(concatInputs, fmt.Sprintf("[%d:v][%d:a]", i, i))
	}

	filterComplex := fmt.Sprintf("%sconcat=n=%d:v=1:a=1[outv][outa]",
		strings.Join(concatInputs, ""), len(inputPaths))

	args = append(args, "-filter_complex", filterComplex)
	args = append(args, "-map", "[outv]", "-map", "[outa]")

	// Output settings
	args = append(args,
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "192k",
		outputPath,
	)

	return e.Execute(ctx, args)
}
