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
	owned   bool
	done    chan error
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
	if s.ready() {
		s.running = true
		s.owned = false
		return nil
	}

	s.cmd = exec.Command("ollama", "serve")
	s.cmd.Env = append(s.cmd.Environ(), fmt.Sprintf("OLLAMA_HOST=:%d", s.port))
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ollama serve: %w", err)
	}

	s.running = true
	s.owned = true
	s.done = make(chan error, 1)
	go func() {
		s.done <- s.cmd.Wait()
	}()
	return nil
}

// Stop terminates the ollama serve process
func (s *Server) Stop() error {
	if !s.running {
		return nil
	}
	if !s.owned {
		s.running = false
		return nil
	}
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	// Send SIGTERM to the process group
	pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGTERM)
	} else {
		s.cmd.Process.Signal(syscall.SIGTERM)
	}

	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		// Force kill if it doesn't exit gracefully
		if pgid, err := syscall.Getpgid(s.cmd.Process.Pid); err == nil {
			syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			s.cmd.Process.Kill()
		}
		<-s.done
	}

	s.running = false
	return nil
}

// WaitReady polls the health endpoint until the server is ready
func (s *Server) WaitReady() error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s.ready() {
			return nil
		}
		if s.owned {
			select {
			case err := <-s.done:
				s.running = false
				if err == nil {
					return fmt.Errorf("ollama server exited before becoming ready")
				}
				return fmt.Errorf("ollama server exited before becoming ready: %w", err)
			default:
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

func (s *Server) ready() bool {
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get(fmt.Sprintf("%s/api/tags", s.Host()))
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}
