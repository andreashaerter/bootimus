package server

import (
	"net"
	"strings"
	"testing"
)

func TestStartRejectsOccupiedRequiredPorts(t *testing.T) {
	for _, service := range []string{"TFTP", "boot HTTP", "admin HTTP"} {
		t.Run(service, func(t *testing.T) {
			tftpSocket, err := net.ListenUDP("udp", &net.UDPAddr{})
			if err != nil {
				t.Fatal(err)
			}
			defer tftpSocket.Close()
			httpSocket, err := net.Listen("tcp", ":0")
			if err != nil {
				t.Fatal(err)
			}
			defer httpSocket.Close()
			adminSocket, err := net.Listen("tcp", ":0")
			if err != nil {
				t.Fatal(err)
			}
			defer adminSocket.Close()
			config := &Config{
				TFTPPort:  tftpSocket.LocalAddr().(*net.UDPAddr).Port,
				HTTPPort:  httpSocket.Addr().(*net.TCPAddr).Port,
				AdminPort: adminSocket.Addr().(*net.TCPAddr).Port,
				DataDir:   t.TempDir(),
				ISODir:    t.TempDir(),
			}
			if service != "TFTP" {
				tftpSocket.Close()
			}
			if service != "boot HTTP" {
				httpSocket.Close()
			}
			if service != "admin HTTP" {
				adminSocket.Close()
			}
			server := &Server{config: config}
			err = server.Start()
			if err == nil {
				server.Shutdown()
				t.Fatalf("Start succeeded with an occupied %s port", service)
			}
			if !strings.Contains(err.Error(), service) {
				t.Fatalf("error does not identify %s: %v", service, err)
			}
			if service != "TFTP" {
				rebound, err := net.ListenUDP("udp", &net.UDPAddr{Port: config.TFTPPort})
				if err != nil {
					t.Fatalf("TFTP socket leaked after failed startup: %v", err)
				}
				rebound.Close()
			}
			if service != "boot HTTP" {
				rebound, err := net.Listen("tcp", httpSocket.Addr().String())
				if err != nil {
					t.Fatalf("HTTP socket leaked after failed startup: %v", err)
				}
				rebound.Close()
			}
		})
	}
}

func TestStartRejectsInvalidAdminTLS(t *testing.T) {
	server := &Server{config: &Config{AdminTLS: AdminTLSConfig{
		Enabled: true, CertFile: "missing.pem", KeyFile: "missing.key",
	}}}
	if err := server.Start(); err == nil || !strings.Contains(err.Error(), "admin TLS") {
		t.Fatalf("Start error = %v, want admin TLS failure", err)
	}
}
