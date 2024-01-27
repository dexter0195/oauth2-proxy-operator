# oauth2-proxy-operator (WIP)

This is a kubernetes operator for Oauth2Proxy. It allows you to run an instance of OAuth2Proxy by just specifying a CRD like this:

```
apiVersion: oauth2proxy.oauth2proxy-operator.dexter0195.com/v1alpha1
kind: OAuth2Proxy
metadata:
  name: oauth2proxy-sample
spec:
  size: 1
  envFromExistingSecret:
    secretRef:
      name: oauth2proxy-sample
  config: |
    provider = "azure"
    email_domains = ["*"]
    client_id = "xxx"
    azure_tenant = "xxx"
    oidc_issuer_url = "https://sts.windows.net/xxx/"
---
apiVersion: v1
kind: Secret
metadata:
  name: oauth2proxy-sample
type: Opaque
stringData:
  OAUTH2_PROXY_CLIENT_SECRET: "xxxx"
  OAUTH2_PROXY_COOKIE_SECRET: "xxx"
```

# WIP

This is a work in progress project that I created mostly for playing around with the operator-sdk. I wouldn't recommend running this in production.

Contributions are always welcome if you see potential in this tool :) 

## How to run

```bash
make generate
make manifests
make install
make build
export OAUTH2PROXY_IMAGE="quay.io/oauth2-proxy/oauth2-proxy:latest"
make run
```

## Future developments

- Generate automatically the Ingress resources based on the CRD
- Allow to use OAuth2proxy as a sidecar by automatically injecting the OAuth2 to new pods with a particular annotation