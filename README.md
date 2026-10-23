# Uber System Design, Part 1

A small ride-hailing location service: drivers send their position over a WebSocket, riders get the drivers within 2 km pushed to them once a second. It runs on a local Kubernetes cluster (kind) behind HAProxy, and k6 plays the drivers.

```
app/   Go service (main.go) + the live map (map.html)
k8s/   kind cluster, HAProxy values, app, Redis, autoscaler
k6/    load test: drivers + one rider per 10 drivers
```

## You need

Docker, [kind](https://kind.sigs.k8s.io), kubectl, [helm](https://helm.sh), [k6](https://k6.io).

## 1. Cluster, HAProxy, metrics-server

```bash
kind create cluster --config k8s/kind.yaml

helm repo add haproxytech https://haproxytech.github.io/helm-charts
helm install haproxy haproxytech/kubernetes-ingress -n haproxy --create-namespace -f k8s/haproxy-values.yaml --wait

helm repo add metrics-server https://kubernetes-sigs.github.io/metrics-server/
helm install metrics-server metrics-server/metrics-server -n kube-system --set 'args={--kubelet-insecure-tls}' --wait
```

## 2. Day one: one pod

```bash
docker build -t location-service:v1 app
kind load docker-image location-service:v1 --name uber
kubectl apply -f k8s/app.yaml
```

Open http://localhost:8088 for the live map, then send 50 drivers:

```bash
MAX=50 RAMP=10s k6 run k6/drivers.js
```

## 3. Friday night: the pod runs out of memory

```bash
ulimit -n 65000
MAX=7000 RAMP=3m k6 run k6/drivers.js
kubectl get pods -w        # watch for OOMKilled
```

## 4. Three pods: riders see only part of the drivers

```bash
kubectl scale deploy/location-service --replicas=3
MAX=3000 RAMP=1m k6 run k6/drivers.js     # look at rider_drivers_seen
```

## 5. Redis pub/sub: every pod sees every driver

```bash
kubectl apply -f k8s/redis.yaml
kubectl set env deploy/location-service REDIS_ADDR=redis:6379
MAX=3000 RAMP=1m k6 run k6/drivers.js     # rider_drivers_seen goes back up
```

## 6. Autoscaling

```bash
kubectl scale deploy/location-service --replicas=1
kubectl apply -f k8s/hpa.yaml
MAX=10000 RAMP=4m k6 run k6/drivers.js
kubectl get hpa,pods -w
```

The exact numbers depend on your machine.

## Clean up

```bash
kind delete cluster --name uber
```
