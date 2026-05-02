# cert-manager-webhook-infoblox

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/cert-manager-webhook-infoblox)](https://artifacthub.io/packages/search?repo=cert-manager-webhook-infoblox)
[![Docker Pulls](https://img.shields.io/docker/pulls/tazthemaniac/cert-manager-webhook-infoblox)](https://hub.docker.com/r/tazthemaniac/cert-manager-webhook-infoblox)
[![Docker Hub](https://img.shields.io/docker/v/tazthemaniac/cert-manager-webhook-infoblox?sort=semver&label=docker%20hub)](https://hub.docker.com/r/tazthemaniac/cert-manager-webhook-infoblox)
[![Docker](https://github.com/tazthemaniac/cert-manager-webhook-infoblox/actions/workflows/docker.yml/badge.svg)](https://github.com/tazthemaniac/cert-manager-webhook-infoblox/actions/workflows/docker.yml)
[![Helm](https://github.com/tazthemaniac/cert-manager-webhook-infoblox/actions/workflows/helm.yml/badge.svg)](https://github.com/tazthemaniac/cert-manager-webhook-infoblox/actions/workflows/helm.yml)

Forked from the [cert-manager/webhook-example](https://github.com/cert-manager/webhook-example) repository.  
Heavily inspired from the work done by [Luis Gracia](https://github.com/luisico) on their now archived  [cert-manager-webhook-infoblox-wapi](https://github.com/luisico/cert-manager-webhook-infoblox-wapi) project.

A [cert-manager](https://cert-manager.io) ACME DNS01 webhook solver for [Infoblox WAPI](https://www.infoblox.com/products/dns/). It enables cert-manager to create and clean up DNS TXT challenge records against an Infoblox GRID via the WAPI REST API.

## Requirements

- Kubernetes 1.34+
- cert-manager 1.19+ (tested with 1.19)
- Infoblox NIOS 8.x+ with WAPI v2.x enabled
- Helm 3.x (for chart-based installation)

## Installation

### 1. Create credentials

Create a Kubernetes secret containing the Infoblox username and password **before** installing the chart:

```sh
kubectl create secret generic infoblox-credentials \
  --namespace cert-manager \
  --from-literal=username=admin \
  --from-literal=password=s3cr3t
```

### 2. Install the Helm chart

```sh
helm repo add cert-manager-webhook-infoblox https://tazthemaniac.github.io/cert-manager-webhook-infoblox/
helm repo update

helm install cert-manager-webhook-infoblox cert-manager-webhook-infoblox/cert-manager-webhook-infoblox \
  --namespace cert-manager \
  --set groupName=acme.yourcompany.com
```

The `groupName` must be unique within your cluster and must match every Issuer/ClusterIssuer that references this solver.

By default the chart grants the webhook read access to a secret named `infoblox-credentials` in the same namespace. If your Secret has a different name, override it at install time:

```sh
helm install cert-manager-webhook-infoblox cert-manager-webhook-infoblox/cert-manager-webhook-infoblox \
  --namespace cert-manager \
  --set groupName=acme.yourcompany.com \
  --set credentialsSecret.name=infoblox-credentials
```

### 3. Configure a ClusterIssuer

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: you@yourcompany.com
    privateKeySecretRef:
      name: letsencrypt-prod-key   # cert-manager stores the ACME account key here; created automatically, must be unique per issuer
    solvers:
      - dns01:
          webhook:
            groupName: acme.yourcompany.com   # must match Helm groupName
            solverName: infoblox
            config:
              host: infoblox.yourcompany.com
              view: External
              usernameSecretRef:
                name: infoblox-credentials
                key: username
              passwordSecretRef:
                name: infoblox-credentials
                key: password
```

## Configuration reference

All fields are optional unless marked required.

| Field                 | Type              | Default  | Description                                                |
| --------------------- | ----------------- | -------- | ---------------------------------------------------------- |
| `host`                | string            | -        | **Required.** Infoblox GRID member FQDN or IP              |
| `port`                | string            | `"443"`  | WAPI HTTPS port                                            |
| `version`             | string            | `"2.10"` | WAPI version (e.g. `"2.11"`)                               |
| `view`                | string            | `""`     | DNS view containing the zone                               |
| `sslVerify`           | bool              | `false`  | Enable TLS certificate verification                        |
| `httpRequestTimeout`  | int               | `60`     | Per-request timeout in seconds                             |
| `httpPoolConnections` | int               | `10`     | Max idle connections to Infoblox                           |
| `ttl`                 | uint32            | `300`    | TTL set on created TXT records                             |
| `useTtl`              | bool              | `false`  | Whether to set the TTL field on TXT records                |
| `usernameSecretRef`   | SecretKeySelector | -        | Reference to a Secret key containing the Infoblox username |
| `passwordSecretRef`   | SecretKeySelector | -        | Reference to a Secret key containing the Infoblox password |

## Development

### Prerequisites

- Go 1.25+
- `make`
- Docker (for container builds)
- Helm 3 (for chart linting)

### Build

```sh
make build
```

### Unit tests

```sh
make test
```

### Integration / conformance tests

Integration tests require a real Infoblox server and a configured `testdata/infoblox/config.json` (see `testdata/infoblox/config.json.sample`).

Before running, ensure:

- `testdata/infoblox/config.json` exists and contains valid connection details for your Infoblox GRID
- The `view` field in `config.json` is set to the DNS view that is **externally reachable**, i.e. the view that serves the zone you are testing against. ACME DNS01 challenges are verified from the public internet, so using an internal-only view will cause the conformance suite to fail
- A `testdata/infoblox/credentials.yaml` Secret manifest exists (see `credentials.yaml.sample`); the test framework applies it automatically to the test namespace

```sh
TEST_ZONE_NAME=example.com. make test-integration
```

### Lint

```sh
make vet
make helm-lint
```

### Container image

```sh
make docker-build
make docker-push
```

The image is published to `tazthemaniac/cert-manager-webhook-infoblox` on Docker Hub. The tag is automatically derived from `appVersion` in `charts/cert-manager-webhook-infoblox/Chart.yaml`. To override it:

```sh
make docker-build-push IMAGE_TAG=2.0.0
```

## Project structure

```
.
├── main.go                               # Webhook solver implementation
├── main_test.go                          # Conformance integration test (build tag: integration)
├── main_unit_test.go                     # Unit tests
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
├── charts/
│   └── cert-manager-webhook-infoblox/    # Helm chart
└── testdata/
    └── infoblox/
        ├── config.json.sample            # Webhook config template
        └── credentials.yaml.sample       # Kubernetes Secret template
```
