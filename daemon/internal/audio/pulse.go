// SPDX-License-Identifier: GPL-3.0-or-later

package audio

import (
	"context"
	"log/slog"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/jfreymuth/pulse/proto"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// Pulse talks to pipewire-pulse at $XDG_RUNTIME_DIR/pulse/native (started by systemd socket
// activation on the first connection).
type Pulse struct {
	Log    *slog.Logger
	Broker *events.Broker

	mu     sync.Mutex // guards client, conn, last
	client *proto.Client
	conn   net.Conn
	last   Status
}

// Run keeps a connection to the audio server, reconnecting when it restarts, and publishes
// audio.changed until ctx ends.
func (p *Pulse) Run(ctx context.Context) {
	wait := time.Second
	for ctx.Err() == nil {
		changes, closed, err := p.connect()
		if err != nil {
			p.Log.Debug("audio server not reachable", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = min(wait*2, 30*time.Second)
			continue
		}
		wait = time.Second
		p.Log.Info("connected to the audio server")
		p.refresh()
		p.serve(ctx, changes, closed)
		p.disconnect()
		p.refresh()
	}
}

// serve waits for changes until the connection closes or ctx ends. The server sends bursts of
// change notifications: the state is read again at most every 100 ms.
func (p *Pulse) serve(ctx context.Context, changes, closed <-chan struct{}) {
	var timer <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-closed:
			p.Log.Warn("audio server connection closed")
			return
		case <-changes:
			if timer == nil {
				timer = time.After(100 * time.Millisecond)
			}
		case <-timer:
			timer = nil
			p.refresh()
		}
	}
}

func (p *Pulse) connect() (changes, closed chan struct{}, err error) {
	client, conn, err := proto.Connect("")
	if err != nil {
		return nil, nil, err
	}
	changes, closed = make(chan struct{}, 1), make(chan struct{})
	var once sync.Once
	// The callback runs on the client's read loop: it must not send requests.
	client.Callback = func(msg any) {
		switch msg.(type) {
		case *proto.SubscribeEvent:
			select {
			case changes <- struct{}{}:
			default: // a refresh is already pending
			}
		case *proto.ConnectionClosed:
			once.Do(func() { close(closed) })
		}
	}
	props := proto.PropList{"application.name": proto.PropListString("owneetd")}
	if err := client.Request(&proto.SetClientName{Props: props}, &proto.SetClientNameReply{}); err != nil {
		conn.Close()
		return nil, nil, err
	}
	mask := proto.SubscriptionMaskSink | proto.SubscriptionMaskServer | proto.SubscriptionMaskCard
	if err := client.Request(&proto.Subscribe{Mask: mask}, nil); err != nil {
		conn.Close()
		return nil, nil, err
	}
	p.mu.Lock()
	p.client, p.conn = client, conn
	p.mu.Unlock()
	return changes, closed, nil
}

func (p *Pulse) disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		p.conn.Close()
	}
	p.client, p.conn = nil, nil
}

func (p *Pulse) refresh() {
	st := p.Status()
	p.mu.Lock()
	changed := !reflect.DeepEqual(st, p.last)
	p.last = st
	p.mu.Unlock()
	if changed {
		p.Broker.Publish("audio.changed", st)
	}
}

func (p *Pulse) currentClient() (*proto.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil {
		return nil, ErrUnavailable
	}
	return p.client, nil
}

// sinks reads the outputs and the name of the default one.
func (p *Pulse) sinks() ([]Sink, string, error) {
	c, err := p.currentClient()
	if err != nil {
		return nil, "", err
	}
	var list proto.GetSinkInfoListReply
	if err := c.Request(&proto.GetSinkInfoList{}, &list); err != nil {
		return nil, "", err
	}
	var server proto.GetServerInfoReply
	if err := c.Request(&proto.GetServerInfo{}, &server); err != nil {
		return nil, "", err
	}
	out := make([]Sink, 0, len(list))
	for _, s := range list {
		props := make(map[string]string, len(s.Properties))
		for k, v := range s.Properties {
			props[k] = v.String()
		}
		volumes := make([]uint32, len(s.ChannelVolumes))
		for i, v := range s.ChannelVolumes {
			volumes[i] = uint32(v)
		}
		out = append(out, Sink{Index: s.SinkIndex, Name: s.SinkName, Description: s.Device,
			ActivePort: s.ActivePortName, Props: props, Channels: len(volumes), Volume: Percent(volumes),
			Muted: s.Mute})
	}
	return out, server.DefaultSinkName, nil
}

// Status reports the outputs, the default one and its volume.
func (p *Pulse) Status() Status {
	sinks, def, err := p.sinks()
	if err != nil {
		return Status{Outputs: []Output{}}
	}
	return Build(sinks, def)
}

// defaultSink returns the default output.
func (p *Pulse) defaultSink() (*proto.Client, Sink, error) {
	c, err := p.currentClient()
	if err != nil {
		return nil, Sink{}, err
	}
	sinks, def, err := p.sinks()
	if err != nil {
		return nil, Sink{}, err
	}
	for _, s := range sinks {
		if s.Name == def && s.Name != dummySink {
			return c, s, nil
		}
	}
	return nil, Sink{}, ErrNoOutput
}

// SetVolume sets the default output's volume (0-100, all channels alike).
func (p *Pulse) SetVolume(percent int) error {
	c, sink, err := p.defaultSink()
	if err != nil {
		return err
	}
	var cv proto.ChannelVolumes
	for _, v := range Channels(percent, sink.Channels) {
		cv = append(cv, proto.Volume(v))
	}
	return c.Request(&proto.SetSinkVolume{SinkIndex: sink.Index, ChannelVolumes: cv}, nil)
}

// SetMuted mutes or unmutes the default output.
func (p *Pulse) SetMuted(muted bool) error {
	c, sink, err := p.defaultSink()
	if err != nil {
		return err
	}
	return c.Request(&proto.SetSinkMute{SinkIndex: sink.Index, Mute: muted}, nil)
}

// SetOutput makes an output the default one; playing sounds follow it.
func (p *Pulse) SetOutput(id string) error {
	c, err := p.currentClient()
	if err != nil {
		return err
	}
	sinks, _, err := p.sinks()
	if err != nil {
		return err
	}
	for _, s := range sinks {
		if s.Name == id && s.Name != dummySink {
			if err := c.Request(&proto.SetDefaultSink{SinkName: id}, nil); err != nil {
				return err
			}
			p.Log.Info("default sound output changed", "output", id)
			return nil
		}
	}
	return ErrUnknownOutput
}
