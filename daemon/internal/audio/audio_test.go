// SPDX-License-Identifier: GPL-3.0-or-later

package audio

import "testing"

func TestKind(t *testing.T) {
	cases := []struct {
		s    Sink
		want string
	}{
		{Sink{Name: "alsa_output.pci-0000_00_11.0.analog-stereo", ActivePort: "analog-output-lineout",
			Props: map[string]string{"device.bus": "pci", "device.form_factor": "internal"}}, KindSpeakers},
		{Sink{ActivePort: "analog-output-speaker", Props: map[string]string{"device.bus": "pci"}}, KindSpeakers},
		{Sink{ActivePort: "analog-output-headphones", Props: map[string]string{"device.bus": "pci"}}, KindHeadphones},
		{Sink{Name: "alsa_output.pci-0000_03_00.1.hdmi-stereo", ActivePort: "hdmi-output-0"}, KindHDMI},
		{Sink{Description: "Radeon High Definition Audio Controller Digital Stereo (HDMI)"}, KindHDMI},
		{Sink{Name: "bluez_output.A8_8C_3E_20_CE_C7.1", Props: map[string]string{"device.api": "bluez5",
			"device.form_factor": "headset"}}, KindBluetooth},
		{Sink{ActivePort: "analog-output", Props: map[string]string{"device.bus": "usb"}}, KindUSB},
		{Sink{Props: map[string]string{"device.bus": "usb", "device.form_factor": "headset"}}, KindHeadphones},
		{Sink{Name: "something"}, KindOther},
	}
	for _, c := range cases {
		if got := Kind(c.s); got != c.want {
			t.Errorf("Kind(%+v) = %s, want %s", c.s, got, c.want)
		}
	}
}

func TestVolumeConversion(t *testing.T) {
	if got := Percent([]uint32{26214, 26214}); got != 40 {
		t.Errorf("Percent(40%%) = %d", got)
	}
	if got := Percent([]uint32{0x8000, 0x10000}); got != 100 { // loudest channel
		t.Errorf("Percent(50%%, 100%%) = %d", got)
	}
	if got := Channels(40, 2); len(got) != 2 || got[0] != 26214 || got[1] != 26214 {
		t.Errorf("Channels(40, 2) = %v", got)
	}
	if got := Channels(150, 1); got[0] != 0x10000 {
		t.Errorf("Channels(150) = %v, want clamped to 100%%", got)
	}
	if got := Channels(-5, 0); len(got) != 1 || got[0] != 0 {
		t.Errorf("Channels(-5, 0) = %v", got)
	}
	for p := 0; p <= 100; p++ {
		if got := Percent(Channels(p, 2)); got != p {
			t.Fatalf("round trip %d -> %d", p, got)
		}
	}
}

func TestBuild(t *testing.T) {
	sinks := []Sink{
		{Name: "auto_null", Description: "Dummy Output"},
		{Name: "usb", Description: "USB Audio", Props: map[string]string{"device.bus": "usb"}, Volume: 70},
		{Name: "hdmi", Description: "HDMI", ActivePort: "hdmi-output-0", Volume: 55, Muted: true},
	}
	st := Build(sinks, "hdmi")
	if !st.Available || st.Output != "hdmi" || st.Volume != 55 || !st.Muted {
		t.Fatalf("status = %+v", st)
	}
	if len(st.Outputs) != 2 || st.Outputs[0].ID != "hdmi" || !st.Outputs[0].Default || st.Outputs[1].Kind != KindUSB {
		t.Fatalf("outputs = %+v", st.Outputs)
	}
	if st := Build(sinks[:1], "auto_null"); st.Output != "" || len(st.Outputs) != 0 {
		t.Fatalf("dummy only: %+v", st)
	}
}
