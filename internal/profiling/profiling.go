package profiling

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
)

// Session escribe perfiles compatibles con go tool pprof sin exponer un servidor HTTP.
// Los archivos se crean de forma exclusiva para no sobrescribir evidencias anteriores.
type Session struct {
	cpu      *os.File
	heap     *os.File
	stopOnce sync.Once
	stopErr  error
}

func Start(cpuPath, heapPath string) (*Session, error) {
	session := &Session{}
	var err error
	if strings.TrimSpace(cpuPath) != "" {
		session.cpu, err = createExclusive(cpuPath)
		if err != nil {
			return nil, fmt.Errorf("INPUT_INVALID: crear perfil CPU: %w", err)
		}
	}
	if strings.TrimSpace(heapPath) != "" {
		session.heap, err = createExclusive(heapPath)
		if err != nil {
			session.cleanup()
			return nil, fmt.Errorf("INPUT_INVALID: crear perfil heap: %w", err)
		}
	}
	if session.cpu != nil {
		if err := pprof.StartCPUProfile(session.cpu); err != nil {
			session.cleanup()
			return nil, fmt.Errorf("ENGINE_FAILED: iniciar perfil CPU: %w", err)
		}
	}
	return session, nil
}

func (s *Session) Stop() error {
	if s == nil {
		return nil
	}
	s.stopOnce.Do(func() {
		var failures []error
		if s.cpu != nil {
			pprof.StopCPUProfile()
			if err := s.cpu.Sync(); err != nil {
				failures = append(failures, err)
			}
			if err := s.cpu.Close(); err != nil {
				failures = append(failures, err)
			}
		}
		if s.heap != nil {
			runtime.GC()
			if err := pprof.WriteHeapProfile(s.heap); err != nil {
				failures = append(failures, err)
			}
			if err := s.heap.Sync(); err != nil {
				failures = append(failures, err)
			}
			if err := s.heap.Close(); err != nil {
				failures = append(failures, err)
			}
		}
		s.stopErr = errors.Join(failures...)
	})
	return s.stopErr
}

func createExclusive(path string) (*os.File, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(parent); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("el directorio del perfil debe ser un directorio regular")
	}
	return os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
}

func (s *Session) cleanup() {
	for _, file := range []*os.File{s.cpu, s.heap} {
		if file == nil {
			continue
		}
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
	}
}
