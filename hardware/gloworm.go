package hardware

import (
	"fmt"

	"github.com/gloworm-vision/gloworm-app/hardware/gpio"
)

type Gloworm struct {
	gpio         gpio.GPIO
	pwmFrequency int
}

func NewGloworm(pigpioAddr string, pwmFrequency int) (Hardware, error) {
	g, err := gpio.DialPigpio(pigpioAddr)
	if err != nil {
		return nil, fmt.Errorf("unable to dial pigpio to setup gpio: %w", err)
	}

	return &Gloworm{
		gpio:         g,
		pwmFrequency: pwmFrequency,
	}, nil
}

func (g *Gloworm) Name() string {
	return "Gloworm"
}

func (g *Gloworm) SetLights(on bool) error {
	if err := g.gpio.Write(13, gpio.High); err != nil {
		return fmt.Errorf("can't turn on left LED cluster: %w", err)
	}

	if err := g.gpio.Write(18, gpio.High); err != nil {
		return fmt.Errorf("can't turn on right LED cluster: %w", err)
	}

	return nil
}

func (g *Gloworm) SetLightBrightness(v float64) error {
	if err := g.gpio.PWM(13, g.pwmFrequency, v); err != nil {
		return fmt.Errorf("can't set left LED cluster brightness: %w", err)
	}

	if err := g.gpio.PWM(18, g.pwmFrequency, v); err != nil {
		return fmt.Errorf("can't set left LED cluster brightness: %w", err)
	}

	return nil
}

func (g *Gloworm) SetStatus(status Status, value bool) error {
	switch status {
	case TargetAquired:
		if err := g.gpio.Write(4, gpio.Level(value)); err != nil {
			return fmt.Errorf("can't set LED A high: %w", err)
		}
	default:
		return ErrUnsupportedStatus{fmt.Errorf("status %q not implemented by Gloworm", status)}
	}

	return nil
}
