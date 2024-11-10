# gcpidentitytokenportal

Web portal for vending GCP identity tokens via metadata service with flexible audience selection.

## Overview

`gcpidentitytokenportal` is a simple web application that provides an interface for vending GCP identity tokens with the ability to specify the audience. This is useful for scenarios where you need to obtain a GCP identity token for testing or debugging purposes. The service account used to obtain the identity token is determined by the service account that the application is running as.

![Application Interface](./assets/interface.png)

## Usage

The latest `gcpidentitytokenportal` Docker image is available for deployment from GitHub Packages at [ghcr.io/unitvectory-labs/gcpidentitytokenportal](https://github.com/UnitVectorY-Labs/gcpidentitytokenportal/pkgs/container/gcpidentitytokenportal).

The application can be run outside of GCP using the following command:

```bash
docker run --name gcpidentitytokenportal -d -p 8080:8080 \
  -v /path/to/your-service-account-key.json:/creds.json \
  -v /path/to/your-config.yaml:/config.yaml \
  -e GOOGLE_APPLICATION_CREDENTIALS=/creds.json \
  ghcr.io/unitvectory-labs/gcpidentitytokenportal:v0.1.0
```

This application is intended to run on GCP in an environment such as Cloud Run.

No special permissions are required to run this application. The service account under which the application runs will be used to obtain the identity token. The recipient system must verify the token using Google's public keys. Any resource access permissions are determined by the service account running the application, but these permissions are not required to obtain the identity token.

## Configuration

This application is configured using environment variables:

- `GOOGLE_APPLICATION_CREDENTIALS`: (Optional) The path to your Google Cloud service account key file. If not provided and running on GCP, the application will use the default service account credentials. If not provided and not running on GCP, the application will fail to start.
- `PORT`: The port on which the server listens (default: 8080).

When running the Docker container and using `GOOGLE_APPLICATION_CREDENTIALS` to set the path to the credentials file, this path will be for the file in the container, therefore you will need to mount the file from the host machine to the container. This can be done by using the `-v` flag when running the container. A path such as `/config.yaml` can be used to mount the file and then `GOOGLE_APPLICATION_CREDENTIALS=/config.yaml` can be used to set the environment variable.

By default, any audience can be specified. To restrict audiences, mount a YAML file to the container at `/config.yaml` with `audiences` defined as a list of allowed values:

```yaml
# These audiences will be provided as options in the dropdown menu
audiences:
  - https://api.example.com
  - https://service.example.com
```
