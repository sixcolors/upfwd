# upfwd

[![CI](https://github.com/sixcolors/upfwd/actions/workflows/ci.yml/badge.svg)](https://github.com/sixcolors/upfwd/actions/workflows/ci.yml)
[![CodeQL](https://github.com/sixcolors/upfwd/actions/workflows/codeql.yml/badge.svg)](https://github.com/sixcolors/upfwd/actions/workflows/codeql.yml)
[![Trivy](https://github.com/sixcolors/upfwd/actions/workflows/trivy.yml/badge.svg)](https://github.com/sixcolors/upfwd/actions/workflows/trivy.yml)
[![Coverage Status](https://coveralls.io/repos/github/sixcolors/upfwd/badge.svg?branch=main)](https://coveralls.io/github/sixcolors/upfwd?branch=main)

## Description

This is a simple HTTP server that performs a health check on a target URL and redirects to the new server if the health check passes (StatusTemporaryRedirect). If the health check fails, it returns a 503 Service Unavailable response with a custom HTML page or json response (depending on the Accept header or /api/ path prefix if Accept header is not present).

This service is designed to be run in a Docker container and is intended to be run behind a reverse proxy that handles TLS termination and protocol upgrades.

This was built to facilitate a migration with planned downtime and a DNS change. I am making it available in case it is useful to others.

## Getting Started

This service is designed to be run in a Docker container. The Dockerfile is included in this repository.

For convenience, the html and json are embedded in the binary. If you want to customize the html or json, you can build the binary yourself or open a PR if you have a better idea (please open and link to an issue if you open a PR).

### Dependencies

* Go version 1.20 or later
* Docker version 20.10.8 or later

### Building

* run `go mod download` to download the dependencies
* run `go build` to build the binary
* run `docker build -t upfwd .` to build the Docker image

### Environment Variables

* `SERVER_PORT` - The port to listen on - defaults to `3000` - ***NOTE*** this is the same environment variable used by both docker and the binary.
* `TARGET_URL` - The URL to redirect to if the health check passes - defaults to `https://example.com`
* `HEALTH_CHECK_URL` - The URL to perform the health check against - defaults to `https://example.com/healthz`
* `HEALTH_CHECK_INTERVAL` - The interval in seconds between health checks, in seconds - defaults to `60`
* `HEALTH_CHECK_TIMEOUT` - The timeout in seconds for the health check, in seconds - defaults to `10`
* `HEALTH_CHECK_SUCCESS_CODE` - The HTTP status code that indicates a successful health check - defaults to `200`
* `HEALTH_CHECK_BODY` - The body of the response that indicates a successful health check - If not specified, the body is ignored

### Security Considerations

* This service is designed to be run in a Docker container using HTTP, it should be run behind a reverse proxy that handles TLS termination and authentication and protocol upgrades.
* The Dockerfile is designed to be run as a non-root user.
* The image is distroless, so it does not include a shell or other utilities that could be used to compromise the container.

### Executing program

* run `docker run -p 3000:3000 upfwd` to run the Docker container on port 3000

## Authors

Contributors names and contact info

* [@sixcolors](https://github.com/sixcolors)

## Version History

* 0.1
    * Initial Release

## License

This project is licensed under the MIT License - see [LICENSE](LICENSE) for details

## Contributing

I am open to contributions. Please open an issue before opening a PR.

Steps to contribute:

1. Open an issue (<github.com/sixcolors/upfwd/issues>)
2. Fork it (<github.com/sixcolors/upfwd/fork>)
3. Create your feature branch (`git checkout -b feature/fooBar`)
4. Commit your changes (`git commit -am 'Add some fooBar'`)
5. Push to the branch (`git push origin feature/fooBar`)
6. Create a new Pull Request (<github.com/sixcolors/upfwd/pulls>)
7. Provide a description of the changes and link to the issue
8. Wait for the PR to be reviewed and merged

## Acknowledgments

* [k8s.io/utils](https://k8s.io/utils) - for reading the environment variables and falling back to defaults