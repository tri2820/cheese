package common

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// Global font cache
var (
	fontCache   = make(map[fontCacheKey]font.Face)
	fontCacheMu sync.RWMutex
)

type fontCacheKey struct {
	family string
	size   float64
	dpi    float64
}

// GetFont loads a font face, using a global cache to avoid repeated loading.
// The family is a pattern like "Liberation Sans", "Monospace", etc.
// Size is in points, DPI is dots per inch (use 96 for standard).
func GetFont(family string, size, dpi float64) (font.Face, error) {
	key := fontCacheKey{family: family, size: size, dpi: dpi}

	// Check cache first
	fontCacheMu.RLock()
	if face, ok := fontCache[key]; ok {
		fontCacheMu.RUnlock()
		return face, nil
	}
	fontCacheMu.RUnlock()

	// Load font
	face, err := loadFont(family, size, dpi)
	if err != nil {
		return nil, err
	}

	// Cache it
	fontCacheMu.Lock()
	fontCache[key] = face
	fontCacheMu.Unlock()

	return face, nil
}

// loadFont loads a TTF font by name using fc-match.
func loadFont(fontName string, size float64, dpi float64) (font.Face, error) {
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
