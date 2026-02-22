# cert-manager Webhook for Infoblox

This webhook enables cert-manager to solve ACME DNS01 challenges using Infoblox DNS. It automatically creates and deletes TXT records in Infoblox for challenge validation.

## Requirements

- A Kubernetes cluster with cert-manager installed
- Infoblox WAPI access
- Helm 3

## Setup

### 1. Add the Helm repository

```bash
helm repo add cert-manager-webhook-infoblox https://tazthemaniac.github.io/cert-manager-webhook-infoblox/
```

### 2. Install the chart

All options in the table below this setup section

```bash
# Replace acme.example.com with your unique groupName
helm install cert-manager-webhook-infoblox cert-manager-webhook-infoblox/cert-manager-webhook-infoblox \
  --namespace cert-manager \
  --set groupName="acme.example.com"
```

### 3. Create Infoblox Credentials Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: infoblox-credentials
  namespace: cert-manager
type: Opaque
stringData:
  username: username
  password: password
```

### 4. Create RBAC for Secret Access

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: webhook-infoblox:secret-reader
  namespace: cert-manager
rules:
  - apiGroups: [""]
    resources:
      - secrets
    resourceNames:
      - infoblox-credentials
    verbs:
      - get
      - watch

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: webhook-infoblox:secret-reader
  namespace: cert-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: webhook-infoblox:secret-reader
subjects:
  - apiGroup: ""
    kind: ServiceAccount
    name: cert-manager-webhook-infoblox
    namespace: cert-manager
```

### 5. Create a ClusterIssuer

Take the following template for letsencryp and customize it by replacing the following variables.

- `YOUR_EMAIL` - The email that should be associated with the certificates.
- `GROUP_NAME` - The group name you specified when you installed the chart.
- `INFOBLOX_ADDRESS` - The address or FQDN of the Infoblox server.
- `INFOBLOX_VIEW` - The name of the Infoblox view that can present TXT records for a DNS01 challenge.

The groupName is used to identify your company or business unit that created this webhook. This name will need to be referenced in each Issuer's webhook stanza to inform cert-manager of where to send ChallengePayload resources in order to solve the DNS01 challenge. This group name should be **unique**, hence using your own company's domain here is recommended.

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    email: <YOUR_EMAIL>
    server: https://acme-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: letsencrypt-account-key
    solvers:
      - dns01:
          webhook:
            groupName: <GROUP_NAME>
            solverName: infoblox
            config:
              host: <INFOBLOX_ADDRESS>
              view: <INFOBLOX_VIEW>
              usernameSecretRef:
                name: infoblox-credentials
                key: username
              passwordSecretRef:
                name: infoblox-credentials
                key: password
```

## Options and Default values

Following is a list of all the options available when installing the chart and the default values.

| Name                           | Default Value                              |
| ------------------------------ | ------------------------------------------ |
| groupName                      | ""                                         |
| nameOverride                   | ""                                         |
| fullNameOverride               | ""                                         |
| rootCACertificate.duration     | 43800h                                     |
| servingCertificate.duration    | 8760h                                      |
| certManager.namespace          | cert-manager                               |
| certManager.serviceAccountName | cert-manager                               |
| image.repository               | tazthemaniac/cert-manager-webhook-infoblox |
| image.tag                      | 1.2.0                                      |
| image.pullPolicy               | IfNotPresent                               |
| service.type                   | ClusterIP                                  |
| service.port                   | 443                                        |
| resources                      | {}                                         |
| nodeSelector                   | {}                                         |
| tolerations                    | []                                         |
| affinity                       | {}                                         |

## Troubleshooting

Some common issues include:

- Wrong groupName in your Issuer compared to the webhook
- Wrong DNS view configured in the issuer

## Credits

[Luis Gracia](https://github.com/luisico) for the original Infoblox webhook implementation.

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
