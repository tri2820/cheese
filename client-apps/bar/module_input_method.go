package main

import (
	"log"
	"os/exec"
	"strings"
	"time"
)

type InputMethodModule struct {
	TextModule
	stop      chan struct{}
	markDirty func()
}

func NewInputMethodModule() *InputMethodModule {
	return &InputMethodModule{
		stop: make(chan struct{}),
	}
}

func (m *InputMethodModule) Start(markDirty func()) {
	m.markDirty = markDirty
	if m.refreshAndMarkDirty() {
		return
	}

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		timeout := time.NewTimer(15 * time.Second)
		defer timeout.Stop()

		for {
			select {
			case <-ticker.C:
				if m.refreshAndMarkDirty() {
					return
				}
			case <-timeout.C:
				return
			case <-m.stop:
				return
			}
		}
	}()
}

func (m *InputMethodModule) Close() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
}

func (m *InputMethodModule) OnClick(button uint32) {
	if button != 0x110 {
		return
	}

	cmd := exec.Command("fcitx5-remote", "-t")
	if err := cmd.Run(); err != nil {
		log.Printf("Input method toggle error: %v", err)
		return
	}

	m.refreshAndMarkDirty()
}

func (m *InputMethodModule) HandleCommand(cmd string) bool {
	if cmd != "im-refresh" {
		return false
	}

	m.refreshAndMarkDirty()

	return true
}

func (m *InputMethodModule) refreshAndMarkDirty() bool {
	before := m.Text()
	ok := m.refresh()
	if ok && m.markDirty != nil && m.Text() != before {
		m.markDirty()
	}
	return ok
}

func (m *InputMethodModule) refresh() bool {
	cmd := exec.Command("qdbus", "org.fcitx.Fcitx5", "/controller", "org.fcitx.Fcitx.Controller1.CurrentInputMethod")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	name := strings.TrimSpace(string(output))
	if name == "" {
		return false
	}

	if idx := strings.LastIndex(name, "."); idx >= 0 && idx < len(name)-1 {
		name = name[idx+1:]
	}

	m.SetText(name)
	return true
}
