package main

import (
	"context"

	"github.com/gloworm-vision/gloworm-app/server"
	"github.com/gloworm-vision/gloworm-app/store"
	"github.com/sirupsen/logrus"
	"gocv.io/x/gocv"
)

func main() {
	webcam, err := gocv.OpenVideoCapture(0)
	if err != nil {
		panic(err)
	}
	defer webcam.Close()

	store, err := store.OpenBBolt("store.db", 0666, nil)
	if err != nil {
		panic(err)
	}

	server := server.Server{Addr: ":8080", Store: store, Capture: webcam, Logger: logrus.New()}

	if err := server.Run(context.Background()); err != nil {
		panic(err)
	}
}
