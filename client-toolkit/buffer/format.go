package buffer

import "fmt"

// Format represents a Wayland SHM pixel format.
type Format uint32

// Wayland SHM format constants
const (
	FormatXRGB8888 Format = 0x34325258 // DRM_FORMAT_XRGB8888
	FormatARGB8888 Format = 0x34324758 // DRM_FORMAT_ARGB8888
	FormatRGB888   Format = 0x34324752 // DRM_FORMAT_RGB888
	FormatBGR888   Format = 0x36314752 // DRM_FORMAT_BGR888
	FormatRGB565   Format = 0x36314758 // DRM_FORMAT_RGB565
	FormatBGR565   Format = 0x36316758 // DRM_FORMAT_BGR565
)

// String returns the format name.
func (f Format) String() string {
	switch f {
	case FormatXRGB8888:
		return "XRGB8888"
	case FormatARGB8888:
		return "ARGB8888"
	case FormatRGB888:
		return "RGB888"
	case FormatBGR888:
		return "BGR888"
	case FormatRGB565:
		return "RGB565"
	case FormatBGR565:
		return "BGR565"
	default:
		return fmt.Sprintf("Format(%d)", uint32(f))
	}
}

// BytesPerPixel returns the number of bytes per pixel for this format.
func (f Format) BytesPerPixel() int {
	switch f {
	case FormatXRGB8888, FormatARGB8888, FormatRGB888, FormatBGR888:
		return 4
	case FormatRGB565, FormatBGR565:
		return 2
	default:
		return 4 // default
	}
}
