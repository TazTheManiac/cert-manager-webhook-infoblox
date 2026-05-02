# Local kind testing

This directory contains scripts for building and installing the webhook into an existing kind cluster. It is intended for validating the webhook deployment and configuration before testing against a real Infoblox GRID.

> **Limitation:** kind only gives you a local cluster to deploy into. Actually issuing a certificate still requires a reachable Infoblox GRID for DNS01 challenges, kind cannot substitute for that.

## Prerequisites

The following tools must be available in your PATH:

- [`kind`](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/)
- [`helm`](https://helm.sh/docs/intro/install/) 3.x
- [`docker`](https://docs.docker.com/get-docker/)
- `make` (for building the image)

A kind cluster with cert-manager already installed is assumed.

## Usage

### 1. Create the Infoblox credentials Secret

```sh
kubectl create secret generic infoblox-credentials \
  --namespace cert-manager \
  --from-literal=username=<user> \
  --from-literal=password=<pass>
```

### 2. Install the webhook

```sh
bash kind/setup.sh
```

This will:

1. Build the webhook Docker image locally via `make docker-build`
2. Load the image directly into the kind cluster with `kind load docker-image` no registry push is needed
3. Install the webhook Helm chart from the local `charts/` directory with `imagePullPolicy: Never`

The following environment variables are supported:

| Variable       | Default            | Description                                                    |
| -------------- | ------------------ | -------------------------------------------------------------- |
| `KIND_CLUSTER` | -                  | **Required** - Name of the kind cluster to load the image into |
| `NAMESPACE`    | `cert-manager`     | Namespace to install the chart into                            |
| `GROUP_NAME`   | `acme.example.com` | Webhook group name passed to Helm                              |

Example:

```sh
KIND_CLUSTER=my-cluster bash kind/setup.sh
```

### 3. Apply a ClusterIssuer

Follow the ClusterIssuer setup in the [root README](../README.md#3-configure-a-clusterissuer). Use the Let's Encrypt **staging** server while validating the setup to avoid rate limits on the production API.

### 4. Verify the webhook is healthy

```sh
kubectl get pods -n cert-manager
kubectl describe clusterissuer letsencrypt-staging
```

The ClusterIssuer should show `Ready: True` once the ACME account has been registered. If it stays pending, check the cert-manager controller logs:

```sh
kubectl logs -n cert-manager deployment/cert-manager
```

### 5. Uninstall the webhook

```sh
bash kind/teardown.sh
```

This uninstalls the webhook Helm release. The kind cluster and cert-manager are left intact.

## Files

| File          | Description                                            |
| ------------- | ------------------------------------------------------ |
| `setup.sh`    | Builds and loads the image, installs the webhook chart |
| `teardown.sh` | Uninstalls the webhook Helm release                    |
