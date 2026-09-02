package main

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"

	"github.com/mdp/qrterminal/v3"
	"golang.org/x/term"
)

// Prints where the server can be reached + a QR code for phones/tablets.
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

	// If not a terminal output, skips QR (e.g. log file)
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
