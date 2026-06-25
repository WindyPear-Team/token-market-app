package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const connectorVersion = "0.1.0"

type connectorConfig struct {
	Server string
	Token  string
	Name   string
}

type connectorClient struct {
	config connectorConfig
	http   *http.Client
}

func main() {
	server := flag.String("server", "http://localhost:8080", "Backend server URL")
	token := flag.String("token", "", "Connector token generated from advanced chat")
	name := flag.String("name", "", "Device name shown in advanced chat")
	flag.Parse()

	if strings.TrimSpace(*token) == "" {
		fatalf("missing -token")
	}
	hostname, _ := os.Hostname()
	deviceName := strings.TrimSpace(*name)
	if deviceName == "" {
		deviceName = hostname
	}
	client := connectorClient{
		config: connectorConfig{
			Server: strings.TrimRight(strings.TrimSpace(*server), "/"),
			Token:  strings.TrimSpace(*token),
			Name:   deviceName,
		},
		http: &http.Client{Timeout: 35 * time.Second},
	}
	if err := client.register(); err != nil {
		fatalf("register failed: %v", err)
	}
	fmt.Printf("Connector online as %q\n", deviceName)
	go client.heartbeatLoop()
	client.pollLoop()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
