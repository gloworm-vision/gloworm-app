package main

import (
	"time"

	"github.com/gloworm-vision/gloworm-app/hardware"
)

func main() {
	gloworm, err := hardware.NewGloworm("localhost:8888", 30000)
	if err != nil {
		panic(err)
	}

	for {
		for i := 0.0; i <= 1; i += 0.01 {
			gloworm.(hardware.DimmableLight).SetLightBrightness(i)

			time.Sleep(time.Millisecond * 10)
		}

		for i := 1.0; i >= 0; i -= 0.01 {
			gloworm.(hardware.DimmableLight).SetLightBrightness(i)

			time.Sleep(time.Millisecond * 10)
		}
	}
}