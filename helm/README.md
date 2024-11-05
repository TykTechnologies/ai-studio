# Midsommar Helm Chart

### 1. Install cert-manager in your cluster (if not already installed):

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml
```

### 2. Create a ClusterIssuer for Let's Encrypt:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx

```

### 3. Create a custom values file for your specific environment:

```yaml
# my-values.yaml
midsommar:
  ingress:
    hosts:
      - host: your-actual-domain.com
        paths:
          - path: /
            pathType: Prefix
            port: 8080
      - host: gateway.your-actual-domain.com
        paths:
          - path: /
            pathType: Prefix
            port: 9090
    tls:
      - secretName: your-tls-secret
        hosts:
          - your-actual-domain.com
      - secretName: your-gateway-tls-secret
        hosts:
          - gateway.your-actual-domain.com

config:
  adminEmail: "your-actual-email@domain.com"
  siteUrl: "https://your-actual-domain.com"
  fromEmail: "noreply@your-actual-domain.com"

postgres:
  auth:
    password: "your-secure-password"
```

###  4. Then deploy with:

```bash
helm upgrade --install midsommar . -f my-values.yaml
