package main

import (
	"bufio"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/tri2820/cheese/client-toolkit/display"
)

type VolumeModule struct {
	TextModule
	output *display.Output
	stop   chan struct{}
	cmdMu  sync.Mutex
	cmd    *exec.Cmd
}

func NewVolumeModule(output *display.Output) *VolumeModule {
	return &VolumeModule{
		output: output,
		stop:   make(chan struct{}),
	}
}

func (m *VolumeModule) Start(markDirty func()) {
	m.refresh()

	go m.watch(markDirty)
}

func (m *VolumeModule) Close() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}

	m.cmdMu.Lock()
	defer m.cmdMu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_, _ = m.cmd.Process.Wait()
	}
}

func (m *VolumeModule) refresh() {
	cmd := exec.Command("wpctl", "get-volume", "@DEFAULT_AUDIO_SINK@")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 2 {
		return
	}

	level, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return
	}

	muted := strings.Contains(string(output), "MUTED")
	icon := volumeIcon(level, muted)
	text := icon
	if !muted {
		text = strconv.Itoa(int(level*100+0.5)) + "% " + icon
	}

	m.SetText(text)
}

func (m *VolumeModule) watch(markDirty func()) {
	cmd := exec.Command("pw-mon", "--hide-props", "--print-separator")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Audio monitor pipe error on %s: %v", m.output.Name, err)
		return
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		log.Printf("Audio monitor start error on %s: %v", m.output.Name, err)
		return
	}

	m.cmdMu.Lock()
	m.cmd = cmd
	m.cmdMu.Unlock()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-m.stop:
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.Contains(line, "added:") &&
			!strings.Contains(line, "removed:") &&
			!strings.Contains(line, "changed:") &&
			!strings.Contains(line, "metadata.name") &&
			!strings.Contains(line, "default.audio") &&
			!strings.Contains(line, "Audio/Sink") &&
			!strings.Contains(line, "Audio/Source") {
			continue
		}

		m.refresh()
		markDirty()
	}

	if err := scanner.Err(); err != nil {
		select {
		case <-m.stop:
		default:
			log.Printf("Audio monitor read error on %s: %v", m.output.Name, err)
		}
	}
}

func volumeIcon(level float64, muted bool) string {
	if muted {
		return "󰸈"
	}
	if level < 0.34 {
		return "󰕿"
	}
	if level < 0.67 {
		return "󰖀"
	}
	return "󰕾"
}
