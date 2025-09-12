# Movie Server

This server serves movie lists via a REST API.

## Build Instructions

```bash
$ singularity build movie-server.sif singularity.def
```

## Run client:

```bash
singularity run [image-name] [options]
```

### Options:

- `--server SERVER` Specify the server URL
- `--port PORT` Specify the server port (default: 8080)
- `--year YEAR` Specify the year to fetch movies for
- `--username USERNAME` Specify the username for authentication
- `--password PASSWORD` Specify the password for authentication
- `--verbose` Enable verbose output (optional)

### Example:

```bash
singularity run movie-server.sif --year 1900 --username user --password pass --verbose
```

## Run server

```bash
singularity instance start [image-name] [name] [options]
singularity instance stop [name]
```

### Options:

- `-port PORT` Specify the port for the server (default: 8080)

### Example:

```bash
singularity instance start movie-server.sif my_server --port 8081
singularity instance stop my_server
```
