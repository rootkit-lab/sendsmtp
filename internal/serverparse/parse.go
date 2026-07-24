package serverparse

import (
	"strconv"
	"strings"

	"github.com/wiz/sendsmtp/internal/store"
)

// ParseLines accepts:
//
//	|IP|password
//	IP|password
//	IP|port|password          (prefer SOCKS port)
//	IP:sshPort|password
//	user@IP|password
//	IP:port:user:password     (already-running SOCKS5)
func ParseLines(raw string) (servers []store.Server, invalid int) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.Trim(line, "|")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// SOCKS already running: host:port:user:pass
		if parts := strings.Split(line, ":"); len(parts) == 4 && !strings.Contains(line, "|") {
			port, err := strconv.Atoi(parts[1])
			if err != nil || port <= 0 || parts[0] == "" {
				invalid++
				continue
			}
			servers = append(servers, store.Server{
				Host:          parts[0],
				SSHPort:       22,
				SSHUser:       "root",
				PreferPort:    port,
				ProxyPort:     port,
				ProxyUser:     parts[2],
				ProxyPassword: parts[3],
				Status:        "active",
			})
			continue
		}

		parts := strings.Split(line, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		parts = filterEmpty(parts)
		if len(parts) < 2 {
			invalid++
			continue
		}

		hostPart := parts[0]
		pass := parts[len(parts)-1]
		prefer := 10808
		if len(parts) == 3 {
			if p, err := strconv.Atoi(parts[1]); err == nil && p > 0 {
				prefer = p
			}
		}

		user := "root"
		sshPort := 22
		host := hostPart
		if at := strings.LastIndex(hostPart, "@"); at >= 0 {
			user = hostPart[:at]
			host = hostPart[at+1:]
		}
		if h, p, ok := splitHostPort(host); ok {
			host = h
			sshPort = p
		}
		if host == "" || pass == "" {
			invalid++
			continue
		}
		servers = append(servers, store.Server{
			Host:        host,
			SSHPort:     sshPort,
			SSHUser:     user,
			SSHPassword: pass,
			PreferPort:  prefer,
			ProxyUser:   "sendsmtp",
			Status:      "pending",
		})
	}
	return servers, invalid
}

func filterEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func splitHostPort(s string) (host string, port int, ok bool) {
	// Only treat as host:port when port is numeric (IPv4).
	i := strings.LastIndex(s, ":")
	if i <= 0 {
		return s, 0, false
	}
	p, err := strconv.Atoi(s[i+1:])
	if err != nil || p <= 0 || p > 65535 {
		return s, 0, false
	}
	return s[:i], p, true
}
