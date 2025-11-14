package ffmpeg

import (
	"context"
	"fmt"

	ffmpeg "github.com/u2takey/ffmpeg-go"
	"govid/internal/models"
)

// AddImageOverlay adds an image overlay to a video with animations
func (e *Executor) AddImageOverlay(ctx context.Context, videoPath string, overlay models.ImageOverlay, outputPath string) error {
	// Validate files
	if err := ValidateFile(videoPath); err != nil {
		return fmt.Errorf("video file: %w", err)
	}
	if err := ValidateFile(overlay.FilePath); err != nil {
		return fmt.Errorf("overlay image: %w", err)
	}

	// Build overlay stream with filters
	overlayStream := ffmpeg.Input(overlay.FilePath)

	// Always apply format for transparency
	overlayStream = overlayStream.Filter("format", ffmpeg.Args{"rgba"})

	// Apply animation filters
	switch overlay.Animation {
	case models.AnimationFade:
		duration := 1.0
		if overlay.FadeDuration != nil {
			duration = *overlay.FadeDuration
		}
		totalDuration := overlay.EndTime - overlay.StartTime

		// Fade in
		overlayStream = overlayStream.Filter("fade", ffmpeg.Args{}, ffmpeg.KwArgs{
			"t":     "in",
			"st":    0,
			"d":     duration,
			"alpha": 1,
		})

		// Fade out
		overlayStream = overlayStream.Filter("fade", ffmpeg.Args{}, ffmpeg.KwArgs{
			"t":     "out",
			"st":    totalDuration - duration,
			"d":     duration,
			"alpha": 1,
		})

	case models.AnimationZoom:
		zoomFrom := 1.0
		zoomTo := 1.5
		if overlay.ZoomFrom != nil {
			zoomFrom = *overlay.ZoomFrom
		}
		if overlay.ZoomTo != nil {
			zoomTo = *overlay.ZoomTo
		}
		duration := overlay.EndTime - overlay.StartTime
		zoomRate := (zoomTo - zoomFrom) / duration

		overlayStream = overlayStream.Filter("zoompan", ffmpeg.Args{}, ffmpeg.KwArgs{
			"z": fmt.Sprintf("if(lte(zoom,%.2f),zoom+%.6f,%.2f)", zoomTo, zoomRate, zoomTo),
			"d": 1,
			"s": "1280x720",
		})
	}

	// Calculate position
	x, y := calculatePosition(overlay)

	// Handle slide animation in overlay position
	if overlay.Animation == models.AnimationSlide && overlay.SlideDirection != nil {
		duration := 1.0
		if overlay.SlideDuration != nil {
			duration = *overlay.SlideDuration
		}
		x, y = calculateSlidePosition(overlay, x, y, duration)
	}

	// Build overlay with position and timing
	videoStream := ffmpeg.Input(videoPath)

	// Apply overlay using Filter method
	// Position goes in Args as "x:y", enable goes in KwArgs
	positionArg := fmt.Sprintf("%s:%s", x, y)

	output := ffmpeg.Filter(
		[]*ffmpeg.Stream{videoStream, overlayStream},
		"overlay",
		ffmpeg.Args{positionArg},
		ffmpeg.KwArgs{
			"enable": fmt.Sprintf("between(t,%.2f,%.2f)", overlay.StartTime, overlay.EndTime),
		},
	).Output(outputPath, ffmpeg.KwArgs{
		"c:v":    "libx264",
		"preset": "medium",
		"crf":    "23",
		"c:a":    "copy",
	}).OverWriteOutput()

	return output.Run()
}


// calculatePosition calculates x,y position based on preset or custom values
func calculatePosition(overlay models.ImageOverlay) (string, string) {
	// If custom position is specified
	if overlay.Position == models.PositionCustom {
		if overlay.X != nil && overlay.Y != nil {
			return fmt.Sprintf("%d", *overlay.X), fmt.Sprintf("%d", *overlay.Y)
		}
	}

	// Predefined positions
	switch overlay.Position {
	case models.PositionTopLeft:
		return "10", "10"
	case models.PositionTopRight:
		return "(main_w-overlay_w-10)", "10"
	case models.PositionBottomLeft:
		return "10", "(main_h-overlay_h-10)"
	case models.PositionBottomRight:
		return "(main_w-overlay_w-10)", "(main_h-overlay_h-10)"
	case models.PositionCenter:
		return "(main_w-overlay_w)/2", "(main_h-overlay_h)/2"
	default:
		return "10", "10" // Default to top-left
	}
}

// calculateSlidePosition calculates position for slide animation
func calculateSlidePosition(overlay models.ImageOverlay, baseX, baseY string, duration float64) (string, string) {
	t := fmt.Sprintf("(t-%.2f)", overlay.StartTime)
	progress := fmt.Sprintf("min(%s/%.2f,1)", t, duration)

	switch *overlay.SlideDirection {
	case models.SlideFromLeft:
		// Start from left (-overlay_w) and slide to baseX
		x := fmt.Sprintf("if(lt(t,%.2f),-overlay_w,-overlay_w+(%s)*(overlay_w+%s))",
			overlay.StartTime, progress, baseX)
		return x, baseY

	case models.SlideFromRight:
		// Start from right (main_w) and slide to baseX
		x := fmt.Sprintf("if(lt(t,%.2f),main_w,main_w-(%s)*(main_w-%s))",
			overlay.StartTime, progress, baseX)
		return x, baseY

	case models.SlideFromTop:
		// Start from top (-overlay_h) and slide to baseY
		y := fmt.Sprintf("if(lt(t,%.2f),-overlay_h,-overlay_h+(%s)*(overlay_h+%s))",
			overlay.StartTime, progress, baseY)
		return baseX, y

	case models.SlideFromBottom:
		// Start from bottom (main_h) and slide to baseY
		y := fmt.Sprintf("if(lt(t,%.2f),main_h,main_h-(%s)*(main_h-%s))",
			overlay.StartTime, progress, baseY)
		return baseX, y

	default:
		return baseX, baseY
	}
}

// AddMultipleOverlays adds multiple image overlays to a video
func (e *Executor) AddMultipleOverlays(ctx context.Context, videoPath string, overlays []models.ImageOverlay, outputPath string) error {
	if len(overlays) == 0 {
		return fmt.Errorf("no overlays provided")
	}

	// Validate files
	if err := ValidateFile(videoPath); err != nil {
		return fmt.Errorf("video file: %w", err)
	}
	for i, overlay := range overlays {
		if err := ValidateFile(overlay.FilePath); err != nil {
			return fmt.Errorf("overlay %d image: %w", i, err)
		}
	}

	// Start with video input
	currentStream := ffmpeg.Input(videoPath)

	// Apply each overlay sequentially
	for _, overlay := range overlays {
		overlayStream := ffmpeg.Input(overlay.FilePath).Filter("format", ffmpeg.Args{"rgba"})

		// Apply fade animation if specified
		if overlay.Animation == models.AnimationFade && overlay.FadeDuration != nil {
			duration := *overlay.FadeDuration
			totalDuration := overlay.EndTime - overlay.StartTime

			overlayStream = overlayStream.Filter("fade", ffmpeg.Args{}, ffmpeg.KwArgs{
				"t":     "in",
				"st":    0,
				"d":     duration,
				"alpha": 1,
			}).Filter("fade", ffmpeg.Args{}, ffmpeg.KwArgs{
				"t":     "out",
				"st":    totalDuration - duration,
				"d":     duration,
				"alpha": 1,
			})
		}

		// Calculate position
		x, y := calculatePosition(overlay)

		// Apply overlay
		currentStream = ffmpeg.Filter(
			[]*ffmpeg.Stream{currentStream, overlayStream},
			"overlay",
			ffmpeg.Args{},
			ffmpeg.KwArgs{
				"x":      x,
				"y":      y,
				"enable": fmt.Sprintf("between(t,%.2f,%.2f)", overlay.StartTime, overlay.EndTime),
			},
		)
	}

	// Output
	output := currentStream.Output(outputPath, ffmpeg.KwArgs{
		"c:v":    "libx264",
		"preset": "medium",
		"crf":    "23",
		"c:a":    "copy",
	}).OverWriteOutput()

	return output.Run()
}
