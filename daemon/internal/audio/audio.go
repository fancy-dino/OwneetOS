// SPDX-License-Identifier: GPL-3.0-or-later

// Package audio controls sound output through PipeWire's PulseAudio protocol (pipewire-pulse):
// volume and mute of the default output, and the choice of output (speakers, HDMI, headphones,
// Bluetooth, USB).
package audio

import (
	"errors"
	"math"
	"sort"
	"strings"
)

// Status is the body of GET /v1/audio.
type Status struct {
	// Available is false when the audio server cannot be reached.
	Available bool `json:"available"`
	// Output is the id of the default output ("" when there is none).
	Output  string   `json:"output"`
	Volume  int      `json:"volume"` // default output, 0-100 (perceived loudness, as in mixers)
	Muted   bool     `json:"muted"`
	Outputs []Output `json:"outputs"`
}

// Output is a sound output.
type Output struct {
	ID      string `json:"id"`   // stable across reboots
	Name    string `json:"name"` // human-readable, from the driver
	Kind    string `json:"kind"`
	Default bool   `json:"default"`
	Volume  int    `json:"volume"`
	Muted   bool   `json:"muted"`
}

// Kinds of output, for the UI's icons.
const (
	KindSpeakers   = "speakers"
	KindHeadphones = "headphones"
	KindHDMI       = "hdmi"
	KindBluetooth  = "bluetooth"
	KindUSB        = "usb"
	KindOther      = "other"
)

// Errors returned to the API.
var (
	ErrUnavailable   = errors.New("the audio server is not running")
	ErrNoOutput      = errors.New("no sound output")
	ErrUnknownOutput = errors.New("unknown sound output")
)

// Sink is what the audio server reports about an output.
type Sink struct {
	Index       uint32
	Name        string
	Description string
	ActivePort  string
	Props       map[string]string
	Channels    int
	Volume      int
	Muted       bool
}

// dummySink is PipeWire's placeholder when there is no real output.
const dummySink = "auto_null"

// Kind tells what kind of output a sink is.
func Kind(s Sink) string {
	port := strings.ToLower(s.ActivePort)
	name := strings.ToLower(s.Name)
	form := s.Props["device.form_factor"]
	switch {
	case strings.Contains(port, "hdmi") || strings.Contains(name, "hdmi") ||
		strings.Contains(strings.ToLower(s.Description), "hdmi"):
		return KindHDMI
	case s.Props["device.api"] == "bluez5":
		return KindBluetooth
	case form == "headset" || form == "headphone" || form == "hands-free" ||
		strings.Contains(port, "headphone") || strings.Contains(port, "headset"):
		return KindHeadphones
	case s.Props["device.bus"] == "usb":
		return KindUSB
	case form == "internal" || form == "speaker" || strings.Contains(port, "speaker") ||
		strings.Contains(port, "lineout") || strings.Contains(port, "analog-output"):
		return KindSpeakers
	}
	return KindOther
}

// Build makes the API status from the sinks and the name of the default sink.
func Build(sinks []Sink, defaultName string) Status {
	st := Status{Available: true, Outputs: []Output{}}
	for _, s := range sinks {
		if s.Name == dummySink {
			continue
		}
		o := Output{ID: s.Name, Name: s.Description, Kind: Kind(s), Default: s.Name == defaultName,
			Volume: s.Volume, Muted: s.Muted}
		if o.Name == "" {
			o.Name = s.Name
		}
		if o.Default {
			st.Output, st.Volume, st.Muted = o.ID, o.Volume, o.Muted
		}
		st.Outputs = append(st.Outputs, o)
	}
	sort.Slice(st.Outputs, func(i, j int) bool {
		a, b := st.Outputs[i], st.Outputs[j]
		if a.Default != b.Default {
			return a.Default
		}
		return a.Name < b.Name
	})
	return st
}

// volumeNorm is 100% in PulseAudio volume units (PA_VOLUME_NORM).
const volumeNorm = 0x10000

// Percent converts PulseAudio channel volumes to a percentage (the loudest channel, as mixers do).
func Percent(channels []uint32) int {
	var top uint32
	for _, v := range channels {
		top = max(top, v)
	}
	return int(math.Round(float64(top) * 100 / volumeNorm))
}

// Channels converts a percentage (clamped to 0-100) to n equal PulseAudio channel volumes.
func Channels(percent, n int) []uint32 {
	percent = min(max(percent, 0), 100)
	v := uint32(math.Round(float64(percent) * volumeNorm / 100))
	out := make([]uint32, max(n, 1))
	for i := range out {
		out[i] = v
	}
	return out
}
