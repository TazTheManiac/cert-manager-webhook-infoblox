A [cert-manager](https://cert-manager.io) ACME DNS01 webhook solver for [Infoblox WAPI](https://www.infoblox.com/products/dns/).

For full documentation and configuration options, see the [Artifact Hub page](https://artifacthub.io/packages/helm/cert-manager-webhook-infoblox/cert-manager-webhook-infoblox).

## Quick Start

### 1. Add the Helm repository

```sh
helm repo add cert-manager-webhook-infoblox https://tazthemaniac.github.io/cert-manager-webhook-infoblox/
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
      name: letsencrypt-account-key
    solvers:
      - dns01:
          webhook:
            groupName: acme.yourcompany.com
            solverName: infoblox
            config:
              host: infoblox.yourcompany.com
              view: default
              usernameSecretRef:
                name: infoblox-credentials
                key: username
              passwordSecretRef:
                name: infoblox-credentials
                key: password
```
