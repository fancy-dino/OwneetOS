// SPDX-License-Identifier: GPL-3.0-or-later

// owneet-screens-off turns off the screens of the graphics cards the console does not use, and
// keeps them off until it is stopped (PROJECT_RULES.md, decision log 2026-10-10). gamescope drives
// one screen and turns off the others on its own card; a screen on another card would otherwise
// keep the last boot splash frame. It becomes DRM master of every other card, turns off all of its
// outputs and holds them; when it stops, the kernel gives the cards back to the text console.
//
//	owneet-screens-off CARD     (e.g. card1: the console's card, left alone)
//
// Run as root by owneet-screens-off@CARD.service, started and stopped by owneetd.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DRM ioctls (include/uapi/drm/drm.h, drm_mode.h).
const (
	drmIoctlSetMaster        = 0x0000641e // _IO('d', 0x1e)
	drmIoctlModeGetResources = 0xc04064a0 // _IOWR('d', 0xa0, struct drm_mode_card_res) (64 bytes)
	drmIoctlModeSetCrtc      = 0xc06864a2 // _IOWR('d', 0xa2, struct drm_mode_crtc) (104 bytes)
)

type modeCardRes struct {
	fbIDPtr, crtcIDPtr, connectorIDPtr, encoderIDPtr uint64
	countFbs, countCrtcs, countConnectors, countEnc  uint32
	minWidth, maxWidth, minHeight, maxHeight         uint32
}

type modeInfo struct {
	clock                                         uint32
	hdisplay, hsyncStart, hsyncEnd, htotal, hskew uint16
	vdisplay, vsyncStart, vsyncEnd, vtotal, vscan uint16
	vrefresh, flags, typ                          uint32
	name                                          [32]byte
}

type modeCrtc struct {
	setConnectorsPtr uint64
	countConnectors  uint32
	crtcID           uint32
	fbID             uint32
	x, y             uint32
	gammaSize        uint32
	modeValid        uint32
	mode             modeInfo
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// crtcs returns the card's CRTCs (display pipes).
func crtcs(fd int) ([]uint32, error) {
	var res modeCardRes
	if err := ioctl(fd, drmIoctlModeGetResources, unsafe.Pointer(&res)); err != nil {
		return nil, err
	}
	if res.countCrtcs == 0 {
		return nil, nil
	}
	ids := make([]uint32, res.countCrtcs)
	res = modeCardRes{crtcIDPtr: uint64(uintptr(unsafe.Pointer(&ids[0]))), countCrtcs: res.countCrtcs}
	if err := ioctl(fd, drmIoctlModeGetResources, unsafe.Pointer(&res)); err != nil {
		return nil, err
	}
	return ids[:res.countCrtcs], nil
}

// turnOff becomes master of a card and turns off all its outputs; the open card is returned so
// that they stay off.
func turnOff(path string) (int, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if err := ioctl(fd, drmIoctlSetMaster, nil); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("in use by another program: %w", err) // e.g. a compositor
	}
	ids, err := crtcs(fd)
	if err != nil {
		unix.Close(fd)
		return -1, err
	}
	for _, id := range ids {
		c := modeCrtc{crtcID: id} // no framebuffer, no connectors, no mode: off
		if err := ioctl(fd, drmIoctlModeSetCrtc, unsafe.Pointer(&c)); err != nil {
			fmt.Fprintf(os.Stderr, "%s: CRTC %d: %v\n", path, id, err)
		}
	}
	return fd, nil
}

var cardName = regexp.MustCompile(`^card[0-9]+$`)

func main() {
	if len(os.Args) != 2 || !cardName.MatchString(os.Args[1]) {
		fmt.Fprintln(os.Stderr, "usage: owneet-screens-off CARD   (the console's card, e.g. card1)")
		os.Exit(2)
	}
	keep := os.Args[1]
	paths, _ := filepath.Glob("/dev/dri/card*")
	var held []int
	for _, p := range paths {
		if filepath.Base(p) == keep || !cardName.MatchString(filepath.Base(p)) {
			continue
		}
		fd, err := turnOff(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s left alone: %v\n", p, err)
			continue
		}
		fmt.Printf("%s: screens turned off\n", p)
		held = append(held, fd)
	}
	if len(held) == 0 {
		fmt.Println("no other graphics card to turn off")
		return
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	for _, fd := range held {
		unix.Close(fd)
	}
}
