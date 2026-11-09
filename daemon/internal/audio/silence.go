// SPDX-License-Identifier: GPL-3.0-or-later

package audio

import (
	"strconv"
	"strings"

	"github.com/jfreymuth/pulse/proto"
)

// A game or a video app left open behind the home screen goes silent, and its sound comes back
// with it (owner, 2026-11-09, [[step-4.5-windows-games-proton]]): each playback stream (sink
// input) of such an app is muted, and unmuted when the app is on screen again. owneetd only
// unmutes the streams it muted itself: a stream muted by its own program stays as it is.

// SetSilence gives the rule: whether a stream of this process must be silent now.
func (p *Pulse) SetSilence(f func(pid int) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.silence = f
}

// streamPID: the process that plays a stream (its "application.process.id"), 0 when unknown.
func streamPID(props proto.PropList) int {
	e, ok := props["application.process.id"]
	if !ok {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimRight(e.String(), "\x00"))
	if err != nil {
		return 0
	}
	return pid
}

// ApplySilence mutes the streams that must be silent now and unmutes the ones it muted that need
// not be any more. Called when what is on screen changes and when a stream starts.
func (p *Pulse) ApplySilence() {
	p.silenceMu.Lock()
	defer p.silenceMu.Unlock()
	client, err := p.currentClient()
	if err != nil {
		return
	}
	p.mu.Lock()
	rule := p.silence
	p.mu.Unlock()
	if rule == nil {
		return
	}
	var streams proto.GetSinkInputInfoListReply
	if err := client.Request(&proto.GetSinkInputInfoList{}, &streams); err != nil {
		p.Log.Debug("cannot list the sound streams", "err", err)
		return
	}
	if p.silenced == nil {
		p.silenced = map[uint32]bool{}
	}
	seen := map[uint32]bool{}
	for _, s := range streams {
		seen[s.SinkInputIndex] = true
		pid := streamPID(s.Properties)
		silent := pid > 0 && rule(pid)
		mine := p.silenced[s.SinkInputIndex]
		switch {
		case silent && !s.Muted:
			if err := client.Request(&proto.SetSinkInputMute{SinkInputIndex: s.SinkInputIndex, Mute: true}, nil); err != nil {
				p.Log.Warn("cannot silence a sound stream", "pid", pid, "err", err)
				continue
			}
			p.silenced[s.SinkInputIndex] = true
			p.Log.Info("sound silenced behind the home screen", "pid", pid, "stream", s.SinkInputIndex)
		case !silent && mine:
			delete(p.silenced, s.SinkInputIndex)
			if !s.Muted {
				continue // unmuted meanwhile by its own program
			}
			if err := client.Request(&proto.SetSinkInputMute{SinkInputIndex: s.SinkInputIndex, Mute: false}, nil); err != nil {
				p.Log.Warn("cannot give a sound stream back", "pid", pid, "err", err)
				continue
			}
			p.Log.Info("sound back", "pid", pid, "stream", s.SinkInputIndex)
		}
	}
	for idx := range p.silenced {
		if !seen[idx] {
			delete(p.silenced, idx) // the stream ended
		}
	}
}
