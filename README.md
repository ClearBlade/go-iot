# Cloud IoT API

ClearBlade IoT Core API's Go client library
[![Go Reference](https://pkg.go.dev/badge/google.golang.org/api.svg)](https://pkg.go.dev/github.com/clearblade/go-iot)

## Setting up service account credentials

[Create a service account](https://clearblade.atlassian.net/wiki/spaces/IC/pages/2240675843/Add+service+accounts+to+a+project) in your project and download the credentials .json file.

To authenticate API requests you can use one of two options:

1. Define an environment variable named `CLEARBLADE_CONFIGURATION` which represents the path to the credentials .json file.
   Example:

   ```
   export CLEARBLADE_CONFIGURATION=/path/to/file.json
   ```

2. Pass in credentials via the Service options:
   ```
   credentials := `{"systemKey":"systemkeyhere","token":"tokenhere","url":"https://iot.clearblade.com","project":"test"}`

   service, err := iot.NewService(ctx, iot.WithServiceAccountCredentials(credentials))
   if err != nil {
     return err
   }
   ```

## Authorization

See the [Authorization](https://clearblade.atlassian.net/wiki/spaces/IC/pages/2240675843/Add+service+accounts+to+a+project)
documentation for authorizing SDK with ClearBlade iot-core.

## API Documentation

View the Go sections of these [Samples](https://clearblade.atlassian.net/wiki/spaces/IC/pages/2310897676/Samples).
