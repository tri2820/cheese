package buffer

import "fmt"

// Format represents a Wayland SHM pixel format.
type Format uint32

// Wayland SHM format constants (fourcc codes from drm_fourcc.h)
const (
	// 32-bit formats (4 bytes per pixel)
	FormatXRGB8888 Format = 0x34325258 // XRGB8888 - memory: B,G,R,X (little-endian)
	FormatARGB8888 Format = 0x34325241 // ARGB8888 - memory: B,G,R,A (little-endian)
	FormatXBGR8888 Format = 0x34324258 // XBGR8888 - memory: R,G,B,X (little-endian)
	FormatABGR8888 Format = 0x34324241 // ABGR8888 - memory: R,G,B,A (little-endian) - matches Go's image.RGBA
	FormatRGBX8888 Format = 0x34325852 // RGBX8888 - memory: X,B,G,R (little-endian)
	FormatRGBA8888 Format = 0x34324152 // RGBA8888 - memory: A,B,G,R (little-endian)
	FormatBGRX8888 Format = 0x34325842 // BGRX8888 - memory: X,R,G,B (little-endian)
	FormatBGRA8888 Format = 0x34324142 // BGRA8888 - memory: A,R,G,B (little-endian)

	// 24-bit formats (3 bytes per pixel)
	FormatRGB888 Format = 0x34324752 // RGB888 - memory: R,G,B
	FormatBGR888 Format = 0x34324742 // BGR888 - memory: B,G,R

	// 16-bit formats (2 bytes per pixel)
	FormatRGB565 Format = 0x36314752 // RGB565
	FormatBGR565 Format = 0x36314742 // BGR565
)

// String returns the format name.
func (f Format) String() string {
	switch f {
	case FormatXRGB8888:
		return "XRGB8888"
	case FormatARGB8888:
		return "ARGB8888"
	case FormatXBGR8888:
		return "XBGR8888"
	case FormatABGR8888:
		return "ABGR8888"
	case FormatRGBX8888:
		return "RGBX8888"
	case FormatRGBA8888:
		return "RGBA8888"
	case FormatBGRX8888:
		return "BGRX8888"
	case FormatBGRA8888:
		return "BGRA8888"
	case FormatRGB888:
		return "RGB888"
	case FormatBGR888:
		return "BGR888"
	case FormatRGB565:
		return "RGB565"
	case FormatBGR565:
		return "BGR565"
	default:
		return fmt.Sprintf("Format(0x%08x)", uint32(f))
	}
}

// BytesPerPixel returns the number of bytes per pixel for this format.
func (f Format) BytesPerPixel() int {
	switch f {
	case FormatXRGB8888, FormatARGB8888, FormatXBGR8888, FormatABGR8888,
		FormatRGBX8888, FormatRGBA8888, FormatBGRX8888, FormatBGRA8888:
		return 4
	case FormatRGB888, FormatBGR888:
		return 3
	case FormatRGB565, FormatBGR565:
		return 2
	default:
		return 4
	}
}
