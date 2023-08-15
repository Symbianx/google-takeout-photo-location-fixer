# Google Takeout Photo Location Fixer

This Go script uses the Google Takeout location information to add GPS data to pictures without without a location.

It works by looking at the time the picture was taken and using the location data to determine your location at the time.


### Requirements

You must download Phil Harvey's exiftool binary from https://exiftool.org/.

Right now, there's no binary published so you also need to build the tool from source. For that you need to [download and install go](https://go.dev/doc/install)


### Running the tool

This command will run the tool process all `.jpg` files in the `sample_data` directory using the location history in `./sample_data/Location\ History/Records.json`:
```shell
go run main.go -d ./sample_data -f ./sample_data/Location\ History/Records.json
```

To get a list of all available options run:
```shell
go run main.go --help
```
