package compute

import (
	"bufio"
	"io"
	"net"
	"os"
	"strings"
)

const fallbackResolverConfig = "nameserver 1.1.1.1\nnameserver 8.8.8.8\n"

// guestResolverConfig supplies the TSI guest with explicit resolvers. Minimal
// root filesystems commonly ship an empty /etc/resolv.conf, while libkrun's
// transparent socket backend still requires applications to have nameserver
// addresses before DNS traffic can be proxied.
func guestResolverConfig() string {
	file, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return fallbackResolverConfig
	}
	defer file.Close()
	servers := parseNameServers(file)
	if len(servers) == 0 {
		return fallbackResolverConfig
	}
	var config strings.Builder
	for _, server := range servers {
		config.WriteString("nameserver ")
		config.WriteString(server)
		config.WriteByte('\n')
	}
	config.WriteString("options timeout:2 attempts:2\n")
	return config.String()
}

func parseNameServers(reader io.Reader) []string {
	var servers []string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() && len(servers) < 3 {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || fields[0] != "nameserver" || net.ParseIP(fields[1]) == nil {
			continue
		}
		servers = append(servers, fields[1])
	}
	return servers
}
