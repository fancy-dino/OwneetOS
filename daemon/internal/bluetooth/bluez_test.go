// SPDX-License-Identifier: GPL-3.0-or-later

package bluetooth

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

type objects = map[dbus.ObjectPath]map[string]map[string]dbus.Variant

func adapterObj(addr string) map[string]map[string]dbus.Variant {
	return map[string]map[string]dbus.Variant{ifaceAdapter: {"Address": dbus.MakeVariant(addr)}}
}

func deviceObj(adapter dbus.ObjectPath, paired bool) map[string]map[string]dbus.Variant {
	return map[string]map[string]dbus.Variant{ifaceDevice: {
		"Adapter": dbus.MakeVariant(adapter), "Paired": dbus.MakeVariant(paired)}}
}

// hci0 / hci1 may swap after a suspend: the choice follows the adapters, not their numbers
func TestChooseAdapterIsStable(t *testing.T) {
	// no pairings: the lowest address, whatever its number
	objs := objects{
		"/org/bluez/hci0": adapterObj("F0:00:00:00:00:02"),
		"/org/bluez/hci1": adapterObj("A0:00:00:00:00:01"),
	}
	if got := chooseAdapter(objs); got != "/org/bluez/hci1" {
		t.Fatalf("without pairings: got %s, want the lowest address (hci1)", got)
	}
	// the adapter with the paired controller wins, also after the numbers swap
	objs["/org/bluez/hci0/dev_94_58_CB_EF_B6_C9"] = deviceObj("/org/bluez/hci0", true)
	objs["/org/bluez/hci1/dev_11_22_33_44_55_66"] = deviceObj("/org/bluez/hci1", false)
	if got := chooseAdapter(objs); got != "/org/bluez/hci0" {
		t.Fatalf("with a pairing: got %s, want hci0", got)
	}
	if got := chooseAdapter(objects{}); got != "" {
		t.Fatalf("no adapter: got %q", got)
	}
}
