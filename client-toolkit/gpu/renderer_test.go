package gpu

import (
	"errors"
	"reflect"
	"testing"
	"unsafe"

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

func TestRendererCreateGenerationMismatchDestroysReturnedSet(t *testing.T) {
	var destroyCalls int
	var gotErr error

	r := &Renderer{
		target:      stubTarget{width: 640, height: 480},
		bufferCount: 2,
		onCreateBuffers: func(width, height, count int) (*BufferSet, error) {
			return &BufferSet{
				Infos: []dmabuf.BufferInfo{{Fd: 1}},
				Destroy: func() {
					destroyCalls++
				},
			}, nil
		},
	}
	r.OnError(func(err error) {
		gotErr = err
	})

	r.handleConfigure()

	if destroyCalls != 1 {
		t.Fatalf("returned buffer set should be destroyed on mismatch, got %d destroy calls", destroyCalls)
	}
	if gotErr == nil {
		t.Fatalf("expected mismatch to emit an error")
	}
	if r.ready {
		t.Fatalf("renderer should not be ready after configure mismatch")
	}
}

func TestRendererCreateGenerationFailureDestroysPartialResources(t *testing.T) {
	sentinel := errors.New("boom")
	var destroySetCalls int
	var destroyedBuffers int
	var createCalls int
	var gotErr error

	r := &Renderer{
		target:      stubTarget{width: 800, height: 600},
		bufferCount: 2,
		onCreateBuffers: func(width, height, count int) (*BufferSet, error) {
			return &BufferSet{
				Infos: []dmabuf.BufferInfo{
					{Fd: 1, Format: dmabuf.FormatXRGB8888},
					{Fd: 2, Format: dmabuf.FormatXRGB8888},
				},
				Destroy: func() {
					destroySetCalls++
				},
			}, nil
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
	if destroySetCalls != 1 {
		t.Fatalf("buffer set destroy should fire once after partial creation failure, got %d", destroySetCalls)
	}
	if destroyedBuffers != 1 {
		t.Fatalf("expected one partially created wl_buffer to be destroyed, got %d", destroyedBuffers)
	}
	if r.active != nil {
		t.Fatalf("renderer should not keep a failed generation active")
	}
	if r.ready {
		t.Fatalf("renderer should not be ready after partial creation failure")
	}
}

func TestRendererResizeRetiresBusyGenerationAndCleansItOnRelease(t *testing.T) {
	var destroyCalls []string
	var createCalls int

	makeSet := func(label string) *BufferSet {
		return &BufferSet{
			Infos: []dmabuf.BufferInfo{{Fd: 1, Format: dmabuf.FormatXRGB8888}},
			Destroy: func() {
				destroyCalls = append(destroyCalls, label)
			},
		}
	}

	current := &dmabuf.Buffer{}
	setBufferBusy(t, current, true)

	active := &generation{
		width:   800,
		height:  600,
		set:     makeSet("old"),
		buffers: []*dmabuf.Buffer{current},
	}

	r := &Renderer{
		target:      stubTarget{width: 1024, height: 768},
		bufferCount: 1,
		active:      active,
		lastWidth:   800,
		lastHeight:  600,
		ready:       true,
		onCreateBuffers: func(width, height, count int) (*BufferSet, error) {
			createCalls++
			return makeSet("new"), nil
		},
		onRender: func(bufferIndex, width, height int, time uint32) error {
			return nil
		},
	}
	r.createBufferFn = func(width, height int, info dmabuf.BufferInfo) (*dmabuf.Buffer, error) {
		return &dmabuf.Buffer{}, nil
	}
	r.destroyBufferFn = func(*dmabuf.Buffer) error {
		return nil
	}

	r.handleConfigure()

	if createCalls != 1 {
		t.Fatalf("resize should create a new generation once, got %d", createCalls)
	}
	if r.active == nil || r.active.width != 1024 || r.active.height != 768 {
		t.Fatalf("new generation should become active after resize")
	}
	if len(r.retired) != 1 {
		t.Fatalf("old busy generation should be retired, got %d retired generations", len(r.retired))
	}
	if len(destroyCalls) != 0 {
		t.Fatalf("busy retired generation should not be destroyed yet, got %v", destroyCalls)
	}

	setBufferBusy(t, current, false)
	r.handleBufferRelease(active, 0)

	if len(r.retired) != 0 {
		t.Fatalf("released retired generation should be collected")
	}
	if len(destroyCalls) != 1 || destroyCalls[0] != "old" {
		t.Fatalf("expected old generation cleanup after release, got %v", destroyCalls)
	}
}

func setBufferBusy(t *testing.T, buf *dmabuf.Buffer, busy bool) {
	t.Helper()

	field := reflect.ValueOf(buf).Elem().FieldByName("busy")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().SetBool(busy)
}
