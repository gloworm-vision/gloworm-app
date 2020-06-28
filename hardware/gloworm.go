package hardware

import (
	"fmt"

	"github.com/gloworm-vision/gloworm-app/hardware/gpio"
)

type GlowormConfig struct {
	PigpioAddr   string
	PWMFrequency int
}

type Gloworm struct {
	gpio         gpio.GPIO
	pwmFrequency int
}

func NewGloworm(config GlowormConfig) (Hardware, error) {
	g, err := gpio.DialPigpio(config.PigpioAddr)
	if err != nil {
		return nil, fmt.Errorf("unable to dial pigpio to setup gpio: %w", err)
	}

	return &Gloworm{
		gpio:         g,
		pwmFrequency: config.PWMFrequency,
	}, nil
}

const (
	glowormLeftCluster  = 13
	glowormRightCluster = 18
	glowormGreenStatus  = 4
)

func (g *Gloworm) SetLights(on bool) error {
	if err := g.gpio.Write(glowormLeftCluster, gpio.High); err != nil {
		return fmt.Errorf("can't turn on left LED cluster: %w", err)
	}

	if err := g.gpio.Write(glowormRightCluster, gpio.High); err != nil {
		return fmt.Errorf("can't turn on right LED cluster: %w", err)
	}

	return nil
}

func (g *Gloworm) SetLightBrightness(v float64) error {
	if err := g.gpio.PWM(glowormLeftCluster, g.pwmFrequency, v); err != nil {
		return fmt.Errorf("can't set left LED cluster brightness: %w", err)
	}

	if err := g.gpio.PWM(glowormRightCluster, g.pwmFrequency, v); err != nil {
		return fmt.Errorf("can't set left LED cluster brightness: %w", err)
	}

	return nil
}

func (g *Gloworm) SetStatus(status Status, value bool) error {
	switch status {
	case TargetAquired:
		if err := g.gpio.Write(glowormGreenStatus, gpio.Level(value)); err != nil {
			return fmt.Errorf("can't set LED A high: %w", err)
		}
	default:
		return ErrUnsupportedStatus{fmt.Errorf("status %q not implemented by Gloworm", status)}
	}

	return nil
}

func (g *Gloworm) Close() error {
	if err := g.gpio.Write(glowormLeftCluster, gpio.Low); err != nil {
		return fmt.Errorf("unable to turn off left cluster: %w", err)
	}
	if err := g.gpio.Write(glowormRightCluster, gpio.Low); err != nil {
		return fmt.Errorf("unable to turn off right cluster: %w", err)
	}
	if err := g.gpio.Write(glowormGreenStatus, gpio.Low); err != nil {
		return fmt.Errorf("unable to turn off green status LED: %w", err)
	}

	return g.gpio.Close()
}
