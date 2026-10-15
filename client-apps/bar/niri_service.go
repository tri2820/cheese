package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"os/exec"
	"sort"
	"sync"
	"time"
)

type NiriWindowLayout struct {
	PosInScrollingLayout [2]int `json:"pos_in_scrolling_layout"`
}

type NiriWindow struct {
	ID          int64            `json:"id"`
	Title       string           `json:"title"`
	AppID       string           `json:"app_id"`
	WorkspaceID int64            `json:"workspace_id"`
	IsFocused   bool             `json:"is_focused"`
	IsFloating  bool             `json:"is_floating"`
	Layout      NiriWindowLayout `json:"layout"`
}

type NiriWorkspace struct {
	ID       int64  `json:"id"`
	Idx      int    `json:"idx"`
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

type niriWorkspaceActivated struct {
	ID int64 `json:"id"`
}

type niriWindowFocusChanged struct {
	ID int64 `json:"id"`
}

type niriWindowLayoutsChanged struct {
	Changes [][2]json.RawMessage `json:"changes"`
}

type niriWindowOpenedOrChanged struct {
	Window NiriWindow `json:"window"`
}

type niriWindowClosed struct {
	ID int64 `json:"id"`
}

type NiriService struct {
	mu          sync.RWMutex
	state       NiriState
	subscribers map[int]func(NiriState)
	nextSubID   int
	done        chan struct{}
}

const (
	niriEventScannerInitialBuf = 64 * 1024
	niriEventScannerMaxBuf     = 4 * 1024 * 1024
	niriEventRestartDelay      = time.Second
)

var errNiriServiceStopped = errors.New("niri service stopped")

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
	state := cloneNiriState(s.state)
	s.mu.Unlock()

	fn(state)

	return func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

func (s *NiriService) run() {
	for {
		if err := s.runEventStream(); err != nil && !errors.Is(err, errNiriServiceStopped) {
			log.Printf("Niri event-stream error: %v", err)
		}

		select {
		case <-s.done:
			return
		case <-time.After(niriEventRestartDelay):
		}

		s.refreshInitial()
	}
}

func (s *NiriService) runEventStream() error {
	cmd := exec.Command("niri", "msg", "-j", "event-stream")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	stopWatcher := make(chan struct{})
	go func() {
		select {
		case <-s.done:
			_ = cmd.Process.Kill()
		case <-stopWatcher:
		}
	}()
	defer close(stopWatcher)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, niriEventScannerInitialBuf), niriEventScannerMaxBuf)
	for scanner.Scan() {
		s.handleEvent(scanner.Bytes())
	}

	scanErr := scanner.Err()
	waitErr := cmd.Wait()

	select {
	case <-s.done:
		return errNiriServiceStopped
	default:
	}

	if scanErr != nil {
		return scanErr
	}
	if waitErr != nil {
		return waitErr
	}

	return errors.New("niri event-stream exited")
}

func (s *NiriService) handleEvent(line []byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		return
	}

	s.mu.Lock()

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
	if data, ok := raw["WorkspaceActivated"]; ok {
		var ev niriWorkspaceActivated
		if err := json.Unmarshal(data, &ev); err == nil {
			targetOutput := ""
			for i := range s.state.Workspaces {
				if s.state.Workspaces[i].ID == ev.ID {
					targetOutput = s.state.Workspaces[i].Output
					break
				}
			}
			if targetOutput != "" {
				for i := range s.state.Workspaces {
					if s.state.Workspaces[i].Output != targetOutput {
						continue
					}
					isActive := s.state.Workspaces[i].ID == ev.ID
					if s.state.Workspaces[i].IsActive != isActive {
						s.state.Workspaces[i].IsActive = isActive
						changed = true
					}
				}
			}
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
	if data, ok := raw["WindowLayoutsChanged"]; ok {
		var ev niriWindowLayoutsChanged
		if err := json.Unmarshal(data, &ev); err == nil {
			for _, pair := range ev.Changes {
				if len(pair) != 2 {
					continue
				}
				var id int64
				var layout NiriWindowLayout
				if err := json.Unmarshal(pair[0], &id); err != nil {
					continue
				}
				if err := json.Unmarshal(pair[1], &layout); err != nil {
					continue
				}
				for i := range s.state.Windows {
					if s.state.Windows[i].ID == id {
						s.state.Windows[i].Layout = layout
						changed = true
						break
					}
				}
			}
		}
	}
	if data, ok := raw["WindowOpenedOrChanged"]; ok {
		var ev niriWindowOpenedOrChanged
		if err := json.Unmarshal(data, &ev); err == nil {
			updated := false
			for i := range s.state.Windows {
				if s.state.Windows[i].ID == ev.Window.ID {
					s.state.Windows[i] = ev.Window
					updated = true
					break
				}
			}
			if !updated {
				s.state.Windows = append(s.state.Windows, ev.Window)
			}
			changed = true
		}
	}
	if data, ok := raw["WindowClosed"]; ok {
		var ev niriWindowClosed
		if err := json.Unmarshal(data, &ev); err == nil {
			for i := range s.state.Windows {
				if s.state.Windows[i].ID == ev.ID {
					s.state.Windows = append(s.state.Windows[:i], s.state.Windows[i+1:]...)
					changed = true
					break
				}
			}
		}
	}
	if !changed {
		s.mu.Unlock()
		return
	}

	state := cloneNiriState(s.state)
	subs := make([]func(NiriState), 0, len(s.subscribers))
	for _, fn := range s.subscribers {
		subs = append(subs, fn)
	}
	s.mu.Unlock()

	for _, fn := range subs {
		if fn != nil {
			fn(state)
		}
	}
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
	sort.SliceStable(windows, func(i, j int) bool {
		xi := windows[i].Layout.PosInScrollingLayout[0]
		xj := windows[j].Layout.PosInScrollingLayout[0]
		if xi != xj {
			return xi < xj
		}
		return windows[i].ID < windows[j].ID
	})
	return windows
}

func cloneNiriState(state NiriState) NiriState {
	return NiriState{
		Windows:    append([]NiriWindow(nil), state.Windows...),
		Workspaces: append([]NiriWorkspace(nil), state.Workspaces...),
	}
}
