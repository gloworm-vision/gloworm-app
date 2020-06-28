package store

import (
	"io"

	"github.com/gloworm-vision/gloworm-app/hardware"
	"github.com/gloworm-vision/gloworm-app/pipeline"
)

// Store describes a persistent storage engine for gloworm-app information.
type Store interface {
	PipelineConfig(name string) (pipeline.Config, error)
	ListPipelineConfigs() ([]string, error)
	PutPipelineConfig(name string, p pipeline.Config) error

	DefaultPipelineConfig() (string, error)
	PutDefaultPipelineConfig(name string) error

	HardwareConfig() (hardware.Config, error)
	PutHardwareConfig(h hardware.Config) error

	io.Closer
}
