// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"
	"unsafe"
)

// The structures passed to the kernel must have the kernel's sizes (they are in the ioctl numbers).
func TestStructSizes(t *testing.T) {
	if s := unsafe.Sizeof(modeCardRes{}); s != 64 {
		t.Errorf("drm_mode_card_res: %d bytes, want 64", s)
	}
	if s := unsafe.Sizeof(modeInfo{}); s != 68 {
		t.Errorf("drm_mode_modeinfo: %d bytes, want 68", s)
	}
	if s := unsafe.Sizeof(modeCrtc{}); s != 104 {
		t.Errorf("drm_mode_crtc: %d bytes, want 104", s)
	}
	if n := uintptr(drmIoctlModeGetResources>>16) & 0x3fff; n != unsafe.Sizeof(modeCardRes{}) {
		t.Errorf("GETRESOURCES size %d", n)
	}
	if n := uintptr(drmIoctlModeSetCrtc>>16) & 0x3fff; n != unsafe.Sizeof(modeCrtc{}) {
		t.Errorf("SETCRTC size %d", n)
	}
}
