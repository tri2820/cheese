package font

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// LoadFont loads a TTF font by name using fc-match.
// The fontName is a pattern like "Arial", "DejaVu Sans", etc.
// Size is in points, DPI is dots per inch (use 96 for standard, or output.DPI() for monitor-specific).
func LoadFont(fontName string, size float64, dpi float64) (font.Face, error) {
	// Get both path and family name to verify the match
	cmd := exec.Command("fc-match", "-f", "%{file}|%{family}", fontName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fc-match failed: %w", err)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) != 2 {
		return nil, fmt.Errorf("fc-match returned unexpected format")
	}

	fontPath := parts[0]
	matchedFamily := parts[1]

	if fontPath == "" {
		return nil, fmt.Errorf("fc-match returned empty path")
	}

	// Verify the matched font actually contains the requested name
	fontNameLower := strings.ToLower(fontName)
	matchedFamilyLower := strings.ToLower(matchedFamily)
	if !strings.Contains(matchedFamilyLower, fontNameLower) {
		return nil, fmt.Errorf("font '%s' not found (got '%s' instead)", fontName, matchedFamily)
	}

	// Read font file
	fontBytes, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read font file: %w", err)
	}

	// Parse the font
	f, err := opentype.Parse(fontBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse font file: %w", err)
	}

	// Create font face at specified size with hinting
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
}
