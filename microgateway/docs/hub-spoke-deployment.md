# Hub-and-Spoke Deployment Guide

This guide provides practical deployment examples and patterns for the microgateway hub-and-spoke architecture.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Docker Deployments](#docker-deployments)
3. [Kubernetes Deployments](#kubernetes-deployments)
4. [Cloud Deployments](#cloud-deployments)
5. [Production Patterns](#production-patterns)
6. [Multi-Environment Setup](#multi-environment-setup)

## Quick Start

### Prerequisites

- Docker and Docker Compose (for containerized deployment)
- PostgreSQL database (for production) or SQLite (for development)
- Network connectivity between control and edge instances

### 5-Minute Setup

1. **Start Control Instance:**
```bash
# Create database
docker run --name mgw-postgres -e POSTGRES_PASSWORD=postgres -d -p 5432:5432 postgres:15

# Run database migration
GATEWAY_MODE=control \
DATABASE_TYPE=postgres \
DATABASE_DSN="postgres://postgres:postgres@localhost:5432/postgres" \
./microgateway -migrate

# Start control instance
GATEWAY_MODE=control \
DATABASE_TYPE=postgres \
DATABASE_DSN="postgres://postgres:postgres@localhost:5432/postgres" \
GRPC_PORT=50051 \
GRPC_AUTH_TOKEN=quickstart-token \
./microgateway
```

2. **Start Edge Instance:**
```bash
# In another terminal
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=localhost:50051 \
EDGE_ID=quickstart-edge \
EDGE_NAMESPACE=demo \
EDGE_AUTH_TOKEN=quickstart-token \
./microgateway
```

3. **Verify Setup:**
```bash
# Check control instance
curl http://localhost:8080/health

# Check edge instance (should be running on port 8081)
curl http://localhost:8081/health

# List connected edges
curl http://localhost:8080/api/v1/edges
```

## Docker Deployments

### Single Host Development

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: microgateway
      POSTGRES_USER: mgw
      POSTGRES_PASSWORD: mgw_password
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U mgw -d microgateway"]
      interval: 5s
      timeout: 5s
      retries: 5

  control:
    image: microgateway:latest
    environment:
      GATEWAY_MODE: control
      DATABASE_TYPE: postgres
      DATABASE_DSN: postgres://mgw:mgw_password@postgres:5432/microgateway
      GRPC_PORT: 50051
      GRPC_AUTH_TOKEN: docker-demo-token
      PORT: 8080
    ports:
      - "8080:8080"
      - "50051:50051"
    depends_on:
      postgres:
        condition: service_healthy
    command: >
      sh -c "
        ./microgateway -migrate &&
        ./microgateway
      "

  edge-1:
    image: microgateway:latest
    environment:
      GATEWAY_MODE: edge
      CONTROL_ENDPOINT: control:50051
      EDGE_ID: edge-1
      EDGE_NAMESPACE: demo
      EDGE_AUTH_TOKEN: docker-demo-token
      PORT: 8080
    ports:
      - "8081:8080"
    depends_on:
      - control

  edge-2:
    image: microgateway:latest
    environment:
      GATEWAY_MODE: edge
      CONTROL_ENDPOINT: control:50051
      EDGE_ID: edge-2
      EDGE_NAMESPACE: demo
      EDGE_AUTH_TOKEN: docker-demo-token
      PORT: 8080
    ports:
      - "8082:8080"
    depends_on:
      - control

volumes:
  postgres_data:
```

**Deploy:**
```bash
docker-compose up -d
```

### Multi-Host Production

Create separate compose files for control and edge instances:

**`control-compose.yml`:**
```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: microgateway_prod
      POSTGRES_USER: mgw_prod
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./postgres-init:/docker-entrypoint-initdb.d
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U mgw_prod -d microgateway_prod"]
      interval: 10s
      timeout: 5s
      retries: 3

  control:
    image: microgateway:latest
    environment:
      GATEWAY_MODE: control
      DATABASE_TYPE: postgres
      DATABASE_DSN: postgres://mgw_prod:${POSTGRES_PASSWORD}@postgres:5432/microgateway_prod
      GRPC_PORT: 50051
      GRPC_TLS_ENABLED: ${GRPC_TLS_ENABLED:-false}
      GRPC_TLS_CERT_PATH: /etc/certs/server.crt
      GRPC_TLS_KEY_PATH: /etc/certs/server.key
      GRPC_AUTH_TOKEN: ${GRPC_AUTH_TOKEN}
      LOG_LEVEL: info
      LOG_FORMAT: json
    ports:
      - "8080:8080"
      - "50051:50051"
    volumes:
      - ./certs:/etc/certs:ro
      - ./logs:/var/log/microgateway
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped
    command: >
      sh -c "
        ./microgateway -migrate &&
        ./microgateway
      "

volumes:
  postgres_data:
```

**`edge-compose.yml`:**
```yaml
version: '3.8'

services:
  edge:
    image: microgateway:latest
    environment:
      GATEWAY_MODE: edge
      CONTROL_ENDPOINT: ${CONTROL_ENDPOINT}
      EDGE_ID: ${EDGE_ID}
      EDGE_NAMESPACE: ${EDGE_NAMESPACE}
      EDGE_TLS_ENABLED: ${EDGE_TLS_ENABLED:-false}
      EDGE_AUTH_TOKEN: ${EDGE_AUTH_TOKEN}
      LOG_LEVEL: info
      LOG_FORMAT: json
    ports:
      - "8080:8080"
    volumes:
      - ./certs:/etc/certs:ro
      - ./logs:/var/log/microgateway
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:8080/health || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
```

**Deploy Control:**
```bash
export POSTGRES_PASSWORD=secure_password
export GRPC_AUTH_TOKEN=production_token
docker-compose -f control-compose.yml up -d
```

**Deploy Edge:**
```bash
export CONTROL_ENDPOINT=control.internal.company.com:50051
export EDGE_ID=prod-us-west-1
export EDGE_NAMESPACE=production
export EDGE_AUTH_TOKEN=production_token
docker-compose -f edge-compose.yml up -d
```

## Kubernetes Deployments

### Namespace Setup

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: microgateway
---
apiVersion: v1
kind: Secret
metadata:
  name: microgateway-secrets
  namespace: microgateway
type: Opaque
stringData:
  postgres-password: "secure_postgres_password"
  grpc-auth-token: "secure_grpc_auth_token"
  database-dsn: "postgres://mgw:secure_postgres_password@postgres:5432/microgateway"
```

### PostgreSQL Database

```yaml
# postgres.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: microgateway
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:15
        env:
        - name: POSTGRES_DB
          value: microgateway
        - name: POSTGRES_USER
          value: mgw
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: microgateway-secrets
              key: postgres-password
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
        readinessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - mgw
            - -d
            - microgateway
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: postgres-storage
        persistentVolumeClaim:
          claimName: postgres-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: microgateway
spec:
  selector:
    app: postgres
  ports:
  - port: 5432
    targetPort: 5432
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
  namespace: microgateway
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
```

### Control Instance

```yaml
# control.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: microgateway-control
  namespace: microgateway
spec:
  replicas: 2
  selector:
    matchLabels:
      app: microgateway-control
  template:
    metadata:
      labels:
        app: microgateway-control
    spec:
      initContainers:
      - name: migrate
        image: microgateway:latest
        command: ["./microgateway", "-migrate"]
        env:
        - name: GATEWAY_MODE
          value: "control"
        - name: DATABASE_TYPE
          value: "postgres"
        - name: DATABASE_DSN
          valueFrom:
            secretKeyRef:
              name: microgateway-secrets
              key: database-dsn
      containers:
      - name: microgateway
        image: microgateway:latest
        env:
        - name: GATEWAY_MODE
          value: "control"
        - name: DATABASE_TYPE
          value: "postgres"
        - name: DATABASE_DSN
          valueFrom:
            secretKeyRef:
              name: microgateway-secrets
              key: database-dsn
        - name: GRPC_PORT
          value: "50051"
        - name: GRPC_AUTH_TOKEN
          valueFrom:
            secretKeyRef:
              name: microgateway-secrets
              key: grpc-auth-token
        - name: PORT
          value: "8080"
        - name: LOG_LEVEL
          value: "info"
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 50051
          name: grpc
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 256Mi
---
apiVersion: v1
kind: Service
metadata:
  name: microgateway-control
  namespace: microgateway
spec:
  selector:
    app: microgateway-control
  ports:
  - port: 8080
    targetPort: 8080
    name: http
  - port: 50051
    targetPort: 50051
    name: grpc
---
apiVersion: v1
kind: Service
metadata:
  name: microgateway-control-lb
  namespace: microgateway
spec:
  type: LoadBalancer
  selector:
    app: microgateway-control
  ports:
  - port: 8080
    targetPort: 8080
    name: http
  - port: 50051
    targetPort: 50051
    name: grpc
```

### Edge Instance

```yaml
# edge.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: microgateway-edge
  namespace: microgateway
spec:
  replicas: 3
  selector:
    matchLabels:
      app: microgateway-edge
  template:
    metadata:
      labels:
        app: microgateway-edge
    spec:
      containers:
      - name: microgateway
        image: microgateway:latest
        env:
        - name: GATEWAY_MODE
          value: "edge"
        - name: CONTROL_ENDPOINT
          value: "microgateway-control:50051"
        - name: EDGE_NAMESPACE
          value: "production"
        - name: EDGE_AUTH_TOKEN
          valueFrom:
            secretKeyRef:
              name: microgateway-secrets
              key: grpc-auth-token
        - name: PORT
          value: "8080"
        - name: LOG_LEVEL
          value: "info"
        - name: EDGE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        ports:
        - containerPort: 8080
          name: http
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
---
apiVersion: v1
kind: Service
metadata:
  name: microgateway-edge
  namespace: microgateway
spec:
  type: LoadBalancer
  selector:
    app: microgateway-edge
  ports:
  - port: 80
    targetPort: 8080
    name: http
```

**Deploy to Kubernetes:**
```bash
kubectl apply -f namespace.yaml
kubectl apply -f postgres.yaml
kubectl apply -f control.yaml
kubectl apply -f edge.yaml
```

## Cloud Deployments

### AWS with ECS

**Task Definition for Control Instance:**
```json
{
  "family": "microgateway-control",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "256",
  "memory": "512",
  "executionRoleArn": "arn:aws:iam::ACCOUNT:role/ecsTaskExecutionRole",
  "taskRoleArn": "arn:aws:iam::ACCOUNT:role/microgatewaTaskRole",
  "containerDefinitions": [
    {
      "name": "microgateway-control",
      "image": "your-account.dkr.ecr.region.amazonaws.com/microgateway:latest",
      "portMappings": [
        {"containerPort": 8080, "protocol": "tcp"},
        {"containerPort": 50051, "protocol": "tcp"}
      ],
      "environment": [
        {"name": "GATEWAY_MODE", "value": "control"},
        {"name": "DATABASE_TYPE", "value": "postgres"},
        {"name": "GRPC_PORT", "value": "50051"}
      ],
      "secrets": [
        {
          "name": "DATABASE_DSN",
          "valueFrom": "arn:aws:secretsmanager:region:account:secret:mgw-database-dsn"
        },
        {
          "name": "GRPC_AUTH_TOKEN",
          "valueFrom": "arn:aws:secretsmanager:region:account:secret:mgw-grpc-token"
        }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/microgateway-control",
          "awslogs-region": "us-west-2",
          "awslogs-stream-prefix": "ecs"
        }
      },
      "healthCheck": {
        "command": ["CMD-SHELL", "curl -f http://localhost:8080/health || exit 1"],
        "interval": 30,
        "timeout": 5,
        "retries": 3
      }
    }
  ]
}
```

**Task Definition for Edge Instance:**
```json
{
  "family": "microgateway-edge",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "256",
  "memory": "256",
  "executionRoleArn": "arn:aws:iam::ACCOUNT:role/ecsTaskExecutionRole",
  "containerDefinitions": [
    {
      "name": "microgateway-edge",
      "image": "your-account.dkr.ecr.region.amazonaws.com/microgateway:latest",
      "portMappings": [
        {"containerPort": 8080, "protocol": "tcp"}
      ],
      "environment": [
        {"name": "GATEWAY_MODE", "value": "edge"},
        {"name": "CONTROL_ENDPOINT", "value": "mgw-control-nlb.internal:50051"},
        {"name": "EDGE_NAMESPACE", "value": "production"}
      ],
      "secrets": [
        {
          "name": "EDGE_AUTH_TOKEN",
          "valueFrom": "arn:aws:secretsmanager:region:account:secret:mgw-grpc-token"
        }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/microgateway-edge",
          "awslogs-region": "us-west-2",
          "awslogs-stream-prefix": "ecs"
        }
      }
    }
  ]
}
```

### Google Cloud with GKE

**CloudSQL Integration:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: cloudsql-instance-credentials
type: Opaque
data:
  credentials.json: <base64-encoded-service-account-key>
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: microgateway-control
spec:
  template:
    spec:
      containers:
      - name: microgateway
        image: gcr.io/PROJECT_ID/microgateway:latest
        env:
        - name: DATABASE_DSN
          value: "postgres://mgw:password@127.0.0.1:5432/microgateway"
      - name: cloudsql-proxy
        image: gcr.io/cloudsql-docker/gce-proxy:1.33.2
        command:
        - "/cloud_sql_proxy"
        - "-instances=PROJECT_ID:REGION:INSTANCE_NAME=tcp:5432"
        - "-credential_file=/secrets/cloudsql/credentials.json"
        volumeMounts:
        - name: cloudsql-instance-credentials
          mountPath: /secrets/cloudsql
          readOnly: true
      volumes:
      - name: cloudsql-instance-credentials
        secret:
          secretName: cloudsql-instance-credentials
```

### Azure with ACI

**Resource Group and Database:**
```bash
# Create resource group
az group create --name microgateway-rg --location eastus

# Create PostgreSQL server
az postgres server create \
  --resource-group microgateway-rg \
  --name microgateway-db \
  --location eastus \
  --admin-user mgwadmin \
  --admin-password SecurePassword123 \
  --sku-name GP_Gen5_2
```

**Container Instances:**
```yaml
# control-aci.yaml
apiVersion: 2019-12-01
location: eastus
name: microgateway-control
properties:
  containers:
  - name: microgateway-control
    properties:
      image: youregistry.azurecr.io/microgateway:latest
      resources:
        requests:
          cpu: 0.5
          memoryInGb: 1
      ports:
      - port: 8080
      - port: 50051
      environmentVariables:
      - name: GATEWAY_MODE
        value: control
      - name: DATABASE_TYPE
        value: postgres
      - name: DATABASE_DSN
        secureValue: postgres://mgwadmin:SecurePassword123@microgateway-db.postgres.database.azure.com:5432/microgateway
      - name: GRPC_AUTH_TOKEN
        secureValue: azure-secure-token
  osType: Linux
  ipAddress:
    type: Public
    ports:
    - protocol: tcp
      port: 8080
    - protocol: tcp
      port: 50051
tags:
  app: microgateway
  component: control
```

## Production Patterns

### High Availability Control Plane

**Multi-AZ Deployment:**
```yaml
# HA Control with shared database
apiVersion: apps/v1
kind: Deployment
metadata:
  name: microgateway-control
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - microgateway-control
              topologyKey: kubernetes.io/hostname
      containers:
      - name: microgateway
        # ... container spec
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
```

### Geographic Distribution

**Multi-Region Setup:**
```bash
# US West Control
GATEWAY_MODE=control \
DATABASE_DSN="postgres://user:pass@us-west-db:5432/mgw" \
GRPC_AUTH_TOKEN=global-token \
./microgateway

# US East Edge
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=us-west-control.company.com:50051 \
EDGE_NAMESPACE=us-east \
EDGE_AUTH_TOKEN=global-token \
./microgateway

# Europe Edge
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=us-west-control.company.com:50051 \
EDGE_NAMESPACE=europe \
EDGE_AUTH_TOKEN=global-token \
./microgateway

# Asia Edge
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=us-west-control.company.com:50051 \
EDGE_NAMESPACE=asia \
EDGE_AUTH_TOKEN=global-token \
./microgateway
```

### Auto-Scaling Configuration

**Horizontal Pod Autoscaler (HPA):**
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: microgateway-edge-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: microgateway-edge
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

## Multi-Environment Setup

### Environment Separation

**Development:**
```bash
# Dev Control
GATEWAY_MODE=control
EDGE_NAMESPACE=development
GRPC_AUTH_TOKEN=dev-token

# Dev Edge
GATEWAY_MODE=edge
CONTROL_ENDPOINT=dev-control.internal:50051
EDGE_NAMESPACE=development
EDGE_AUTH_TOKEN=dev-token
```

**Staging:**
```bash
# Staging Control
GATEWAY_MODE=control
DATABASE_DSN="postgres://mgw:pass@staging-db:5432/mgw_staging"
GRPC_AUTH_TOKEN=staging-token

# Staging Edge
GATEWAY_MODE=edge
CONTROL_ENDPOINT=staging-control.internal:50051
EDGE_NAMESPACE=staging
EDGE_AUTH_TOKEN=staging-token
```

**Production:**
```bash
# Production Control
GATEWAY_MODE=control
DATABASE_DSN="postgres://mgw:pass@prod-db:5432/mgw_production"
GRPC_TLS_ENABLED=true
GRPC_AUTH_TOKEN=production-token

# Production Edge
GATEWAY_MODE=edge
CONTROL_ENDPOINT=prod-control.internal:50051
EDGE_NAMESPACE=production
EDGE_TLS_ENABLED=true
EDGE_AUTH_TOKEN=production-token
```

### CI/CD Integration

**GitLab CI Pipeline:**
```yaml
# .gitlab-ci.yml
stages:
  - build
  - test
  - deploy-dev
  - deploy-staging
  - deploy-production

build:
  stage: build
  script:
    - docker build -t $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA .
    - docker push $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA

deploy-edge:
  stage: deploy-production
  script:
    - kubectl set image deployment/microgateway-edge 
        microgateway=$CI_REGISTRY_IMAGE:$CI_COMMIT_SHA
    - kubectl rollout status deployment/microgateway-edge
  only:
    - main
```

This deployment guide provides comprehensive examples for various deployment scenarios. For operational procedures, see the [Operations Guide](./hub-spoke-operations.md).