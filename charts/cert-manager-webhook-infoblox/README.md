# cert-manager-webhook-infoblox

A [cert-manager](https://cert-manager.io) ACME DNS01 webhook solver for [Infoblox WAPI](https://www.infoblox.com/products/dns/). It enables cert-manager to create and clean up DNS TXT challenge records against an Infoblox GRID via the WAPI REST API.

## Requirements

- Kubernetes 1.34+
- cert-manager 1.19+
- Infoblox NIOS 8.x+ with WAPI v2.x enabled
- Helm 3.x

## Installation

### 1. Add the Helm repository

```sh
helm repo add cert-manager-webhook-infoblox https://tazthemaniac.github.io/cert-manager-webhook-infoblox/
helm repo update
```

### 2. Create the Infoblox credentials Secret

```sh
kubectl create secret generic infoblox-credentials \
  --namespace cert-manager \
  --from-literal=username=admin \
  --from-literal=password=s3cr3t
```

### 3. Install the chart

```sh
helm install cert-manager-webhook-infoblox cert-manager-webhook-infoblox/cert-manager-webhook-infoblox \
  --namespace cert-manager \
  --set groupName="acme.yourcompany.com"
```

The `groupName` must be unique within your cluster and must match every Issuer/ClusterIssuer that references this solver. Using your own company's domain is recommended.

The chart automatically creates a Role and RoleBinding granting the webhook access to the credentials Secret. If your Secret has a different name, override it at install time:

```sh
helm install cert-manager-webhook-infoblox cert-manager-webhook-infoblox/cert-manager-webhook-infoblox \
  --namespace cert-manager \
  --set groupName="acme.yourcompany.com" \
  --set credentialsSecret.name=infoblox-credentials
```

### 4. Create a ClusterIssuer

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    email: you@yourcompany.com
    server: https://acme-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: letsencrypt-prod-key   # cert-manager stores the ACME account key here. Created automatically, but must be unique per issuer
    solvers:
      - dns01:
          webhook:
            groupName: acme.yourcompany.com
            solverName: infoblox
            config:
              host: infoblox.yourcompany.com
              view: External
              # sslVerify: false   # set to false if Infoblox uses a self-signed or private CA certificate
              usernameSecretRef:
                name: infoblox-credentials
                key: username
              passwordSecretRef:
                name: infoblox-credentials
                key: password
```

## Configuration

| Parameter                          | Default                                      | Description                                                                          |
| ---------------------------------- | -------------------------------------------- | ------------------------------------------------------------------------------------ |
| `groupName`                        | `""`                                         | **Required.** Unique group name for this webhook (use your company's domain)         |
| `credentialsSecret.name`         | `infoblox-credentials`                       | Name of the Secret containing the Infoblox credentials                       |
| `certManager.namespace`            | `cert-manager`                               | Namespace where cert-manager is installed                                            |
| `certManager.serviceAccountName`   | `cert-manager`                               | cert-manager's ServiceAccount name                                                   |
| `replicaCount`                     | `1`                                          | Number of webhook pod replicas                                                       |
| `image.repository`                 | `tazthemaniac/cert-manager-webhook-infoblox` | Container image repository                                                           |
| `image.tag`                        | `""`                                         | Container image tag, defaults to the chart appVersion                                |
| `image.pullPolicy`                 | `IfNotPresent`                               | Image pull policy                                                                    |
| `imagePullSecrets`                 | `[]`                                         | Image pull secrets for private registries                                            |
| `rootCACertificate.duration`       | `43800h`                                     | Duration of the self-signed root CA certificate (5 years)                            |
| `servingCertificate.duration`      | `8760h`                                      | Duration of the webhook serving certificate (1 year)                                 |
| `podAnnotations`                   | `{}`                                         | Annotations added to the webhook pod                                                 |
| `service.type`                     | `ClusterIP`                                  | Kubernetes service type                                                              |
| `service.port`                     | `443`                                        | Kubernetes service port                                                              |
| `service.annotations`              | `{}`                                         | Annotations added to the service                                                     |
| `resources`                        | `{}`                                         | Resource requests and limits for the webhook container                               |
| `nodeSelector`                     | `{}`                                         | Node selector for the webhook pod                                                    |
| `tolerations`                      | `[]`                                         | Tolerations for the webhook pod                                                      |
| `affinity`                         | `{}`                                         | Affinity rules for the webhook pod                                                   |
| `topologySpreadConstraints`        | `[]`                                         | Topology spread constraints for the webhook pod                                      |
| `podDisruptionBudget.enabled`      | `false`                                      | Enable a PodDisruptionBudget for the webhook                                         |
| `podDisruptionBudget.minAvailable` | -                                            | Minimum pods that must remain available; integer or percentage (e.g. `1` or `"50%"`) |

## Issuer `config` reference

| Field                 | Default  | Description                                                             |
| --------------------- | -------- | ----------------------------------------------------------------------- |
| `host`                | -        | **Required.** Infoblox GRID member FQDN or IP                           |
| `port`                | `"443"`  | WAPI HTTPS port                                                         |
| `version`             | `"2.10"` | WAPI version                                                            |
| `view`                | `""`     | DNS view containing the zone                                            |
| `sslVerify`           | `true`   | Enable TLS certificate verification                                     |
| `httpRequestTimeout`  | `60`     | Per-request timeout in seconds                                          |
| `httpPoolConnections` | `10`     | Max idle connections to Infoblox                                        |
| `ttl`                 | `300`    | TTL set on created TXT records                                          |
| `useTtl`              | `false`  | Whether to set the TTL field on TXT records                             |
| `usernameSecretRef`   | -       | **Required.** Reference to the Secret key holding the Infoblox username |
| `passwordSecretRef`   | -       | **Required.** Reference to the Secret key holding the Infoblox password |

## Upgrading from 1.x

### Breaking changes

| Change                                                                    | Action required                                                                                                                                                 |
| ------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `scheme` config field removed                                             | Remove `scheme` from your Issuer/ClusterIssuer `config` block. HTTPS is always used. For self-signed or private CA certificates set `sslVerify: false` instead. |
| `version` default changed from `2.13.5` to `2.10`                         | Explicitly set `version` in your Issuer config if you depend on a specific WAPI version.                                                                        |
| RBAC created automatically by the chart                                   | Delete any manually-managed `webhook-infoblox:secret-reader` Role and RoleBinding, the chart now manages them.                                                  |
| New `credentialsSecret.name` Helm value (default: `infoblox-credentials`) | Set `--set credentialsSecret.name=<name>` at upgrade time if your credentials Secret has a different name. |
| `groupName` is now schema-validated as non-empty                          | Always was required; `helm upgrade` will now fail explicitly if it is not set.                                                                                  |
| Runtime changed to `distroless/static-debian12:nonroot` (UID 65532)       | Update any PodSecurityPolicy, OPA, or Kyverno rules that pinned UID `65534`.                                                                                    |
