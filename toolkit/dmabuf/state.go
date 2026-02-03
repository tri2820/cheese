package dmabuf

import (
	"github.com/tri2820/cheese/protocols/linux_dmabuf_unstable_v1"
)

// State manages the linux-dmabuf protocol state.
// It tracks supported formats/modifiers and provides methods to create buffers.
type State struct {
	dmabuf  *linux_dmabuf_unstable_v1.ZwpLinuxDmabufV1
	version uint32

	// Formats advertised via modifier events (protocol v3+)
	formats []FormatModifier
}

// NewState creates a new DMA-BUF state from a bound global.
func NewState(dmabuf *linux_dmabuf_unstable_v1.ZwpLinuxDmabufV1, version uint32) *State {
	s := &State{
		dmabuf:  dmabuf,
		version: version,
		formats: make([]FormatModifier, 0),
	}

	// Set up format/modifier handlers for v3
	dmabuf.SetModifierHandler(s.handleModifier)

	return s
}

// handleModifier handles modifier events from the compositor.
func (s *State) handleModifier(ev linux_dmabuf_unstable_v1.ZwpLinuxDmabufV1ModifierEvent) {
	modifier := uint64(ev.ModifierHi)<<32 | uint64(ev.ModifierLo)
	s.formats = append(s.formats, FormatModifier{
		Format:   Format(ev.Format),
		Modifier: Modifier(modifier),
	})
}

// Formats returns all supported format/modifier pairs.
func (s *State) Formats() []FormatModifier {
	return s.formats
}

// SupportsFormat returns true if the given format is supported.
func (s *State) SupportsFormat(format Format) bool {
	for _, fm := range s.formats {
		if fm.Format == format {
			return true
		}
	}
	return false
}

// SupportsFormatModifier returns true if the given format+modifier pair is supported.
func (s *State) SupportsFormatModifier(format Format, modifier Modifier) bool {
	for _, fm := range s.formats {
		if fm.Format == format && fm.Modifier == modifier {
			return true
		}
	}
	return false
}

// CreateParams creates a new buffer params builder.
// Use the returned Params to add planes and create a buffer.
func (s *State) CreateParams() (*Params, error) {
	params, err := s.dmabuf.CreateParams()
	if err != nil {
		return nil, err
	}
	return &Params{params: params}, nil
}

// Version returns the protocol version.
func (s *State) Version() uint32 {
	return s.version
}

// WlDmabuf returns the underlying protocol object.
func (s *State) WlDmabuf() *linux_dmabuf_unstable_v1.ZwpLinuxDmabufV1 {
	return s.dmabuf
}
