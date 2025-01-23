#!/bin/bash
set -e

# 1. Create a Kind cluster
kind create cluster --config=test/kind-config.yaml
nodes=$(docker ps --filter "label=io.x-k8s.kind.role" --format "{{.Names}}")
for node in $nodes; do
    echo "Enabling conntrack counters on $node..."
    docker exec $node sh -c 'echo "1" > /proc/sys/net/netfilter/nf_conntrack_acct'
done

# Cleanup function
cleanup() {
    echo "Cleaning up test environment..."
    echo "Collecting kubenetmon-server logs..."
    kubectl logs -n kubenetmon-server -l app.kubernetes.io/name=kubenetmon-server --tail=-1 || echo "Failed to get server logs"
    echo "Collecting kubenetmon-agent logs..."
    kubectl logs -n kubenetmon-agent -l app.kubernetes.io/name=kubenetmon-agent --tail=-1 || echo "Failed to get agent logs"
    kind delete cluster
}

# Register cleanup for interrupts and termination
trap cleanup EXIT INT TERM

# 2. Install ClickHouse
docker pull clickhouse/clickhouse-server:latest
kind load docker-image --name kind clickhouse/clickhouse-server:latest
kubectl apply -f test/clickhouse-deployment.yaml
echo "Waiting for ClickHouse pod to be ready..."
kubectl wait --namespace default --for=condition=ready pod -l app=clickhouse --timeout=120s
clickhouse_pod=$(kubectl get pods -l app=clickhouse -o jsonpath="{.items[0].metadata.name}")

# Execute the SQL command
kubectl exec -i "$clickhouse_pod" -- clickhouse-client --query="$(cat test/network_flows_0.sql)"

# 3. Build kubenetmon docker image
docker build -t local/kubenetmon:1.0.0 .

# 4. Deploy server Helm chart
kubectl create namespace kubenetmon-server
kind load docker-image --name kind local/kubenetmon:1.0.0
helm template kubenetmon-server ./deploy/helm/kubenetmon-server \
    -f ./deploy/helm/kubenetmon-server/values.yaml \
    --set image.tag=1.0.0 \
    --set deployment.replicaCount=1 \
    --set inserter.batchSize=10 \
    --set inserter.batchSendTimeout=1s \
    --set inserter.disableTLS=true \
    --set region=us-west-2 \
    --set cluster=cluster \
    --set environment=development \
    --set cloud=aws \
    --namespace=kubenetmon-server | kubectl apply -n kubenetmon-server -f -

# 5. Deploy agent Helm chart
kubectl create namespace kubenetmon-agent
kind load docker-image --name kind local/kubenetmon:1.0.0
helm template kubenetmon-agent ./deploy/helm/kubenetmon-agent \
    -f ./deploy/helm/kubenetmon-agent/values.yaml \
    --set image.tag=1.0.0 \
    --set configuration.collectionInterval=1s \
    --set configuration.skipConntrackSanityCheck=true \
    --namespace=kubenetmon-agent | kubectl apply -n kubenetmon-agent -f -
echo "Waiting for kubenetmon-agent pods to be ready..."
kubectl wait --namespace kubenetmon-agent --for=condition=ready pod -l app.kubernetes.io/name=kubenetmon-agent --timeout=180s

echo "Kind cluster setup complete. Run 'kubectl get pods --all-namespaces' to verify."

echo "Sleeping 60 seconds to let some traffic flow"
sleep 60

# Capture logs for debugging in CI
echo "Fetching kubenetmon-server logs..."
kubectl logs -n kubenetmon-server -l app.kubernetes.io/name=kubenetmon-server --tail=-1 || echo "Failed to fetch server logs"

echo "Fetching kubenetmon-agent logs..."
kubectl logs -n kubenetmon-agent -l app.kubernetes.io/name=kubenetmon-agent --tail=-1 || echo "Failed to fetch agent logs"

# Port forwarding for integration tests
echo "Configure port forwarding"
kubectl port-forward svc/clickhouse 9000:9000 &

if ! go test ./integration -v -tags 'integration' -v; then
    echo "Tests failed!"
    exit 1
fi

echo "Tests passed!"

exit 0
