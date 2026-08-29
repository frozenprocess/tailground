// Command e2eprobe is the tiny in-node connectivity helper the E2E harness
// bakes into every container.
//
//	e2eprobe serve   [--ports 22,80,443] [--proto tcp|udp|both]
//	e2eprobe connect host:port --proto tcp|udp [--timeout 2s]
package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const payload = "e2eprobe\n"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "serve":
		serve(os.Args[2:])
	case "connect":
		connect(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: e2eprobe serve [--ports 22,80,443] [--proto tcp|udp|both]")
	fmt.Fprintln(os.Stderr, "       e2eprobe connect host:port --proto tcp|udp [--timeout 2s]")
	os.Exit(2)
}

func serve(args []string) {
	ports, proto := "22", "both"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port", "--ports":
			i++
			ports = args[i]
		case "--proto":
			i++
			proto = args[i]
		}
	}
	errc := make(chan error, 64)
	n := 0
	for _, p := range strings.Split(ports, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if proto == "tcp" || proto == "both" {
			p := p
			n++
			go func() { errc <- serveTCP(p) }()
		}
		if proto == "udp" || proto == "both" {
			p := p
			n++
			go func() { errc <- serveUDP(p) }()
		}
	}
	if n == 0 {
		fmt.Fprintln(os.Stderr, "e2eprobe serve: no ports")
		os.Exit(2)
	}
	if err := <-errc; err != nil {
		fmt.Fprintln(os.Stderr, "e2eprobe serve:", err)
		os.Exit(1)
	}
}

func serveTCP(port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer c.Close()
			buf := make([]byte, 512)
			_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, _ := c.Read(buf)
			if n > 0 {
				_, _ = c.Write(buf[:n])
			}
		}()
	}
}

func serveUDP(port string) error {
	pc, err := net.ListenPacket("udp", ":"+port)
	if err != nil {
		return err
	}
	buf := make([]byte, 1500)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			return err
		}
		_, _ = pc.WriteTo(buf[:n], addr)
	}
}

func connect(args []string) {
	if len(args) < 1 {
		usage()
	}
	addr := args[0]
	proto, timeout := "tcp", 2*time.Second
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--proto":
			i++
			proto = args[i]
		case "--timeout":
			i++
			d, err := time.ParseDuration(args[i])
			if err == nil {
				timeout = d
			}
		}
	}
	if err := roundTrip(proto, addr, timeout); err != nil {
		fmt.Fprintln(os.Stderr, "e2eprobe connect:", err)
		os.Exit(1)
	}
	fmt.Println("ok")
}

func roundTrip(proto, addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	c, err := net.DialTimeout(proto, addr, timeout)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(deadline)
	if _, err := c.Write([]byte(payload)); err != nil {
		return err
	}
	buf := make([]byte, len(payload))
	n, err := c.Read(buf)
	if err != nil {
		return err
	}
	if string(buf[:n]) != payload {
		return errors.New("echo mismatch")
	}
	return nil
}
