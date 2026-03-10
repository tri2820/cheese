package main

import (
	"log"
	"os/exec"
	"strings"
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
	m.refresh()
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

	before := m.Text()
	m.refresh()
	if m.markDirty != nil && m.Text() != before {
		m.markDirty()
	}
}

func (m *InputMethodModule) HandleCommand(cmd string) bool {
	if cmd != "im-refresh" {
		return false
	}

	before := m.Text()
	m.refresh()
	if m.markDirty != nil && m.Text() != before {
		m.markDirty()
	}

	return true
}

func (m *InputMethodModule) refresh() {
	cmd := exec.Command("qdbus", "org.fcitx.Fcitx5", "/controller", "org.fcitx.Fcitx.Controller1.CurrentInputMethod")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	name := strings.TrimSpace(string(output))
	if name == "" {
		m.SetText("")
		return
	}

	if idx := strings.LastIndex(name, "."); idx >= 0 && idx < len(name)-1 {
		name = name[idx+1:]
	}

	m.SetText(name)
}
