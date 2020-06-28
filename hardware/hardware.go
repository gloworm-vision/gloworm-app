package hardware

import "io"

// New creates a hardware interface from the given configuration. This hardware
// may or may not implement any functionality at all, see the Hardware interface
// documentation for more details.
func New(c Config) (Hardware, error) {
	if c.Gloworm != nil {
		return NewGloworm(*c.Gloworm)
	}

	// no hardware is valid hardware
	return nil, nil
}

// Config holds configuration information for all of the supported gloworm-app
// hardware. No more than one config should be specified (not null), but it is
// valid for no config to be specified at all.
type Config struct {
	Gloworm *GlowormConfig
}

// Hardware defines a common interface for hardware gloworm-app can run on.
// Because not all hardware has status LEDs, or LED cluster brightness control,
// or even an LED cluster at all, this interface is just a closer and only specified
// for documentation purposes. When specific hardware functionality is required
// you should type assert to a more specific interface that describes that
// functionality. For example, you can attempt an assertion to the BinaryLight
// interface if you need binary control of the hardware LED cluster.
type Hardware interface {
	io.Closer
}

// BinaryLight describes hardware with an LED cluster that can be toggled on/off
type BinaryLight interface {
	// SetLights turns the LED cluster on or off
	SetLights(on bool) error
}

// DimmableLight describes hardware with an LED cluster that can be dimmed
type DimmableLight interface {
	// SetLightBrightness sets the LED cluster brightness (from off - 0, to fully on - 1)
	SetLightBrightness(v float64) error
}

// Status defines a list of statuses that can be indicated in various ways by different
// hardware
type Status int

const (
	// TargetAcquired is true when gloworm-app is tracking a contour and sending it's
	// location over network tables
	TargetAquired Status = iota
)

type ErrUnsupportedStatus struct {
	error
}

func (err ErrUnsupportedStatus) Is(target error) bool {
	_, ok := target.(ErrUnsupportedStatus)
	return ok
}

// StatusIndicators describes hardware with one or more status indicators
type StatusIndicators interface {
	// SetStatus sets a status on or off. If the underlying hardware can't indicate this
	// status, it should return an ErrUnsupportedStatus error.
	SetStatus(status Status, value bool) error
}
