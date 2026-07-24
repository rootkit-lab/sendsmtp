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

//go:embed embed/socks5d
var socks5dBinary []byte

const remoteBin = "/usr/local/bin/sendsmtp-socks5d"

// Result is the outcome of a SOCKS5 deploy on a remote host.
type Result struct {
	ProxyPort     int    `json:"proxy_port"`
	ProxyUser     string `json:"proxy_user"`
	ProxyPassword string `json:"proxy_password"`
	Message       string `json:"message"`
}

// ProgressFunc reports deploy phase (ssh|upload|start).
type ProgressFunc func(phase, message string)

// Deploy uploads socks5d over SSH and starts it on a free port near PreferPort.
func Deploy(srv store.Server, timeout time.Duration) (Result, error) {
	return DeployWithProgress(srv, timeout, nil)
}

// DeployWithProgress is Deploy with optional phase callbacks.
func DeployWithProgress(srv store.Server, timeout time.Duration, onProgress ProgressFunc) (Result, error) {
	if len(socks5dBinary) < 1000 {
		return Result{}, fmt.Errorf("embedded socks5d binary missing — rebuild with GOOS=linux")
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
		prefer = 10808
	}
	user := srv.ProxyUser
	if user == "" {
		user = "sendsmtp"
	}
	pass := srv.ProxyPassword
	if pass == "" {
		pass = randomPass(16)
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

	sum := sha256.Sum256(socks5dBinary)
	wantHash := hex.EncodeToString(sum[:])

	emit("port", fmt.Sprintf("porta livre perto de %d…", prefer))
	port, err := pickFreePortFast(client, prefer, cmdTO)
	if err != nil {
		return Result{}, err
	}

	// Skip upload when remote binary already matches.
	needUpload := true
	remoteHash, _ := run(client, fmt.Sprintf(
		"sha256sum %s 2>/dev/null | awk '{print $1}'", remoteBin), cmdTO)
	if strings.TrimSpace(remoteHash) == wantHash {
		needUpload = false
	}

	if needUpload {
		emit("upload", fmt.Sprintf("upload gzip (~%d KB)…", len(gzipBytes())/1024))
		if err := uploadGzipBinary(client, socks5dBinary, remoteBin, cmdTO); err != nil {
			return Result{}, fmt.Errorf("upload: %w", err)
		}
	} else {
		emit("upload", "binário já no servidor (skip)")
	}

	emit("start", fmt.Sprintf("iniciar SOCKS :%d…", port))
	_, _ = run(client, "pkill -f '/sendsmtp-socks5d' 2>/dev/null || true", 8*time.Second)
	startCmd := fmt.Sprintf(
		"nohup %s -addr ':%d' -user %s -pass %s >/var/log/sendsmtp-socks5d.log 2>&1 & sleep 0.3; "+
			"(ss -lnt 2>/dev/null | grep -q ':%d ' || netstat -lnt 2>/dev/null | grep -q ':%d ') && echo OK || (tail -n 30 /var/log/sendsmtp-socks5d.log; echo FAIL)",
		remoteBin, port, shellQuote(user), shellQuote(pass), port, port,
	)
	out, err := run(client, startCmd, cmdTO)
	if err != nil || !strings.Contains(out, "OK") {
		return Result{}, fmt.Errorf("start socks5d: %s", strings.TrimSpace(out))
	}

	_, _ = run(client, fmt.Sprintf(
		"(command -v ufw >/dev/null && ufw allow %d/tcp) || true", port), 8*time.Second)

	return Result{
		ProxyPort:     port,
		ProxyUser:     user,
		ProxyPassword: pass,
		Message:       fmt.Sprintf("SOCKS5 on %s:%d", srv.Host, port),
	}, nil
}

var gzipCache []byte

func gzipBytes() []byte {
	if gzipCache != nil {
		return gzipCache
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(socks5dBinary)
	_ = zw.Close()
	gzipCache = buf.Bytes()
	return gzipCache
}

func pickFreePortFast(client *ssh.Client, prefer int, timeout time.Duration) (int, error) {
	// One remote shell: scan prefer..prefer+40 for a free listen port.
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
	// Last line in case of noise.
	lines := strings.Fields(out)
	p, err := strconv.Atoi(lines[len(lines)-1])
	if err != nil || p <= 0 {
		return 0, fmt.Errorf("no free port near %d (%q)", prefer, out)
	}
	return p, nil
}

func uploadGzipBinary(client *ssh.Client, data []byte, remotePath string, timeout time.Duration) error {
	_ = data
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
