package proxydeploy

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/wiz/sendsmtp/internal/store"
	"golang.org/x/crypto/ssh"
)

//go:embed embed/sendsmtp-agent
var agentBinary []byte

const remoteBin = "/usr/local/bin/sendsmtp-agent"
const remoteOldSocks = "/usr/local/bin/sendsmtp-socks5d"

// Result is the outcome of an agent deploy on a remote host.
type Result struct {
	ProxyPort     int    `json:"proxy_port"`
	ProxyUser     string `json:"proxy_user"`
	ProxyPassword string `json:"proxy_password"`
	Message       string `json:"message"`
}

// ProgressFunc reports deploy phase (ssh|upload|start).
type ProgressFunc func(phase, message string)

// Deploy uploads sendsmtp-agent over SSH and starts it on a free port near PreferPort.
func Deploy(srv store.Server, timeout time.Duration) (Result, error) {
	return DeployWithProgress(srv, timeout, nil)
}

// DeployWithProgress is Deploy with optional phase callbacks.
func DeployWithProgress(srv store.Server, timeout time.Duration, onProgress ProgressFunc) (Result, error) {
	if len(agentBinary) < 1000 {
		return Result{}, fmt.Errorf("embedded sendsmtp-agent missing — run: task agent")
	}
	if srv.Host == "" || srv.SSHPassword == "" {
		return Result{}, fmt.Errorf("host and SSH password required")
	}
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	dialTO := 12 * time.Second
	if dialTO > timeout {
		dialTO = timeout
	}
	cmdTO := 20 * time.Second
	if cmdTO > timeout {
		cmdTO = timeout
	}
	if srv.SSHPort <= 0 {
		srv.SSHPort = 22
	}
	if srv.SSHUser == "" {
		srv.SSHUser = "root"
	}
	prefer := srv.PreferPort
	if prefer <= 0 {
		prefer = 18080
	}
	// proxy_user unused for agent; keep label for UI. Token = proxy_password.
	token := srv.ProxyPassword
	if token == "" {
		token = randomPass(24)
	}

	emit := func(phase, msg string) {
		if onProgress != nil {
			onProgress(phase, msg)
		}
	}

	emit("ssh", fmt.Sprintf("SSH %s…", srv.Host))
	cfg := &ssh.ClientConfig{
		User:            srv.SSHUser,
		Auth:            []ssh.AuthMethod{ssh.Password(srv.SSHPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         dialTO,
	}
	addr := net.JoinHostPort(srv.Host, strconv.Itoa(srv.SSHPort))
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return Result{}, fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	sum := sha256.Sum256(agentBinary)
	wantHash := hex.EncodeToString(sum[:])

	emit("port", fmt.Sprintf("porta livre perto de %d…", prefer))
	port, err := pickFreePortFast(client, prefer, cmdTO)
	if err != nil {
		return Result{}, err
	}

	needUpload := true
	remoteHash, _ := run(client, fmt.Sprintf(
		"sha256sum %s 2>/dev/null | awk '{print $1}'", remoteBin), cmdTO)
	if strings.TrimSpace(remoteHash) == wantHash {
		needUpload = false
	}

	if needUpload {
		emit("upload", fmt.Sprintf("upload agent gzip (~%d KB)…", len(gzipBytes())/1024))
		if err := uploadGzipBinary(client, remoteBin, cmdTO); err != nil {
			return Result{}, fmt.Errorf("upload: %w", err)
		}
	} else {
		emit("upload", "agent já no servidor (skip)")
	}

	emit("start", fmt.Sprintf("iniciar agent :%d…", port))
	// Stop old socks + previous agent.
	_, _ = run(client, "pkill -f '/sendsmtp-socks5d' 2>/dev/null || true; pkill -f '/sendsmtp-agent' 2>/dev/null || true; rm -f "+remoteOldSocks, 8*time.Second)

	startCmd := fmt.Sprintf(
		"nohup env AGENT_TOKEN=%s %s -addr ':%d' -token %s -conc 64 >/var/log/sendsmtp-agent.log 2>&1 & sleep 0.4; "+
			"curl -fsS --max-time 3 http://127.0.0.1:%d/health >/dev/null && echo OK || "+
			"((ss -lnt 2>/dev/null || netstat -lnt 2>/dev/null) | grep -q ':%d ' && echo OK || (tail -n 40 /var/log/sendsmtp-agent.log; echo FAIL))",
		shellQuote(token), remoteBin, port, shellQuote(token), port, port,
	)
	out, err := run(client, startCmd, cmdTO)
	if err != nil || !strings.Contains(out, "OK") {
		return Result{}, fmt.Errorf("start agent: %s", strings.TrimSpace(out))
	}

	_, _ = run(client, fmt.Sprintf(
		"(command -v ufw >/dev/null && ufw allow %d/tcp) || true", port), 8*time.Second)

	return Result{
		ProxyPort:     port,
		ProxyUser:     "agent",
		ProxyPassword: token,
		Message:       fmt.Sprintf("agent on %s:%d", srv.Host, port),
	}, nil
}

var gzipCache []byte

func gzipBytes() []byte {
	if gzipCache != nil {
		return gzipCache
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(agentBinary)
	_ = zw.Close()
	gzipCache = buf.Bytes()
	return gzipCache
}

func pickFreePortFast(client *ssh.Client, prefer int, timeout time.Duration) (int, error) {
	script := fmt.Sprintf(`
prefer=%d
for i in $(seq 0 40); do
  p=$((prefer+i))
  if ! (ss -lnt 2>/dev/null || netstat -lnt 2>/dev/null) | grep -q ":$p "; then
    echo $p
    exit 0
  fi
done
echo NONE
exit 1
`, prefer)
	out, err := run(client, script, timeout)
	out = strings.TrimSpace(out)
	if err != nil || out == "" || out == "NONE" {
		return 0, fmt.Errorf("no free port near %d", prefer)
	}
	lines := strings.Fields(out)
	p, err := strconv.Atoi(lines[len(lines)-1])
	if err != nil || p <= 0 {
		return 0, fmt.Errorf("no free port near %d (%q)", prefer, out)
	}
	return p, nil
}

func uploadGzipBinary(client *ssh.Client, remotePath string, timeout time.Duration) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf(
		"gzip -dc > %s.tmp && mv %s.tmp %s && chmod 755 %s",
		remotePath, remotePath, remotePath, remotePath,
	)
	errCh := make(chan error, 1)
	go func() { errCh <- session.Run(cmd) }()

	gz := gzipBytes()
	doneWrite := make(chan error, 1)
	go func() {
		_, werr := stdin.Write(gz)
		_ = stdin.Close()
		doneWrite <- werr
	}()

	select {
	case err := <-errCh:
		werr := <-doneWrite
		if err != nil {
			return err
		}
		return werr
	case <-time.After(timeout):
		_ = session.Close()
		return fmt.Errorf("upload timeout")
	}
}

func run(client *ssh.Client, cmd string, timeout time.Duration) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	done := make(chan error, 1)
	var buf strings.Builder
	session.Stdout = &buf
	session.Stderr = &buf
	go func() { done <- session.Run(cmd) }()

	select {
	case err := <-done:
		return buf.String(), err
	case <-time.After(timeout):
		_ = session.Close()
		return buf.String(), fmt.Errorf("command timeout")
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func randomPass(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
