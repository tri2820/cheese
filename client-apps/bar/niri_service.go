package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"os/exec"
	"sync"
)

type NiriWindow struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	AppID       string `json:"app_id"`
	WorkspaceID int64  `json:"workspace_id"`
	IsFocused   bool   `json:"is_focused"`
	IsFloating  bool   `json:"is_floating"`
}

type NiriWorkspace struct {
	ID       int64  `json:"id"`
	Output   string `json:"output"`
	IsActive bool   `json:"is_active"`
}

type NiriState struct {
	Windows    []NiriWindow
	Workspaces []NiriWorkspace
}

type niriWindowsChanged struct {
	Windows []NiriWindow `json:"windows"`
}

type niriWorkspacesChanged struct {
	Workspaces []NiriWorkspace `json:"workspaces"`
}

type niriWindowFocusChanged struct {
	ID int64 `json:"id"`
}

type NiriService struct {
	mu          sync.RWMutex
	state       NiriState
	subscribers map[int]func(NiriState)
	nextSubID   int
	done        chan struct{}
}

func NewNiriService() *NiriService {
	s := &NiriService{
		subscribers: make(map[int]func(NiriState)),
		done:        make(chan struct{}),
	}
	s.refreshInitial()
	go s.run()
	return s
}

func (s *NiriService) Close() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}
}

func (s *NiriService) Subscribe(fn func(NiriState)) func() {
	if fn == nil {
		return func() {}
	}

	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = fn
	state := s.state
	s.mu.Unlock()

	fn(state)

	return func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

func (s *NiriService) run() {
	cmd := exec.Command("niri", "msg", "-j", "event-stream")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("Niri event-stream pipe error: %v", err)
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Niri event-stream start error: %v", err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-s.done:
			_ = cmd.Process.Kill()
			return
		default:
		}
		s.handleEvent(scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Niri event-stream error: %v", err)
	}
}

func (s *NiriService) handleEvent(line []byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false
	if data, ok := raw["WindowsChanged"]; ok {
		var ev niriWindowsChanged
		if err := json.Unmarshal(data, &ev); err == nil {
			s.state.Windows = ev.Windows
			changed = true
		}
	}
	if data, ok := raw["WorkspacesChanged"]; ok {
		var ev niriWorkspacesChanged
		if err := json.Unmarshal(data, &ev); err == nil {
			s.state.Workspaces = ev.Workspaces
			changed = true
		}
	}
	if data, ok := raw["WindowFocusChanged"]; ok {
		var ev niriWindowFocusChanged
		if err := json.Unmarshal(data, &ev); err == nil {
			for i := range s.state.Windows {
				s.state.Windows[i].IsFocused = s.state.Windows[i].ID == ev.ID
			}
			changed = true
		}
	}
	if !changed {
		return
	}

	state := s.state
	subs := make([]func(NiriState), 0, len(s.subscribers))
	for _, fn := range s.subscribers {
		subs = append(subs, fn)
	}
	go func() {
		for _, fn := range subs {
			if fn != nil {
				fn(state)
			}
		}
	}()
}

func (s *NiriService) refreshInitial() {
	var windows []NiriWindow
	var workspaces []NiriWorkspace
	if data := runJSON("niri", "msg", "-j", "windows"); len(data) > 0 {
		if err := json.Unmarshal(data, &windows); err != nil {
			log.Printf("Niri initial windows error: %v", err)
		}
	}
	if data := runJSON("niri", "msg", "-j", "workspaces"); len(data) > 0 {
		if err := json.Unmarshal(data, &workspaces); err != nil {
			log.Printf("Niri initial workspaces error: %v", err)
		}
	}

	s.mu.Lock()
	s.state = NiriState{Windows: windows, Workspaces: workspaces}
	s.mu.Unlock()
}

func runJSON(name string, args ...string) []byte {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("%s %v error: %v", name, args, err)
		return nil
	}
	return bytes.TrimSpace(out)
}

func windowsForOutput(state NiriState, output string) []NiriWindow {
	var activeWorkspaceID int64 = -1
	for _, ws := range state.Workspaces {
		if ws.Output == output && ws.IsActive {
			activeWorkspaceID = ws.ID
			break
		}
	}
	if activeWorkspaceID < 0 {
		return nil
	}

	var windows []NiriWindow
	for _, win := range state.Windows {
		if win.WorkspaceID == activeWorkspaceID {
			windows = append(windows, win)
		}
	}
	return windows
}
