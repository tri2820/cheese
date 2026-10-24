package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	vcpBrightness      = 0x10
	brightnessPoll     = 5 * time.Second
	brightnessWriteGap = 40 * time.Millisecond
	brightnessMinPct   = 0
	ddcResponseWait    = 50 * time.Millisecond
)

// BrightnessDisplay is one monitor's brightness control: the internal panel
// (sysfs backlight via logind) or an external monitor (DDC/CI over i2c).
type BrightnessDisplay struct {
	Key       string // stable id: niri output name, or backlight device name
	Label     string // human label ("Built-in Display", EDID model name)
	Internal  bool
	Available bool
	Percent   int

	rawMax  int
	sysPath string // backlight directory
	busPath string // /dev/i2c-N for external monitors
}

// BrightnessService owns every monitor's brightness. All I/O (logind D-Bus,
// i2c/DDC transactions, sysfs reads) runs on its own goroutines; callers only
// ever touch cached state, so the Wayland main thread never blocks.
type BrightnessService struct {
	mu        sync.Mutex
	displays  []BrightnessDisplay
	pending   map[string]int
	lastSet   map[string]time.Time
	subs      map[int]func([]BrightnessDisplay)
	nextSubID int
	closed    bool
	wake      chan struct{}
	stop      chan struct{}

	ddcMu  sync.Mutex // serializes i2c transactions (one DDC conversation per bus at a time)
	conn   *dbus.Conn // lazily opened system bus for logind SetBrightness
	connMu sync.Mutex
}

func NewBrightnessService() *BrightnessService {
	s := &BrightnessService{
		pending: make(map[string]int),
		lastSet: make(map[string]time.Time),
		subs:    make(map[int]func([]BrightnessDisplay)),
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
	}
	// Initial discovery runs on the service goroutine, never in the constructor.
	go s.pollLoop()
	go s.writeLoop()
	return s
}

func (s *BrightnessService) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	stop := s.stop
	s.subs = make(map[int]func([]BrightnessDisplay))
	s.mu.Unlock()

	close(stop)

	s.connMu.Lock()
	conn := s.conn
	s.conn = nil
	s.connMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// Displays returns a copy of the cached monitor list (non-blocking).
func (s *BrightnessService) Displays() []BrightnessDisplay {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]BrightnessDisplay, len(s.displays))
	copy(out, s.displays)
	return out
}

// Main returns the primary brightness target: the built-in panel when present,
// otherwise the first external monitor.
func (s *BrightnessService) Main() (BrightnessDisplay, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.displays {
		if d.Internal {
			return d, true
		}
	}
	if len(s.displays) > 0 {
		return s.displays[0], true
	}
	return BrightnessDisplay{}, false
}

// Set requests a brightness percentage for one monitor. The cached value is
// updated immediately (so the UI follows the pointer) and the actual write is
// coalesced and performed off-thread.
func (s *BrightnessService) Set(key string, percent int) {
	if percent < brightnessMinPct {
		percent = brightnessMinPct
	}
	if percent > 100 {
		percent = 100
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	found := false
	for i := range s.displays {
		if s.displays[i].Key == key {
			s.displays[i].Percent = percent
			found = true
			break
		}
	}
	if !found {
		s.mu.Unlock()
		return
	}
	s.pending[key] = percent
	s.lastSet[key] = time.Now()
	snapshot := make([]BrightnessDisplay, len(s.displays))
	copy(snapshot, s.displays)
	subs := s.subscribersLocked()
	s.mu.Unlock()

	for _, fn := range subs {
		fn(snapshot)
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Subscribe registers a change callback and returns an unsubscribe func.
func (s *BrightnessService) Subscribe(fn func([]BrightnessDisplay)) func() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return func() {}
	}
	s.nextSubID++
	id := s.nextSubID
	s.subs[id] = fn
	s.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subs, id)
			s.mu.Unlock()
		})
	}
}

func (s *BrightnessService) subscribersLocked() []func([]BrightnessDisplay) {
	out := make([]func([]BrightnessDisplay), 0, len(s.subs))
	for _, fn := range s.subs {
		out = append(out, fn)
	}
	return out
}

func (s *BrightnessService) pollLoop() {
	s.refresh()
	ticker := time.NewTicker(brightnessPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.refresh()
		case <-s.stop:
			return
		}
	}
}

func (s *BrightnessService) refresh() {
	displays := s.discover()
	if displays == nil {
		return
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	// A failed read (DDC flakiness on wake, backlight race) must not zero the
	// slider: keep the last known level and mark it available again.
	for i := range displays {
		if displays[i].Available {
			continue
		}
		for _, p := range s.displays {
			if p.Key != displays[i].Key || !p.Available {
				continue
			}
			displays[i].Available = true
			displays[i].Percent = p.Percent
			if displays[i].rawMax <= 0 {
				displays[i].rawMax = p.rawMax
			}
			break
		}
	}
	// Keep optimistic values for writes that are still in flight (or landed
	// within the last second) so a poll never yanks the slider backwards.
	for i := range displays {
		if pct, ok := s.pending[displays[i].Key]; ok {
			displays[i].Percent = pct
			continue
		}
		if ts, ok := s.lastSet[displays[i].Key]; ok && time.Since(ts) < time.Second {
			for _, d := range s.displays {
				if d.Key == displays[i].Key {
					displays[i].Percent = d.Percent
					break
				}
			}
		}
	}
	changed := !sameDisplayState(s.displays, displays)
	s.displays = displays
	snapshot := make([]BrightnessDisplay, len(displays))
	copy(snapshot, displays)
	subs := s.subscribersLocked()
	s.mu.Unlock()

	if !changed {
		return
	}
	for _, fn := range subs {
		fn(snapshot)
	}
}

func sameDisplayState(a, b []BrightnessDisplay) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Percent != b[i].Percent || a[i].Available != b[i].Available || a[i].Label != b[i].Label {
			return false
		}
	}
	return true
}

// discover enumerates every brightness-capable monitor and reads its level.
// Runs off the main thread (sysfs + DDC reads).
func (s *BrightnessService) discover() []BrightnessDisplay {
	var out []BrightnessDisplay

	if dirs, err := filepath.Glob("/sys/class/backlight/*"); err == nil {
		sort.Strings(dirs)
		for _, dir := range dirs {
			maxRaw, err := readIntFile(filepath.Join(dir, "max_brightness"))
			if err != nil || maxRaw <= 0 {
				continue
			}
			name := filepath.Base(dir)
			d := BrightnessDisplay{
				Key:       name,
				Label:     "Built-in Display",
				Internal:  true,
				Available: true,
				rawMax:    maxRaw,
				sysPath:   dir,
			}
			if cur, err := readIntFile(filepath.Join(dir, "brightness")); err == nil {
				d.Percent = scaleToPercent(cur, maxRaw)
			}
			out = append(out, d)
		}
	}

	for _, o := range discoverConnectedExternalOutputs() {
		d := BrightnessDisplay{
			Key:     o.Name,
			Label:   edidDisplayLabel(o.ConnDir),
			busPath: o.DevPath,
		}
		if d.Label == "" {
			d.Label = o.Name
		}
		cur, maxRaw, err := s.readDDCBrightness(o.DevPath)
		if err == nil && maxRaw > 0 {
			d.Available = true
			d.rawMax = maxRaw
			d.Percent = scaleToPercent(cur, maxRaw)
		}
		out = append(out, d)
	}

	return out
}

func (s *BrightnessService) writeLoop() {
	for {
		select {
		case <-s.wake:
		case <-s.stop:
			return
		}

		for {
			s.mu.Lock()
			if len(s.pending) == 0 || s.closed {
				s.mu.Unlock()
				break
			}
			keys := make([]string, 0, len(s.pending))
			for k := range s.pending {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			key := keys[0]
			pct := s.pending[key]
			delete(s.pending, key)
			var target BrightnessDisplay
			var ok bool
			for _, d := range s.displays {
				if d.Key == key {
					target, ok = d, true
					break
				}
			}
			s.mu.Unlock()

			if !ok {
				continue
			}
			s.write(target, pct)
			time.Sleep(brightnessWriteGap)
		}
	}
}

func (s *BrightnessService) write(d BrightnessDisplay, percent int) {
	if d.Internal {
		s.writeBacklight(d, percent)
		return
	}
	if err := s.writeDDCBrightness(d, percent); err != nil {
		log.Printf("BrightnessService: DDC write failed on %s: %v", d.Key, err)
	}
}

func (s *BrightnessService) writeBacklight(d BrightnessDisplay, percent int) {
	raw := uint32(math.Round(float64(percent) / 100.0 * float64(d.rawMax)))
	if raw > uint32(d.rawMax) {
		raw = uint32(d.rawMax)
	}
	if err := s.logindSetBrightness(d.Key, raw); err == nil {
		return
	} else {
		log.Printf("BrightnessService: logind SetBrightness failed (%v), falling back to brightnessctl", err)
	}
	_ = exec.Command("brightnessctl", "-d", d.Key, "set", strconv.FormatUint(uint64(raw), 10)).Run()
}

func (s *BrightnessService) logindSetBrightness(name string, raw uint32) error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if s.conn == nil {
		conn, err := dbus.SystemBus()
		if err != nil {
			return err
		}
		s.conn = conn
	}
	obj := s.conn.Object("org.freedesktop.login1", dbus.ObjectPath("/org/freedesktop/login1/session/auto"))
	call := obj.Call("org.freedesktop.login1.Session.SetBrightness", 0, "backlight", name, raw)
	if call.Err != nil {
		// Reconnect on the next attempt: the cached connection may be stale.
		_ = s.conn.Close()
		s.conn = nil
		return call.Err
	}
	return nil
}

// ---- DDC/CI ----

func (s *BrightnessService) readDDCBrightness(devPath string) (cur, maxRaw int, err error) {
	s.ddcMu.Lock()
	defer s.ddcMu.Unlock()

	f, err := openDDCBus(devPath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	req := []byte{0x51, 0x82, 0x01, vcpBrightness, 0}
	chk := byte(0x6E)
	for i := 0; i < 4; i++ {
		chk ^= req[i]
	}
	req[4] = chk
	if _, err := f.Write(req); err != nil {
		return 0, 0, err
	}
	time.Sleep(ddcResponseWait)

	resp := make([]byte, 12)
	n, err := f.Read(resp)
	if err != nil {
		return 0, 0, err
	}
	if n < 11 || resp[2] != 0x02 || resp[4] != vcpBrightness {
		return 0, 0, fmt.Errorf("unexpected DDC reply: % x", resp[:n])
	}
	if resp[3] != 0 {
		return 0, 0, fmt.Errorf("DDC result code 0x%02x", resp[3])
	}
	maxRaw = int(resp[6])<<8 | int(resp[7])
	cur = int(resp[8])<<8 | int(resp[9])
	if maxRaw <= 0 {
		return 0, 0, fmt.Errorf("DDC reported max %d", maxRaw)
	}
	return cur, maxRaw, nil
}

func (s *BrightnessService) writeDDCBrightness(d BrightnessDisplay, percent int) error {
	s.ddcMu.Lock()
	defer s.ddcMu.Unlock()

	maxRaw := d.rawMax
	if maxRaw <= 0 {
		maxRaw = 100
	}
	val := int(math.Round(float64(percent) / 100.0 * float64(maxRaw)))
	if val > maxRaw {
		val = maxRaw
	}

	f, err := openDDCBus(d.busPath)
	if err != nil {
		return err
	}
	defer f.Close()

	req := []byte{0x51, 0x84, 0x03, vcpBrightness, byte(val >> 8), byte(val), 0}
	chk := byte(0x6E)
	for i := 0; i < 6; i++ {
		chk ^= req[i]
	}
	req[6] = chk
	if _, err := f.Write(req); err != nil {
		return err
	}
	// Consume the (optional) acknowledgement so the bus isn't left mid-reply.
	time.Sleep(ddcResponseWait)
	resp := make([]byte, 12)
	_, _ = f.Read(resp)
	return nil
}

func openDDCBus(devPath string) (*os.File, error) {
	f, err := os.OpenFile(devPath, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), i2cSlaveIoctl, ddcSlaveAddr); errno != 0 {
		_ = f.Close()
		return nil, errno
	}
	return f, nil
}

// ---- helpers ----

func readIntFile(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

func scaleToPercent(raw, maxRaw int) int {
	if maxRaw <= 0 {
		return 0
	}
	pct := int(math.Round(float64(raw) / float64(maxRaw) * 100.0))
	if pct < brightnessMinPct {
		pct = brightnessMinPct
	}
	if pct > 100 {
		pct = 100
	}
	return pct
}

// edidDisplayLabel builds a "MAKE MODEL" label ("AOC 24B2W1G5") from the EDID
// blob, falling back to "" when the EDID is unreadable.
func edidDisplayLabel(connDir string) string {
	makeName, model := edidNames(connDir)
	switch {
	case makeName != "" && model != "":
		if strings.HasPrefix(model, makeName) {
			return model
		}
		return makeName + " " + model
	case model != "":
		return model
	default:
		return makeName
	}
}

// edidNames returns the manufacturer name (from the PNP ID) and the monitor
// name descriptor (0xFC) of the display on a connector.
func edidNames(connDir string) (makeName, model string) {
	b, err := os.ReadFile(filepath.Join(connDir, "edid"))
	if err != nil || len(b) < 128 {
		return "", ""
	}

	if v := uint16(b[8])<<8 | uint16(b[9]); v != 0 {
		letters := [3]byte{
			byte((v>>10)&0x1f) + 'A' - 1,
			byte((v>>5)&0x1f) + 'A' - 1,
			byte(v&0x1f) + 'A' - 1,
		}
		makeName = strings.TrimSpace(string(letters[:]))
	}

	for _, off := range []int{54, 72, 90, 108} {
		d := b[off : off+18]
		if d[0] != 0 || d[1] != 0 || d[2] != 0 || d[3] != 0xFC {
			continue
		}
		model = strings.TrimSpace(strings.Trim(string(d[5:18]), "\x00\n\r "))
		break
	}
	return makeName, model
}
