package main

import (
	"context"
	"log"

	"github.com/gloworm-vision/gloworm-app/internal/server"
	"github.com/gloworm-vision/gloworm-app/pipeline"
	"github.com/sirupsen/logrus"
	"gocv.io/x/gocv"
)

func main() {
	pipelineConfig := pipeline.Config{
		MinThresh:  pipeline.HSV{H: 28, S: 70, V: 90},
		MaxThresh:  pipeline.HSV{H: 38, S: 255, V: 255},
		MinContour: 0.01,
		MaxContour: 0.5,
	}

	pipeline := pipeline.New(pipelineConfig)

	webcam, err := gocv.OpenVideoCapture(0)
	if err != nil {
		panic(err)
	}
	defer webcam.Close()

	server := server.Server{Addr: ":8080", Capture: webcam, Pipeline: pipeline, Logger: logrus.New()}

	if err := server.Run(context.Background()); err != nil {
		log.Println(err)
		return
	}
}
