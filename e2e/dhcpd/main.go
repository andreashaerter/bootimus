package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

type pool struct {
	mu     sync.Mutex
	start  net.IP
	count  int
	leases map[string]net.IP
}

func (p *pool) lease(mac net.HardwareAddr) (net.IP, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ip, ok := p.leases[mac.String()]; ok {
		return ip, true
	}
	if len(p.leases) >= p.count {
		return nil, false
	}
	ip := make(net.IP, 4)
	copy(ip, p.start)
	ip[3] += byte(len(p.leases))
	p.leases[mac.String()] = ip
	return ip, true
}

func main() {
	iface := flag.String("iface", "", "interface to serve on")
	server := flag.String("server", "", "address of this server")
	start := flag.String("start", "", "first address of the pool")
	count := flag.Int("count", 10, "pool size")
	mask := flag.String("mask", "255.255.255.0", "subnet mask")
	lease := flag.Duration("lease", time.Hour, "lease time")
	flag.Parse()

	serverIP := net.ParseIP(*server).To4()
	startIP := net.ParseIP(*start).To4()
	maskIP := net.ParseIP(*mask).To4()
	if *iface == "" || serverIP == nil || startIP == nil || maskIP == nil {
		fmt.Fprintln(os.Stderr, "usage: dhcpd -iface IF -server IP -start IP [-count N] [-mask MASK] [-lease D]")
		os.Exit(2)
	}
	p := &pool{start: startIP, count: *count, leases: map[string]net.IP{}}

	handler := func(conn net.PacketConn, peer net.Addr, m *dhcpv4.DHCPv4) {
		var kind dhcpv4.MessageType
		switch m.MessageType() {
		case dhcpv4.MessageTypeDiscover:
			kind = dhcpv4.MessageTypeOffer
		case dhcpv4.MessageTypeRequest:
			kind = dhcpv4.MessageTypeAck
		default:
			return
		}
		ip, ok := p.lease(m.ClientHWAddr)
		if !ok {
			log.Printf("dhcpd: pool exhausted, ignoring %s from %s", m.MessageType(), m.ClientHWAddr)
			return
		}
		reply, err := dhcpv4.NewReplyFromRequest(m,
			dhcpv4.WithMessageType(kind),
			dhcpv4.WithYourIP(ip),
			dhcpv4.WithOption(dhcpv4.OptServerIdentifier(serverIP)),
			dhcpv4.WithNetmask(net.IPMask(maskIP)),
			dhcpv4.WithLeaseTime(uint32(lease.Seconds())),
		)
		if err != nil {
			log.Printf("dhcpd: build reply: %v", err)
			return
		}
		if _, err := conn.WriteTo(reply.ToBytes(), &net.UDPAddr{IP: net.IPv4bcast, Port: 68}); err != nil {
			log.Printf("dhcpd: send %s: %v", kind, err)
			return
		}
		log.Printf("dhcpd: %s from %s -> %s %s", m.MessageType(), m.ClientHWAddr, kind, ip)
	}

	srv, err := server4.NewServer(*iface, &net.UDPAddr{IP: net.IPv4zero, Port: 67}, handler)
	if err != nil {
		log.Fatalf("dhcpd: listen on %s: %v", *iface, err)
	}
	log.Printf("dhcpd: listening on %s, pool %s +%d, server %s", *iface, startIP, *count, serverIP)
	if err := srv.Serve(); err != nil {
		log.Fatalf("dhcpd: %v", err)
	}
}
