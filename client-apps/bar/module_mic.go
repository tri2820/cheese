package main

import (
	"bufio"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"

	"github.com/tri2820/cheese/client-toolkit/display"
)

type MicModule struct {
	TextModule
	output    *display.Output
	stop      chan struct{}
	cmdMu     sync.Mutex
	cmd       *exec.Cmd
	markDirty func()
}

func NewMicModule(output *display.Output) *MicModule {
	return &MicModule{
		output: output,
		stop:   make(chan struct{}),
	}
}

func (m *MicModule) Start(markDirty func()) {
	m.markDirty = markDirty
	m.refresh()
	go m.watch(markDirty)
}

func (m *MicModule) Close() {
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

func (m *MicModule) refresh() {
	cmd := exec.Command("wpctl", "get-volume", "@DEFAULT_AUDIO_SOURCE@")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	if strings.Contains(string(output), "MUTED") {
		m.SetText("󰍭")
		return
	}
	m.SetText("󰍬")
}

func (m *MicModule) OnClick(button uint32) {
	if button != 0x110 {
		return
	}

	cmd := exec.Command("wpctl", "set-mute", "@DEFAULT_AUDIO_SOURCE@", "toggle")
	if err := cmd.Run(); err != nil {
		log.Printf("Mic toggle error on %s: %v", m.output.Name, err)
		return
	}

	before := m.Text()
	m.refresh()
	if m.markDirty != nil && m.Text() != before {
		m.markDirty()
	}
}

func (m *MicModule) watch(markDirty func()) {
	cmd := exec.Command("pw-mon", "--hide-props", "--print-separator")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Mic monitor pipe error on %s: %v", m.output.Name, err)
		return
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		log.Printf("Mic monitor start error on %s: %v", m.output.Name, err)
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

		before := m.Text()
		m.refresh()
		if m.Text() != before {
			markDirty()
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case <-m.stop:
		default:
			log.Printf("Mic monitor read error on %s: %v", m.output.Name, err)
		}
	}
}
