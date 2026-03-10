package dmabuf

// BufferInfo contains the DMA-BUF metadata for a single buffer.
type BufferInfo struct {
	Fd       int      // DMA-BUF file descriptor
	Stride   int      // Bytes per row
	Format   Format   // DRM format (e.g., FormatXRGB8888)
	Modifier Modifier // DRM modifier (e.g., ModLinear)
}
