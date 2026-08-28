package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mdp/qrterminal/v3"
	"golang.org/x/term"
)

const mdnsTimeout = 500 * time.Millisecond

// announce prints where the server can be reached. The address comes from the
// listener rather than a constant: the port is whichever one listen settled on.
func announce(w io.Writer, addr net.Addr) {
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		slog.Warn("Cannot read port from listen address", "addr", addr.String(), "error", err.Error())
		return
	}
	ip, err := lanIP()
	if err != nil {
		slog.Warn("No LAN address found", "error", err.Error())
		return
	}

	url := "http://" + net.JoinHostPort(ip.String(), port)
	fmt.Fprintln(w)
	if name := mdnsHost(ip); name != "" {
		named := "http://" + net.JoinHostPort(name, port)
		fmt.Fprintf(w, "  Open on your phone: %s\n", named)
		fmt.Fprintf(w, "  Or by address:      %s\n", url)
		url = named
	} else {
		fmt.Fprintf(w, "  Open on your phone: %s\n", url)
	}
	fmt.Fprintln(w)

	// Redirected output gets the URL but no QR: the code is escape sequences,
	// which are noise in a log file and unscannable there anyway.
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return
	}

	qrterminal.GenerateWithConfig(url, qrterminal.Config{
		Level:          qrterminal.L,
		Writer:         w,
		HalfBlocks:     true,
		QuietZone:      4,
		BlackChar:      "\033[40m \033[0m",
		WhiteChar:      "\033[107m \033[0m",
		BlackWhiteChar: "\033[40;97m▄\033[0m",
		WhiteBlackChar: "\033[40;97m▀\033[0m",
	})
	fmt.Fprintln(w)
}

// Returns machine's mDNS name if available, otherwise empty string
func mdnsHost(ip net.IP) string {
	host, err := os.Hostname()
	if err != nil || host == "" || strings.Contains(host, ".") {
		return ""
	}
	name := host + ".local"

	ctx, cancel := context.WithTimeout(context.Background(), mdnsTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, name)
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if net.ParseIP(addr).Equal(ip) {
			return name
		}
	}
	return ""
}

// Returns LAN ip, reachable by other devices
func lanIP() (net.IP, error) {
	if conn, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		defer conn.Close()
		if ip := conn.LocalAddr().(*net.UDPAddr).IP; ip.IsGlobalUnicast() {
			return ip, nil
		}
	}

	// Fall back to the first address on an interface
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip := ipNet.IP.To4(); ip != nil && ip.IsGlobalUnicast() {
				return ip, nil
			}
		}
	}
	return nil, fmt.Errorf("no non-loopback IPv4 address on any interface")
}
