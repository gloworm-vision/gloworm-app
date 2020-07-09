package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gloworm-vision/gloworm-app/networktables"
	"github.com/sirupsen/logrus"
)

func main() {
	client := networktables.Client{Logger: logrus.New()}

	if err := client.Open(context.Background()); err != nil {
		panic(err)
	}
	defer client.Close()

	for i := 0.0; i < 100; i++ {
		fmt.Println(client.Ping())
		time.Sleep(time.Second)
	}

	http.ListenAndServe(":8080", nil)
}
