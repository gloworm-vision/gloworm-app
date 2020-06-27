package gpio

// Level describes the binary state of a GPIO pin: either LOW or HIGH.
type Level bool

const (
	Low  Level = false
	High Level = true
)

type GPIO interface {
	// Write sets a pin to LOW or HIGH
	Write(pin int, level Level) error

	// PWM sets the frequency and duty cycle (0 - 1) for a given pin.
	PWM(pin int, frequency int, duty float64) error
}
