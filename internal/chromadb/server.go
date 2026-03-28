package chromadb

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ServerConfig holds configuration for the embedded Chroma server process.
type ServerConfig struct {
	Port    int
	DataDir string // where Chroma stores its data
	LogDir  string // where to write chroma.log
}

// DefaultServerConfig returns a ServerConfig using ~/.gt/chroma as data dir.
func DefaultServerConfig() ServerConfig {
	home, _ := os.UserHomeDir()
	return ServerConfig{
		Port:    8000,
		DataDir: filepath.Join(home, ".gt", "chroma"),
		LogDir:  filepath.Join(home, ".gt", "logs"),
	}
}

// Server manages the lifecycle of the Chroma subprocess.
type Server struct {
	cfg ServerConfig
	cmd *exec.Cmd
	log *log.Logger
}

// NewServer creates a new Chroma server manager.
func NewServer(cfg ServerConfig, logger *log.Logger) *Server {
	return &Server{cfg: cfg, log: logger}
}

// Start launches the Chroma server process. Non-blocking.
func (s *Server) Start(ctx context.Context) error {
	if err := os.MkdirAll(s.cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("chroma: create data dir: %w", err)
	}
	if err := os.MkdirAll(s.cfg.LogDir, 0755); err != nil {
		return fmt.Errorf("chroma: create log dir: %w", err)
	}

	// Check if chroma CLI is available.
	if _, err := exec.LookPath("chroma"); err != nil {
		return fmt.Errorf("chroma binary not found in PATH — install with: pip install chromadb")
	}

	logFile, err := os.OpenFile(
		filepath.Join(s.cfg.LogDir, "chroma.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644,
	)
	if err != nil {
		return fmt.Errorf("chroma: open log: %w", err)
	}

	s.cmd = exec.CommandContext(ctx, "chroma", "run",
		"--path", s.cfg.DataDir,
		"--port", fmt.Sprintf("%d", s.cfg.Port),
	)
	s.cmd.Stdout = logFile
	s.cmd.Stderr = logFile

	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("chroma: start: %w", err)
	}

	s.log.Printf("Chroma server started (pid=%d, port=%d, data=%s)", s.cmd.Process.Pid, s.cfg.Port, s.cfg.DataDir)

	// Wait for server to be ready (max 10s).
	client := NewClient(fmt.Sprintf("http://localhost:%d", s.cfg.Port))
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := client.Ping(); err == nil {
			s.log.Printf("Chroma server ready")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("chroma: server did not become ready within 10s")
}

// Stop terminates the Chroma server process.
func (s *Server) Stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
		s.log.Printf("Chroma server stopped")
	}
}

// BaseURL returns the Chroma server base URL.
func (s *Server) BaseURL() string {
	return fmt.Sprintf("http://localhost:%d", s.cfg.Port)
}
