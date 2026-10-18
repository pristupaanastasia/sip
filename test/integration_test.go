package test

import (
	"bytes"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test builds and runs binaries")
	}

	bin := buildBinaries(t)
	stack := startStack(t, bin)

	tests := map[string]struct {
		request      string
		wantExitCode int
		wantOutput   []string
		minDuration  time.Duration
	}{
		"INVITE from testdata gets 100 Trying and 200 OK": {
			request:      readFile(t, "../testdata/invite.txt"),
			wantExitCode: 0,
			wantOutput: []string{
				"Received response:\nSIP/2.0 100 Trying\n",
				"Received response:\nSIP/2.0 200 OK\n",
				"'Content-Type': 'application/sdp'",
			},
			minDuration: time.Second,
		},
		"request file saved with LF line endings": {
			request:      strings.ReplaceAll(readFile(t, "../testdata/invite.txt"), "\r\n", "\n"),
			wantExitCode: 0,
			wantOutput:   []string{"Received response:\nSIP/2.0 200 OK\n"},
			minDuration:  time.Second,
		},
		"Max-Forwards 0 is rejected by the proxy": {
			request: "OPTIONS sip:bob@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP client.example.com;branch=z9hG4bKmf0\r\n" +
				"Max-Forwards: 0\r\n" +
				"From: <sip:alice@example.com>;tag=1\r\n" +
				"To: <sip:bob@example.com>\r\n" +
				"Call-ID: max-forwards-zero\r\n" +
				"CSeq: 1 OPTIONS\r\n" +
				"\r\n",
			wantExitCode: 1,
			wantOutput:   []string{"Received response:\nSIP/2.0 483 Too Many Hops\n"},
		},
		"invalid request is rejected by the proxy with 400": {
			request: "OPTIONS sip:bob@example.com SIP/2.0\r\n" +
				"Via: SIP/2.0/UDP client.example.com;branch=z9hG4bKbadcseq\r\n" +
				"Max-Forwards: 70\r\n" +
				"From: <sip:alice@example.com>;tag=1\r\n" +
				"To: <sip:bob@example.com>\r\n" +
				"Call-ID: cseq-mismatch\r\n" +
				"CSeq: 1 INVITE\r\n" +
				"\r\n",
			wantExitCode: 1,
			wantOutput:   []string{"Received response:\nSIP/2.0 400 Bad Request\n"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			requestFile := filepath.Join(t.TempDir(), "request.txt")
			if writeErr := os.WriteFile(requestFile, []byte(tc.request), 0o600); writeErr != nil {
				t.Fatal(writeErr)
			}

			started := time.Now()
			output, exitCode := runClient(t, filepath.Join(bin, "client"),
				"-proxy", stack.proxyAddr,
				"-request", requestFile,
				"-timeout", "3s",
			)
			elapsed := time.Since(started)

			if exitCode != tc.wantExitCode {
				t.Errorf("client exit code = %d, want %d\n%s", exitCode, tc.wantExitCode, output)
			}
			for _, want := range tc.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("client output does not contain %q\n%s", want, output)
				}
			}
			if elapsed < tc.minDuration {
				t.Errorf("client finished after %s, want at least %s", elapsed, tc.minDuration)
			}
			if trying := strings.Count(output, "SIP/2.0 100 Trying"); trying > 1 {
				t.Errorf("client received %d 100 Trying responses, want at most 1 (server's 100 must not be forwarded)", trying)
			}
		})
	}

	serverLog, proxyLog := stack.logs(t)

	for _, want := range []string{
		"Received request:\nsip:bob@example.com\n",
		"Received response:\nSIP/2.0 100 Trying\n",
		"Received response:\nSIP/2.0 200 OK\n",
		"'Via': 'SIP/2.0/UDP " + stack.viaHost + ";branch=z9hG4bK",
	} {
		if !strings.Contains(proxyLog, want) {
			t.Errorf("proxy log does not contain %q\n%s", want, proxyLog)
		}
	}
	for _, want := range []string{
		"Received request:\nsip:bob@example.com\n",
		"'Max-Forwards': '69'",
	} {
		if !strings.Contains(serverLog, want) {
			t.Errorf("server log does not contain %q\n%s", want, serverLog)
		}
	}
	for _, rejectedCallID := range []string{"max-forwards-zero", "cseq-mismatch"} {
		if strings.Contains(serverLog, rejectedCallID) {
			t.Errorf("rejected request %s reached the server\n%s", rejectedCallID, serverLog)
		}
	}
}

type stack struct {
	proxyAddr string
	viaHost   string
	logs      func(t *testing.T) (serverLog, proxyLog string)
}

func startStack(t *testing.T, bin string) stack {
	t.Helper()
	if os.Getenv("SIP_COMPOSE") != "" {
		return composeStack(t)
	}
	return localStack(t, bin)
}

func localStack(t *testing.T, bin string) stack {
	t.Helper()
	serverAddr := freeAddr(t)
	proxyAddr := freeAddr(t)

	server := start(t, filepath.Join(bin, "server"),
		"-listen", serverAddr,
		"-response", "../testdata/200ok.txt",
	)
	proxy := start(t, filepath.Join(bin, "proxy"),
		"-listen", proxyAddr,
		"-upstream", serverAddr,
		"-via-host", proxyAddr,
	)

	return stack{
		proxyAddr: proxyAddr,
		viaHost:   proxyAddr,
		logs: func(t *testing.T) (string, string) {
			return stop(t, server), stop(t, proxy)
		},
	}
}

func composeStack(t *testing.T) stack {
	t.Helper()
	for _, service := range []string{"server", "proxy"} {
		deadline := time.Now().Add(30 * time.Second)
		for !strings.Contains(composeLogs(t, service), "msg=listening") {
			if time.Now().After(deadline) {
				t.Fatalf("compose service %s did not start listening:\n%s", service, composeLogs(t, service))
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	return stack{
		proxyAddr: "127.0.0.1:5060",
		viaHost:   "proxy:5060",
		logs: func(t *testing.T) (string, string) {
			return composeLogs(t, "server"), composeLogs(t, "proxy")
		},
	}
}

func composeLogs(t *testing.T, service string) string {
	t.Helper()
	output, logsErr := exec.Command("docker", "compose", "-f", "../docker-compose.yml",
		"logs", "--no-color", "--no-log-prefix", service).CombinedOutput()
	if logsErr != nil {
		t.Fatalf("docker compose logs %s: %v\n%s", service, logsErr, output)
	}
	return string(output)
}

type process struct {
	cmd *exec.Cmd
	out *syncBuffer
}

func start(t *testing.T, path string, args ...string) *process {
	t.Helper()
	out := &syncBuffer{}
	cmd := exec.Command(path, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	if startErr := cmd.Start(); startErr != nil {
		t.Fatalf("start %s: %v", path, startErr)
	}
	p := &process{cmd: cmd, out: out}
	t.Cleanup(func() {
		if p.cmd.ProcessState == nil {
			_ = p.cmd.Process.Kill()
			_ = p.cmd.Wait()
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "msg=listening") {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not start listening:\n%s", path, out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	return p
}

func stop(t *testing.T, p *process) string {
	t.Helper()
	if signalErr := p.cmd.Process.Signal(syscall.SIGTERM); signalErr != nil {
		t.Fatalf("send SIGTERM: %v", signalErr)
	}

	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case waitErr := <-done:
		if waitErr != nil {
			t.Errorf("process did not exit cleanly: %v\n%s", waitErr, p.out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("process did not stop after SIGTERM:\n%s", p.out.String())
	}

	log := p.out.String()
	if !strings.Contains(log, "msg=stopped") {
		t.Errorf("process log does not contain the stop message:\n%s", log)
	}
	return log
}

func runClient(t *testing.T, path string, args ...string) (string, int) {
	t.Helper()
	output, runErr := exec.Command(path, args...).CombinedOutput()
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		return string(output), 0
	case errors.As(runErr, &exitErr):
		return string(output), exitErr.ExitCode()
	default:
		t.Fatalf("run client: %v", runErr)
		return "", 0
	}
}

func buildBinaries(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	build := exec.Command("go", "build", "-o", dir+string(os.PathSeparator), "../cmd/...")
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		t.Fatalf("go build: %v\n%s", buildErr, output)
	}
	return dir
}

func freeAddr(t *testing.T) string {
	t.Helper()
	conn, listenErr := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if listenErr != nil {
		t.Fatal(listenErr)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
