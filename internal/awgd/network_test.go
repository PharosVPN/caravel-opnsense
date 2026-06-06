// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

package awgd

import (
	"strings"
	"testing"
)

// fakeNet returns a network whose commands are captured instead of executed.
func fakeNet(iface string) (*network, *[]string) {
	var cmds []string
	n := &network{
		iface:   iface,
		run:     func(name string, args ...string) error { cmds = append(cmds, name+" "+strings.Join(args, " ")); return nil },
		gateway: func() (string, error) { return "192.168.0.1", nil },
	}
	return n, &cmds
}

func TestSetAddress(t *testing.T) {
	n, cmds := fakeNet("awg0")
	if err := n.setAddress("10.86.0.5"); err != nil {
		t.Fatal(err)
	}
	want := "ifconfig awg0 inet 10.86.0.5 10.86.0.5 up"
	if (*cmds)[0] != want {
		t.Errorf("got %q, want %q", (*cmds)[0], want)
	}
}

func TestFullTunnelRoutesAndTeardown(t *testing.T) {
	n, cmds := fakeNet("awg0")
	if err := n.configure(RoutingFull, "203.0.113.7:443", nil); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"route -n add -host 203.0.113.7 192.168.0.1",
		"route -n add -net 0.0.0.0/1 -interface awg0",
		"route -n add -net 128.0.0.0/1 -interface awg0",
	}
	for i, w := range want {
		if (*cmds)[i] != w {
			t.Errorf("cmd[%d] = %q, want %q", i, (*cmds)[i], w)
		}
	}

	// teardown reverses in LIFO order: nets first, then the host pin.
	*cmds = nil
	n.teardown()
	wantUndo := []string{
		"route -n delete -net 128.0.0.0/1",
		"route -n delete -net 0.0.0.0/1",
		"route -n delete -host 203.0.113.7",
	}
	for i, w := range wantUndo {
		if (*cmds)[i] != w {
			t.Errorf("undo[%d] = %q, want %q", i, (*cmds)[i], w)
		}
	}
}

func TestSplitTunnelSkipsDefaultRoute(t *testing.T) {
	n, cmds := fakeNet("awg0")
	err := n.configure(RoutingSplit, "203.0.113.7:443", []string{"10.0.0.0/8", "0.0.0.0/0", "192.168.9.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"route -n add -net 10.0.0.0/8 -interface awg0",
		"route -n add -net 192.168.9.0/24 -interface awg0",
	}
	if len(*cmds) != len(want) {
		t.Fatalf("got %d cmds %v, want %d", len(*cmds), *cmds, len(want))
	}
	for i, w := range want {
		if (*cmds)[i] != w {
			t.Errorf("cmd[%d] = %q, want %q", i, (*cmds)[i], w)
		}
	}
}

func TestRoutingNoneInstallsNothing(t *testing.T) {
	n, cmds := fakeNet("awg0")
	if err := n.configure(RoutingNone, "203.0.113.7:443", []string{"10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	if len(*cmds) != 0 {
		t.Errorf("routing=none installed routes: %v", *cmds)
	}
}
