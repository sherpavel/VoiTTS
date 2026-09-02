package main

import (
	"context"
	"net"
	"os"
	"strings"
	"time"
)

const mdnsTimeout = 500 * time.Millisecond

// Returns the machine's mDNS name or empty string.
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
