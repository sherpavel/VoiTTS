package main

import "net"

// mDNS on Windows is not guaranteed for average user because of the Firewall and other defaults, so easier to assume no mDNS setup.
func mdnsHost(net.IP) string { return "" }
