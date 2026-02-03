package dmabuf

// Format represents a DRM pixel format (fourcc code).
type Format uint32

// Common DRM formats.
// These are the fourcc codes from drm_fourcc.h
const (
	FormatXRGB8888 Format = 0x34325258 // DRM_FORMAT_XRGB8888
	FormatARGB8888 Format = 0x34325241 // DRM_FORMAT_ARGB8888
	FormatXBGR8888 Format = 0x34324258 // DRM_FORMAT_XBGR8888
	FormatABGR8888 Format = 0x34324241 // DRM_FORMAT_ABGR8888
	FormatRGBX8888 Format = 0x34325852 // DRM_FORMAT_RGBX8888
	FormatRGBA8888 Format = 0x34324152 // DRM_FORMAT_RGBA8888
	FormatBGRX8888 Format = 0x34325842 // DRM_FORMAT_BGRX8888
	FormatBGRA8888 Format = 0x34324142 // DRM_FORMAT_BGRA8888
	FormatRGB888   Format = 0x34324752 // DRM_FORMAT_RGB888
	FormatBGR888   Format = 0x34324742 // DRM_FORMAT_BGR888
	FormatRGB565   Format = 0x36314752 // DRM_FORMAT_RGB565
	FormatBGR565   Format = 0x36314742 // DRM_FORMAT_BGR565
)

// Modifier represents a DRM format modifier.
type Modifier uint64

// Common DRM modifiers.
const (
	ModLinear  Modifier = 0                  // DRM_FORMAT_MOD_LINEAR
	ModInvalid Modifier = 0x00ffffffffffffff // DRM_FORMAT_MOD_INVALID
)

// FormatModifier represents a format + modifier pair.
type FormatModifier struct {
	Format   Format
	Modifier Modifier
}

// String returns a human-readable name for the format.
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
		return "unknown"
	}
}

// BytesPerPixel returns the bytes per pixel for this format.
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
