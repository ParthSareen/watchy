package ollama

import (
	"fmt"
	"net/http"
	"os/exec"
	"syscall"
	"time"
)

// Server manages a dedicated Ollama server instance
type Server struct {
	cmd     *exec.Cmd
	port    int
	running bool
}

// NewServer creates a new Ollama server manager for the given port
func NewServer(port int) *Server {
	return &Server{
		port: port,
	}
}

// Start launches the ollama serve process
func (s *Server) Start() error {
	if s.running {
		return nil
	}

	s.cmd = exec.Command("ollama", "serve")
	s.cmd.Env = append(s.cmd.Environ(), fmt.Sprintf("OLLAMA_HOST=:%d", s.port))
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ollama serve: %w", err)
	}

	s.running = true
	return nil
}

// Stop terminates the ollama serve process
func (s *Server) Stop() error {
	if !s.running || s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	// Send SIGTERM to the process group
	pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)
	} else {
		s.cmd.Process.Signal(syscall.SIGTERM)
	}

	// Wait for the process to exit
	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// Force kill if it doesn't exit gracefully
		if pgid, err := syscall.Getpgid(s.cmd.Process.Pid); err == nil {
			syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			s.cmd.Process.Kill()
		}
		<-done
	}

	s.running = false
	return nil
}

// WaitReady polls the health endpoint until the server is ready
func (s *Server) WaitReady() error {
	client := &http.Client{Timeout: 1 * time.Second}
	url := fmt.Sprintf("http://localhost:%d/api/tags", s.port)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("ollama server not ready after 10 seconds")
}

// Host returns the base URL for the Ollama server
func (s *Server) Host() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}
