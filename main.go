package iot

import (
	"context"
	"fmt"
	"log"
)

func main() {
	ctx := context.Background()
	service, err := NewService(ctx)
	if err != nil {
		log.Fatalln(err)
	}
	// device := Device{
	// 	Blocked:  true,
	// 	LogLevel: "NONE",
	// }
	getDevice := NewProjectsLocationsRegistriesDevicesService(service)
	call := getDevice.Get(fmt.Sprintf("projects/%s/locations/%s/registries/%s/devices/%s", "iot-core-test-363819", "us-central1", "andrewtest3", "andrewtest"))
	resp, err := call.Do()
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(resp.NumId)
	}
}
