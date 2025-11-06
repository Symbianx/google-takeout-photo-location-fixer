# Google Takeout Photo Location Fixer

This Go tool uses the Google Takeout location information to add GPS data to pictures without without a location.

It works by looking at the time the picture was taken and using the location data to determine your approximate location at the time.


## Requirements

You must download and install Phil Harvey's exiftool binary from https://exiftool.org/.


## Running the tool

Download the tool from the [latest GitHub release](https://github.com/Symbianx/google-takeout-photo-location-fixer/releases).

This command will run the tool process all `.jpg` files in the `sample_data` directory using the location history in `./sample_data/Location\ History/Records.json`:
```shell
google-takeout-photo-location-fixer -d ./sample_data -f ./sample_data/Location\ History/Records.json
```

If you get an error about not finding the exiftool in the $PATH, you can use the `--exiftool-binary` argument to pass it's location:
```shell
google-takeout-photo-location-fixer  --exiftool-binary /home/Symbianx/Downloads/exiftool -d ./sample_data -f ./sample_data/Location\ History/Records.json
```

To get a list of all available options run:
```shell
google-takeout-photo-location-fixer --help
```


## Development

### Running Tests

The project includes both unit tests and end-to-end (e2e) tests.

To run all tests:
```shell
go test -v ./...
```

To run only unit tests:
```shell
go test -v -run "Test(ReadLocations|GetUnsignedDateDifference|FindLocationFromDate|LocationLessFunc)"
```

To run only e2e tests (requires exiftool to be installed):
```shell
go test -v -run "TestE2E"
```

**Note:** E2E tests require exiftool to be installed and available in your PATH.

### Building from Source

```shell
go build -v
```

