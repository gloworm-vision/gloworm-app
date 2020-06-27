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
		MinThresh:      pipeline.HSV{H: 5, S: 100, V: 0},
		MaxThresh:      pipeline.HSV{H: 30, S: 255, V: 255},
		MinContourArea: 10,
		MaxContourArea: 100,
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
