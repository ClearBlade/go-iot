# Cloud IoT API

ClearBlade IoT Core API's Go client library
[![Go Reference](https://pkg.go.dev/badge/google.golang.org/api.svg)](https://pkg.go.dev/github.com/clearblade/go-iot)

## Setting up service account credentials

[Create a service account](https://clearblade.atlassian.net/wiki/spaces/IC/pages/2240675843/Add+service+accounts+to+a+project) in your project and download the credentials .json file. Define environment variable `CLEARBLADE_CONFIGURATION`, representing the credentials .json file's path. Example:

```
export CLEARBLADE_CONFIGURATION=/path/to/file.json
```

## Authorization

See the [Authorization](https://clearblade.atlassian.net/wiki/spaces/IC/pages/2240675843/Add+service+accounts+to+a+project)
documentation for authorizing SDK with ClearBlade iot-core.

## API Documentation

View the Go sections of these [Samples](https://clearblade.atlassian.net/wiki/spaces/IC/pages/2310897676/Samples).
