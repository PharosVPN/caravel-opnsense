// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

package awgd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/PharosVPN/caravel/core/vp"
	"github.com/amnezia-vpn/amneziawg-go/ipc"
	"github.com/amnezia-vpn/amneziawg-go/tun"
)

// shutdownTimeout bounds teardown so a hung route(8) or device close can't
// outlast OPNsense rc.d's stop window and get SIGKILL'd mid-cleanup.
const shutdownTimeout = 8 * time.Second

// Logf is a minimal structured-logging hook (printf-style).
type Logf func(format string, args ...any)

// Run brings up the tunnel described by r: it creates the FreeBSD tun, starts
// the caravel AmneziaWG engine over it, applies the host addressing/routing, and
// serves the wireguard UAPI socket (/var/run/amneziawg/<if>.sock) for status
// tooling. It blocks until ctx is cancelled, SIGINT/SIGTERM arrives, or the
// engine exits, then tears down routes and destroys the interface before
// returning. Must run as root (tun + route changes).
func Run(ctx context.Context, r *Resolved, logf Logf) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	dev, err := tun.CreateTUN(r.Interface, r.MTU)
	if err != nil {
		return fmt.Errorf("create tun %q: %w", r.Interface, err)
	}
	name, err := dev.Name()
	if err != nil {
		dev.Close()
		return fmt.Errorf("tun name: %w", err)
	}
	logf("interface %s created (mtu %d)", name, r.MTU)

	vt, err := vp.Up(r.VP, dev, r.LogLevel)
	if err != nil {
		dev.Close() // vp.Up closes the device on failure, but be safe
		return fmt.Errorf("bring up tunnel: %w", err)
	}
	logf("amneziawg engine up, endpoint %s", r.VP.Endpoint)

	netw := newNetwork(name)
	if err := netw.setAddress(r.Address); err != nil {
		vt.Close()
		return fmt.Errorf("set address: %w", err)
	}
	if err := netw.configure(r.Routing, r.VP.Endpoint, r.VP.AllowedIPs); err != nil {
		netw.teardown()
		vt.Close()
		return fmt.Errorf("configure routes: %w", err)
	}
	logf("address %s up, routing=%s", r.Address, r.Routing)

	uapiCloser, listenerErrs, err := serveUAPI(name, vt)
	if err != nil {
		netw.teardown()
		vt.Close()
		return err
	}
	logf("uapi socket at /var/run/amneziawg/%s.sock", name)

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigc)

	var runErr error
	select {
	case <-ctx.Done():
		logf("context cancelled, shutting down")
	case s := <-sigc:
		logf("signal %s, shutting down", s)
	case err := <-listenerErrs:
		runErr = fmt.Errorf("uapi listener: %w", err)
	case <-vt.Wait():
		logf("tunnel engine exited, shutting down")
	}

	// Tear down with a deadline: close the UAPI listener, delete routes, then
	// close the engine (which destroys the interface). Bounded so a stuck step
	// can't hold the process past rc.d's stop window.
	done := make(chan struct{})
	go func() {
		uapiCloser()
		netw.teardown()
		vt.Close() // closes the device + tun, which destroys the interface
		close(done)
	}()
	select {
	case <-done:
		logf("interface %s torn down", name)
	case <-time.After(shutdownTimeout):
		logf("teardown exceeded %s; exiting (interface/routes may linger for the OS to reap)", shutdownTimeout)
	}
	return runErr
}

// serveUAPI opens and serves the wireguard UAPI socket for the interface,
// dispatching each client connection to the tunnel. It returns a closer for the
// listener and a channel that receives the listener's terminal error.
func serveUAPI(name string, vt *vp.Tunnel) (func(), <-chan error, error) {
	file, err := ipc.UAPIOpen(name)
	if err != nil {
		return nil, nil, fmt.Errorf("open uapi socket: %w", err)
	}
	listener, err := ipc.UAPIListen(name, file)
	if err != nil {
		file.Close()
		return nil, nil, fmt.Errorf("listen uapi socket: %w", err)
	}
	errs := make(chan error, 1)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				errs <- err
				return
			}
			go vt.IpcHandle(conn)
		}
	}()
	return func() { listener.Close() }, errs, nil
}
