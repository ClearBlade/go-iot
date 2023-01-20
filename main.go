package main

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
	device := Device{
		Blocked:  true,
		LogLevel: "NONE",
	}
	createDevice := NewProjectsLocationsRegistriesDevicesService(service)
	call := createDevice.Patch(fmt.Sprintf("projects/%s/locations/%s/registries/%s/devices/%s", "iot-core-test-363819", "us-central1", "andrewtest2", "go-sdk-patch-test"), &device)
	call.UpdateMask("blocked,logLevel,metadata")
	call.Do()
}
