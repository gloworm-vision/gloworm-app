package server

import (
	"fmt"
	"sync"

	"github.com/gloworm-vision/gloworm-app/hardware"
	"github.com/gloworm-vision/gloworm-app/pipeline"
)

// pipelineManager synchronizes access to the underlying pipeline.
type pipelineManager struct {
	pipeline *pipeline.Pipeline
	mu       *sync.RWMutex
}

func (p *pipelineManager) SetConfig(config pipeline.Config) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.pipeline = &pipeline.Pipeline{Config: config}
}

func (p *pipelineManager) Pipeline() *pipeline.Pipeline {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.pipeline
}

// hardwareManager synchronizes access to the underlying hardware. This is a little more
// complicated than synchronizing the pipeline since we need to close hardware (that is,
// we can't be passing out hardware and then close it while a caller might be using it).
type hardwareManager struct {
	hardware hardware.Hardware
	mu       *sync.RWMutex
}

func (h *hardwareManager) Update(config hardware.Config) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.hardware.Close()

	var err error
	h.hardware, err = hardware.New(config)
	if err != nil {
		return fmt.Errorf("unable to create new hardware from config: %w", err)
	}

	return nil
}

func (h *hardwareManager) View(fn func(h hardware.Hardware)) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	fn(h.hardware)
}
