package main

import (
	"fmt"
	"math"

	"github.com/gloworm-vision/gloworm-app/networktables"
	"github.com/sirupsen/logrus"
)

func main() {
	client := networktables.Client{Logger: logrus.New()}
	defer client.Close()

	client.Ping()

	// fmt.Println(client.Create(networktables.Entry{
	// 	Name: "x",
	// 	Options: networktables.EntryOptions{
	// 		Persist: true,
	// 	},
	// 	Value: networktables.EntryValue{
	// 		EntryType: networktables.Double,
	// 		Double:    3.14,
	// 	},
	// }))
	client.UpdateValue("x", networktables.EntryValue{
		EntryType: networktables.Double,
		Double:    math.E,
	})

	fmt.Println(client.Get("x"))
	fmt.Println(client.Get("x"))
}
