package main

import (
    "fmt"
    "net"
    "strconv"
    "strings"
)

// Endpoint represents an IP:Port tuple
type Endpoint struct {
    IP   net.IP
    Port uint16
}

func (e Endpoint) String() string {
    return fmt.Sprintf("%s:%d", e.IP.String(), e.Port)
}

// NewEndpoint creates a new endpoint with the passed arguments.
func NewEndpoint(ip net.IP, port uint16) Endpoint {
    return Endpoint{
        IP:   ip,
        Port: port,
    }
}

// TryParseProtocolEndpoint tries to parse a protocol-endpoint tuple like "tcp://ip:port"
func TryParseProtocolEndpoint(str string) (string, Endpoint, error) {
    splitted := strings.Split(str, "://")
    if len(splitted) != 2 {
        return "", Endpoint{}, fmt.Errorf("expected string in format schema://ip:port but got `%s`", str)
    }

    prot := splitted[0]

    endpoint, err := TryParseEndpoint(splitted[1])
    if err != nil {
        return "", Endpoint{}, fmt.Errorf("couldn't parse endpoint from `%s`, see: %v", splitted[1], err)
    }

    return prot, endpoint, nil
}

// TryParseEndpoint tries to parse to passed string in the format ip:port as endpoint
func TryParseEndpoint(str string) (Endpoint, error) {
    splitted := strings.Split(str, ":")
    if len(splitted) != 2 {
        return Endpoint{}, fmt.Errorf("expected ip:port but got `%s`", str)
    }

    ip := net.ParseIP(splitted[0]).To4()
    port, err := strconv.Atoi(splitted[1])
    if err != nil {
        return Endpoint{}, fmt.Errorf("couldnt parse port, see: %v", err)
    }

    return NewEndpoint(ip, uint16(port)), nil
}
