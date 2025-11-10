package ffmpeg

import (
	"context"
	"fmt"
	"strings"

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

	// Build FFmpeg command
	args := []string{"-y", "-i", videoPath, "-i", overlay.FilePath}

	// Build filter chain for the overlay
	filterChain := buildOverlayFilter(overlay)

	args = append(args, "-filter_complex", filterChain)
	args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-c:a", "copy", outputPath)

	return e.Execute(ctx, args)
}

// buildOverlayFilter builds the overlay filter string with animations
func buildOverlayFilter(overlay models.ImageOverlay) string {
	var filters []string

	// Start with the image input
	imageFilter := "[1:v]"

	// Add animation filters
	switch overlay.Animation {
	case models.AnimationFade:
		duration := 1.0
		if overlay.FadeDuration != nil {
			duration = *overlay.FadeDuration
		}
		// Fade in at start, fade out at end
		fadeIn := fmt.Sprintf("fade=t=in:st=%.2f:d=%.2f", overlay.StartTime, duration)
		fadeOut := fmt.Sprintf("fade=t=out:st=%.2f:d=%.2f", overlay.EndTime-duration, duration)
		imageFilter += fmt.Sprintf("%s,%s", fadeIn, fadeOut)

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
		// Calculate zoom rate
		zoomRate := (zoomTo - zoomFrom) / duration
		imageFilter += fmt.Sprintf("zoompan=z='if(lte(zoom,%.2f),zoom+%.6f,%.2f)':d=1:s=1280x720",
			zoomTo, zoomRate, zoomTo)

	case models.AnimationSlide:
		// Slide animation will be handled in overlay expression
		// Just pass through the image
		break

	case models.AnimationNone:
		// No animation
		break
	}

	filters = append(filters, imageFilter+"[overlay]")

	// Build overlay filter with position
	overlayExpr := buildOverlayExpression(overlay)
	filters = append(filters, fmt.Sprintf("[0:v][overlay]%s", overlayExpr))

	return strings.Join(filters, ";")
}

// buildOverlayExpression builds the overlay expression with position and timing
func buildOverlayExpression(overlay models.ImageOverlay) string {
	x, y := calculatePosition(overlay)

	// Handle slide animation in overlay expression
	if overlay.Animation == models.AnimationSlide && overlay.SlideDirection != nil {
		duration := 1.0
		if overlay.SlideDuration != nil {
			duration = *overlay.SlideDuration
		}
		x, y = calculateSlidePosition(overlay, x, y, duration)
	}

	// Build enable expression for timing
	enableExpr := fmt.Sprintf("enable='between(t,%.2f,%.2f)'", overlay.StartTime, overlay.EndTime)

	return fmt.Sprintf("overlay=%s:%s:%s", x, y, enableExpr)
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

	// Build FFmpeg command
	args := []string{"-y", "-i", videoPath}

	// Add all overlay images as inputs
	for _, overlay := range overlays {
		args = append(args, "-i", overlay.FilePath)
	}

	// Build complex filter for multiple overlays
	filterComplex := buildMultipleOverlaysFilter(overlays)
	args = append(args, "-filter_complex", filterComplex)
	args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-c:a", "copy", outputPath)

	return e.Execute(ctx, args)
}

// buildMultipleOverlaysFilter builds filter for multiple overlays (simplified version)
func buildMultipleOverlaysFilter(overlays []models.ImageOverlay) string {
	filters := make([]string, 0, len(overlays)*2)

	// Process each overlay
	for i, overlay := range overlays {
		overlayFilter := buildOverlayFilterForIndex(overlay, i)
		filters = append(filters, overlayFilter)
	}

	// Chain overlays
	currentInput := "[0:v]"
	for i := range overlays {
		outputLabel := fmt.Sprintf("[v%d]", i)
		if i == len(overlays)-1 {
			outputLabel = "[outv]"
		}
		overlayLabel := fmt.Sprintf("[overlay%d]", i)

		x, y := calculatePosition(overlays[i])
		enableExpr := fmt.Sprintf("enable='between(t,%.2f,%.2f)'", overlays[i].StartTime, overlays[i].EndTime)

		chainFilter := fmt.Sprintf("%s%soverlay=%s:%s:%s%s",
			currentInput, overlayLabel, x, y, enableExpr, outputLabel)
		filters = append(filters, chainFilter)

		currentInput = outputLabel
	}

	return strings.Join(filters, ";")
}

// buildOverlayFilterForIndex builds filter for a specific overlay index
func buildOverlayFilterForIndex(overlay models.ImageOverlay, index int) string {
	imageFilter := fmt.Sprintf("[%d:v]", index+1) // +1 because 0 is the video

	// Add simple fade animation if specified
	if overlay.Animation == models.AnimationFade && overlay.FadeDuration != nil {
		duration := *overlay.FadeDuration
		fadeIn := fmt.Sprintf("fade=t=in:st=%.2f:d=%.2f", overlay.StartTime, duration)
		fadeOut := fmt.Sprintf("fade=t=out:st=%.2f:d=%.2f", overlay.EndTime-duration, duration)
		imageFilter += fmt.Sprintf("%s,%s", fadeIn, fadeOut)
	}

	return fmt.Sprintf("%s[overlay%d]", imageFilter, index)
}
