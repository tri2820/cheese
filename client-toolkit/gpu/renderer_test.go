package gpu

import (
	"errors"
	"testing"

	"github.com/tri2820/cheese/client-toolkit/dmabuf"
	"github.com/tri2820/cheese/client-toolkit/surface"
)

type stubTarget struct {
	width  int
	height int
}

func (t stubTarget) OnConfigure(func()) {}

func (t stubTarget) Surface() *surface.Surface {
	return nil
}

func (t stubTarget) Width() int {
	return t.width
}

func (t stubTarget) Height() int {
	return t.height
}

func TestRendererDestroyBuffersOnlyNotifiesWhenResourcesActive(t *testing.T) {
	var destroyCalls int

	r := &Renderer{
		onDestroyBuffers: func() {
			destroyCalls++
		},
	}

	r.destroyBuffers()
	if destroyCalls != 0 {
		t.Fatalf("destroy callback should not fire without active resources, got %d", destroyCalls)
	}

	r.resourcesActive = true
	r.ready = true
	r.destroyBuffers()

	if destroyCalls != 1 {
		t.Fatalf("destroy callback should fire once for active resources, got %d", destroyCalls)
	}
	if r.resourcesActive {
		t.Fatalf("resourcesActive should be cleared after destroyBuffers")
	}
	if r.ready {
		t.Fatalf("renderer should not remain ready after destroyBuffers")
	}

	r.destroyBuffers()
	if destroyCalls != 1 {
		t.Fatalf("destroy callback should remain idempotent, got %d", destroyCalls)
	}
}

func TestRendererHandleConfigureMismatchCleansUpResources(t *testing.T) {
	var destroyCalls int
	var gotErr error

	r := &Renderer{
		target:      stubTarget{width: 640, height: 480},
		bufferCount: 2,
		onCreateBuffers: func(width, height, count int) ([]dmabuf.BufferInfo, error) {
			return []dmabuf.BufferInfo{{Fd: 1}}, nil
		},
		onDestroyBuffers: func() {
			destroyCalls++
		},
	}
	r.OnError(func(err error) {
		gotErr = err
	})

	r.handleConfigure()

	if destroyCalls != 1 {
		t.Fatalf("destroy callback should fire for mismatched created resources, got %d", destroyCalls)
	}
	if gotErr == nil {
		t.Fatalf("expected mismatch to emit an error")
	}
	if r.ready {
		t.Fatalf("renderer should not be ready after configure mismatch")
	}
}

func TestRendererHandleConfigureCreateBufferFailureDestroysPartialResources(t *testing.T) {
	sentinel := errors.New("boom")
	var destroyCalls int
	var destroyedBuffers int
	var createCalls int
	var gotErr error

	r := &Renderer{
		target:      stubTarget{width: 800, height: 600},
		bufferCount: 2,
		onCreateBuffers: func(width, height, count int) ([]dmabuf.BufferInfo, error) {
			return []dmabuf.BufferInfo{
				{Fd: 1, Format: dmabuf.FormatXRGB8888},
				{Fd: 2, Format: dmabuf.FormatXRGB8888},
			}, nil
		},
		onDestroyBuffers: func() {
			destroyCalls++
		},
	}
	r.createBufferFn = func(width, height int, info dmabuf.BufferInfo) (*dmabuf.Buffer, error) {
		createCalls++
		if createCalls == 1 {
			return &dmabuf.Buffer{}, nil
		}
		return nil, sentinel
	}
	r.destroyBufferFn = func(*dmabuf.Buffer) error {
		destroyedBuffers++
		return nil
	}
	r.OnError(func(err error) {
		gotErr = err
	})

	r.handleConfigure()

	if !errors.Is(gotErr, sentinel) {
		t.Fatalf("expected create failure to be emitted, got %v", gotErr)
	}
	if destroyCalls != 1 {
		t.Fatalf("destroy callback should fire once after partial creation failure, got %d", destroyCalls)
	}
	if destroyedBuffers != 1 {
		t.Fatalf("expected one partially created wl_buffer to be destroyed, got %d", destroyedBuffers)
	}
	if len(r.buffers) != 0 {
		t.Fatalf("renderer buffers should be cleared after cleanup, got %d", len(r.buffers))
	}
	if r.ready {
		t.Fatalf("renderer should not be ready after partial creation failure")
	}
}
