# gcpidentitytokenportal

Web portal for vending GCP identity tokens via metadata service with flexible audience selection.

## Overview

`gcpidentitytokenportal` is a simple web application that provides an interface for vending GCP identity tokens with the ability to specify the audience. When running on GCP it can use the built in service account, outside of GCP you can specify the path to the JSON file for the service account, or on Kubernetes it can utilize Workload Identity Federation to impersonate a service account. This is useful for scenarios where you need to obtain a GCP identity token for testing or debugging purposes. The service account used to obtain the identity token is determined by the service account that the application is running as.

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

## Kubernetes with Workload Identity Federation & Account Impersonation

When running this application in a Kubernetes cluster, you can use the Kubernetes service account token to impersonate a service account with the necessary permissions to obtain the identity token even when not running on GKE. This assumes that Workload Identity Federation has been configured for the cluster including the public key for Kubernetes registered with the Workload Identity Pool.

Once that is done, **gcpidentitytokenportal** can be configured to use Google's syntax for impersonation. The following is an example of a ConfigMap and Deployment that can be used to run the application in a Kubernetes cluster:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gcp-wif-config
data:
  credential-configuration.json: |
    {
      "universe_domain": "googleapis.com",
      "type": "external_account",
      "audience": "//iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_NAME>/providers/<PROVIDER_NAME>",
      "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
      "token_url": "https://sts.googleapis.com/v1/token",
      "credential_source": {
        "file": "/var/run/secrets/tokens/gcp-token",
        "format": {
          "type": "text"
        }
      },
      "service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/<SERVICE_ACCOUNT_EMAIL>:generateAccessToken"
    }
```

Be sure to replace the placeholder values with your specific configuration settings including `<PROJECT_NUMBER>`, `<POOL_NAME>`, `<PROVIDER_NAME>`, and `<SERVICE_ACCOUNT_EMAIL>`.

This `credential-configuration.json` in the config map contains no secrets and is mounted as a volume in the deployment and is used to obtain the identity token.

The following is an example of a Deployment that can be used to run the application in a Kubernetes cluster ouside of GKE that is configured with Workload Identity Federation:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gcpidentitytokenportal
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: gcpidentitytokenportal
  template:
    metadata:
      labels:
        app: gcpidentitytokenportal
    spec:
      serviceAccountName: <KUBERNETES_SERVICE_ACCOUNT_NAME>
      containers:
        - name: gcpidentitytokenportal
          image: ghcr.io/unitvectory-labs/gcpidentitytokenportal:v0.1.0
          ports:
            - containerPort: 8080
          env:
            - name: GOOGLE_APPLICATION_CREDENTIALS
              value: "/etc/workload-identity/credential-configuration.json"
          volumeMounts:
            - mountPath: /var/run/secrets/tokens
              name: token-volume
              readOnly: true
            - name: workload-identity-credential-configuration
              mountPath: "/etc/workload-identity"
              readOnly: true
      volumes:
        - name: token-volume
          projected:
            sources:
              - serviceAccountToken:
                  path: gcp-token
                  expirationSeconds: 3600
                  audience: https://iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_NAME>/providers/<PROVIDER_NAME>
        - name: workload-identity-credential-configuration
          configMap:
            name: gcp-wif-config
```

Be sure to replace the placeholder values with your specific configuration settings including `<PROJECT_NUMBER>`, `<POOL_NAME>`, `<PROVIDER_NAME>`, and `<KUBERNETES_SERVICE_ACCOUNT_NAME>`.

In order for this to work the service account that we are impersonating needs to have the `Workload Identity User` grant the principal for the Workload Identiy Federation. This principal is in the following format: `principal://iam.googleapis.com/projects/<PROJECT_NUMBER>/locations/global/workloadIdentityPools/<POOL_NAME>/subject/system:serviceaccount:<NAMESPACE>:<KUBERNETES_SERVICE_ACCOUNT_NAME>`
