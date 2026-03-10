package main

import (
	"log"
	"net"
	"sync"

	"github.com/jfreymuth/pulse/proto"
)

type AudioState struct {
	SinkVolume    int
	SinkMuted     bool
	SourceMuted   bool
	DefaultSink   string
	DefaultSource string
}

type AudioService struct {
	mu          sync.RWMutex
	state       AudioState
	subscribers map[int]func(AudioState)
	nextSubID   int

	client *proto.Client
	conn   net.Conn
	events chan struct{}
	done   chan struct{}
}

func NewAudioService() *AudioService {
	s := &AudioService{
		subscribers: make(map[int]func(AudioState)),
		events:      make(chan struct{}, 1),
		done:        make(chan struct{}),
	}

	client, conn, err := proto.Connect("")
	if err != nil {
		log.Printf("Audio service connect error: %v", err)
		return s
	}
	s.client = client
	s.conn = conn

	s.client.Callback = func(val interface{}) {
		switch ev := val.(type) {
		case *proto.SubscribeEvent:
			if !audioEventRelevant(ev.Event) {
				return
			}
			select {
			case s.events <- struct{}{}:
			default:
			}
		case *proto.ConnectionClosed:
			log.Printf("Audio service connection closed")
		}
	}

	props := proto.PropList{
		"application.name": proto.PropListString("cheese-client-bar"),
	}
	if err := s.client.Request(&proto.SetClientName{Props: props}, nil); err != nil {
		log.Printf("Audio service client name error: %v", err)
	}
	if err := s.client.Request(&proto.Subscribe{
		Mask: proto.SubscriptionMaskServer |
			proto.SubscriptionMaskSink |
			proto.SubscriptionMaskSource,
	}, nil); err != nil {
		log.Printf("Audio service subscribe error: %v", err)
	}

	s.refresh()
	go s.run()
	return s
}

func (s *AudioService) Close() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}

	if s.conn != nil {
		_ = s.conn.Close()
	}
}

func (s *AudioService) State() AudioState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *AudioService) Subscribe(fn func(AudioState)) func() {
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

func (s *AudioService) ToggleSourceMute() error {
	s.mu.RLock()
	source := s.state.DefaultSource
	muted := s.state.SourceMuted
	s.mu.RUnlock()

	if s.client == nil || source == "" {
		return nil
	}

	if err := s.client.Request(&proto.SetSourceMute{
		SourceIndex: proto.Undefined,
		SourceName:  source,
		Mute:        !muted,
	}, nil); err != nil {
		return err
	}

	// Trigger a refresh immediately; subscription events will still keep us current.
	s.refresh()
	return nil
}

func (s *AudioService) ToggleSinkMute() error {
	s.mu.RLock()
	sink := s.state.DefaultSink
	muted := s.state.SinkMuted
	s.mu.RUnlock()

	if s.client == nil || sink == "" {
		return nil
	}

	if err := s.client.Request(&proto.SetSinkMute{
		SinkIndex: proto.Undefined,
		SinkName:  sink,
		Mute:      !muted,
	}, nil); err != nil {
		return err
	}

	s.refresh()
	return nil
}

func (s *AudioService) run() {
	for {
		select {
		case <-s.done:
			return
		case <-s.events:
			s.refresh()
		}
	}
}

func (s *AudioService) refresh() {
	if s.client == nil {
		return
	}

	server := proto.GetServerInfoReply{}
	if err := s.client.Request(&proto.GetServerInfo{}, &server); err != nil {
		log.Printf("Audio server info error: %v", err)
		return
	}

	next := s.State()
	next.DefaultSink = server.DefaultSinkName
	next.DefaultSource = server.DefaultSourceName

	if server.DefaultSinkName != "" {
		sink := proto.GetSinkInfoReply{}
		if err := s.client.Request(&proto.GetSinkInfo{
			SinkIndex: proto.Undefined,
			SinkName:  server.DefaultSinkName,
		}, &sink); err == nil {
			next.SinkVolume = averageVolumePercent(sink.ChannelVolumes)
			next.SinkMuted = sink.Mute
		} else {
			log.Printf("Audio sink info error: %v", err)
		}
	}

	if server.DefaultSourceName != "" {
		source := proto.GetSourceInfoReply{}
		if err := s.client.Request(&proto.GetSourceInfo{
			SourceIndex: proto.Undefined,
			SourceName:  server.DefaultSourceName,
		}, &source); err == nil {
			next.SourceMuted = source.Mute
		} else {
			log.Printf("Audio source info error: %v", err)
		}
	}

	s.mu.Lock()
	if next == s.state {
		s.mu.Unlock()
		return
	}
	s.state = next
	subs := make([]func(AudioState), 0, len(s.subscribers))
	for _, fn := range s.subscribers {
		subs = append(subs, fn)
	}
	s.mu.Unlock()

	for _, fn := range subs {
		if fn != nil {
			fn(next)
		}
	}
}

func audioEventRelevant(event proto.SubscriptionEventType) bool {
	switch event.GetFacility() {
	case proto.EventServer, proto.EventSink, proto.EventSource:
		return true
	default:
		return false
	}
}

func averageVolumePercent(volumes proto.ChannelVolumes) int {
	if len(volumes) == 0 {
		return 0
	}

	var total uint64
	for _, vol := range volumes {
		total += uint64(vol)
	}
	avg := float64(total) / float64(len(volumes))
	return int(avg/float64(proto.VolumeNorm)*100.0 + 0.5)
}
