package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const demoPort = "8080"

var errDemoTarget = errors.New("8080 is the demo stack; run make replicas-up and target http://localhost:48080")

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func checkTarget(target string, remote string) (*url.URL, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("%q is not a URL", target)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%q must start with http:// or https://", target)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%q names no host", target)
	}
	if isLoopback(host) {
		if u.Port() == demoPort {
			return nil, errDemoTarget
		}
		return u, nil
	}
	if !strings.EqualFold(remote, host) {
		return nil, fmt.Errorf("%s is not this computer; pass --remote %s to load test it on purpose", host, host)
	}
	return u, nil
}
