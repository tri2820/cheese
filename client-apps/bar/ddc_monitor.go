package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tri2820/cheese/client-toolkit/display"
)

const (
	i2cSlaveIoctl = 0x0703
	ddcSlaveAddr  = 0x37
	vcpPowerMode  = 0xD6
	ddcPollPeriod = 2 * time.Second
	ddcFailThresh = 2
)

type ddcOutputState struct {
	initialized   bool
	everSucceeded bool
	failCount     int
	poweredOff    bool
}

type DDCMonitor struct {
	disp     *display.Display
	onChange func()
	mu       sync.Mutex
	states   map[string]*ddcOutputState
	stop     chan struct{}
	closed   bool
}

func NewDDCMonitor(disp *display.Display, onChange func()) *DDCMonitor {
	m := &DDCMonitor{
		disp:     disp,
		onChange: onChange,
		states:   make(map[string]*ddcOutputState),
		stop:     make(chan struct{}),
	}
	go m.loop()
	return m
}

func (m *DDCMonitor) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	stop := m.stop
	m.mu.Unlock()
	close(stop)
}

func (m *DDCMonitor) IsPoweredOff(outputName string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.states[outputName]
	if !ok {
		return false
	}
	return st.poweredOff
}

func (m *DDCMonitor) loop() {
	ticker := time.NewTicker(ddcPollPeriod)
	defer ticker.Stop()

	m.pollOnce()
	for {
		select {
		case <-ticker.C:
			m.pollOnce()
		case <-m.stop:
			return
		}
	}
}

// ddcOutput is a connected external connector that speaks DDC/CI.
type ddcOutput struct {
	Name    string // niri output name, e.g. "HDMI-A-1"
	DevPath string // /dev/i2c-N serving the connector
	ConnDir string // /sys/class/drm/cardX-<output>
}

func discoverConnectedExternalOutputs() []ddcOutput {
	result := make([]ddcOutput, 0, 2)
	matches, err := filepath.Glob("/sys/class/drm/card*-*/ddc")
	if err != nil {
		return result
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i] < matches[j] })
	for _, ddcLink := range matches {
		connDir := filepath.Dir(ddcLink)
		base := filepath.Base(connDir) // e.g. "card1-HDMI-A-1"
		idx := strings.IndexByte(base, '-')
		if idx < 0 || idx+1 >= len(base) {
			continue
		}
		outName := base[idx+1:]
		if strings.HasPrefix(outName, "eDP") {
			continue
		}
		statusBytes, err := os.ReadFile(filepath.Join(connDir, "status"))
		if err != nil || strings.TrimSpace(string(statusBytes)) != "connected" {
			continue
		}
		target, err := os.Readlink(ddcLink)
		if err != nil {
			continue
		}
		busName := filepath.Base(target)
		if !strings.HasPrefix(busName, "i2c-") {
			continue
		}
		devPath := "/dev/" + busName
		if _, err := os.Stat(devPath); err != nil {
			continue
		}
		result = append(result, ddcOutput{Name: outName, DevPath: devPath, ConnDir: connDir})
	}
	return result
}

func (m *DDCMonitor) pollOnce() {
	extOutputs := discoverConnectedExternalOutputs()
	changed := false

	type niriToggle struct {
		name string
		off  bool
	}
	var toggles []niriToggle

	for _, out := range extOutputs {
		outName, devPath := out.Name, out.DevPath
		powerMode, err := queryDDCPowerMode(devPath)

		m.mu.Lock()
		st, ok := m.states[outName]
		if !ok {
			st = &ddcOutputState{}
			m.states[outName] = st
		}
		wasInit := st.initialized
		prevOff := st.poweredOff

		if err == nil {
			st.initialized = true
			st.everSucceeded = true
			st.failCount = 0
			// MCCS VCP 0xD6: 0x01 = DPM On; 0x02..0x05 = Standby/Suspend/Off
			st.poweredOff = powerMode != 0x01
		} else if st.everSucceeded {
			st.failCount++
			if st.failCount >= ddcFailThresh {
				st.poweredOff = true
			}
		}

		if (st.initialized && !wasInit && st.poweredOff) || (wasInit && st.poweredOff != prevOff) {
			log.Printf("DDCMonitor: output %s power state changed (poweredOff=%v, mode=0x%02x, err=%v)", outName, st.poweredOff, powerMode, err)
			changed = true
			toggles = append(toggles, niriToggle{name: outName, off: st.poweredOff})
		}
		m.mu.Unlock()
	}

	for _, t := range toggles {
		stateArg := "on"
		if t.off {
			stateArg = "off"
		}
		if err := exec.Command("niri", "msg", "output", t.name, stateArg).Run(); err != nil {
			log.Printf("DDCMonitor: failed to run niri msg output %s %s: %v", t.name, stateArg, err)
		} else {
			log.Printf("DDCMonitor: set niri output %s %s", t.name, stateArg)
		}
	}

	if changed && m.onChange != nil {
		m.onChange()
	}
}

func queryDDCPowerMode(devPath string) (byte, error) {
	f, err := os.OpenFile(devPath, os.O_RDWR, 0)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), i2cSlaveIoctl, ddcSlaveAddr)
	if errno != 0 {
		return 0, errno
	}

	req := []byte{0x51, 0x82, 0x01, vcpPowerMode, 0}
	chk := byte(0x6E)
	for i := 0; i < 4; i++ {
		chk ^= req[i]
	}
	req[4] = chk

	if _, err := f.Write(req); err != nil {
		return 0, err
	}
	time.Sleep(45 * time.Millisecond)

	resp := make([]byte, 12)
	n, err := f.Read(resp)
	if err != nil {
		return 0, err
	}
	if n < 10 {
		return 0, fmt.Errorf("short DDC reply (%d bytes)", n)
	}
	// Expected reply: [0x6E, 0x88, 0x02, result_code, vcp_code(0xD6), type, mh, ml, sh, sl, chk]
	if resp[2] != 0x02 || resp[4] != vcpPowerMode {
		return 0, fmt.Errorf("unexpected DDC reply header: % x", resp[:n])
	}
	return resp[9], nil
}
