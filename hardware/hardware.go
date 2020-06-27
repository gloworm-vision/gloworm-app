package hardware

// Hardware defines a common interface for hardware gloworm-app can run on
//
// Because not all hardware has status LEDs, or LED cluster brightness control,
// or even an LED cluster at all, this is a fairly minimal interface. Most of the
// time this interface should be type asserted to a more specific interface. For
// example, you can assert to the BinaryLight interface for binary LED cluster control,
// or the DimmableLight interface for dimmable LED cluster control.
type Hardware interface {
	Name() string
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
