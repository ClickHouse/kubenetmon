# kubenetmon
`kubenetmon` is a stack of services providing us with network observaility.

`kubenetmon-agent` is a daemon running on each node, scraping connection information from conntrack and sending it to `kubenetmon-server` over gRPC.

`kubenetmon-server` is a replica (one or multiple) receiving node-local conntrack records from `kubenetmon-agent`, annotating them with Kubernetes and business-logic metadata, and inserting them into ClickHouse.

A few notes on labeling:

1. All flows to/from node IPs, including flows to/from hostNetwork pods, are ignored.
2. All flows are annotated from the perspective of the "local" vs "remote" pod. The pod running on the same node as `kubenetmon` is "local". Given that `kubenetmon` runs everywhere, this means that we can see the same flow multiple times. For intra-node flows, i.e. those from and to pods on the same node, the "local" pod is whichever pod opened the connection.

## Developing and experimenting in a cluster
Generally, you can test `kubenetmon` in a `kind` cluster. Here is the full list of commands to run manu ltesting locally:
```
BASE_CONFIG_FILE=.github/workflows/configs/kind-config.yaml FINAL_CONFIG_FILE="$(.github/workflows/scripts/kind-setup.sh "${BASE_CONFIG_FILE}")"

FINAL_CONFIG_FILE="$(.github/workflows/scripts/kind-setup.sh "${BASE_CONFIG_FILE}")"

kind create cluster --config=$FINAL_CONFIG_FILE

docker build ./kubenetmon/ -t local/kubenetmon:1.0.0 

kind load docker-image --name kind local/kubenetmon:1.0.0

kubectl create namespace kubenetmon-server

kubectl create namespace kubenetmon-agent

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts

helm install prometheus-operator prometheus-community/kube-prometheus-stack

helm template kubenetmon-server ./kubenetmon/deploy/helm/kubenetmon-server -f ./kubenetmon/deploy/helm/kubenetmon-server/values.yaml --set image.tag=1.0.0 --set inserter.batchSize=10 --set inserter.disableTLS=true --set inserter.skipPing=true --set inserter.clusterType=data-plane --namespace=kubenetmon-server | kubectl apply -n kubenetmon-server -f -

helm template kubenetmon-agent ./kubenetmon/deploy/helm/kubenetmon-agent -f ./kubenetmon/deploy/helm/kubenetmon-agent/values.yaml --set image.tag=1.0.0 --namespace=kubenetmon-agent | kubectl apply -n kubenetmon-agent -f -
```

At this point, `kubenetmon-server` and `kubenetmon-agent` will be installed. They won't yet run, however. `kubenetmon-agent` needs conntrack counters to be enabled on each machine to start up, so you will want to shell into your kind nodes and enable conntrack accounting:
```
kubectl node-shell kind-worker
root@kind-worker:/# /bin/echo "1" > /proc/sys/net/netfilter/nf_conntrack_acct
```

As soon as new connections go through the node, conntrack will have at least one entry in the table with non-zero packet and byte counters, and `kubenetmon-agent` will stop refusing to start.

If you want to make a code change and test it, build the docker image again (possibly with a new tag), load it into the cluster (with the new tag as needed), and apply helm changes again (with the new tag, as needed). 

Note that this won't get you a running ClickHouse server. To spin one up in your kind cluster, run:
```
kubectl apply -f kubenetmon/helpers/clickhouse-deployment.yaml
```
You can then set up port forwarding to this ClickHouse pod and create any tables or databases as necessary in it.

## Unit testing
To run unit tests, you can simply run `make test`. Note that not all packages will be testable on MacOS machines since netlink is not available on MacOS. You will need to run unit tests on a Linux machine, which you can create using VMware (free trial has limited duration), or by simply following [this guide](https://docs.google.com/document/d/1iWN8Ocag5nK9-MxLHvwq9SD6ethbABLthTKdya3mnok/view#heading=h.eah7f64rhe0l), but don't forget to stop your instance when you're done testing!

Visual Studio Code allows you to do development and testing on a remote machine if you want to run tests whenever you make changes.

## Deploying
`kubenetmon` is built into a multiarch Docker image that runs on both `arm` and `x86` nodes. Both `kubenetmon-server` and `kubenetmon-agent` binaries are packaged into the image. A new image is built for every `kubenetmon` change merged into main and deployed with ArgoCD (automatically for AWS dev, manually via `data-plane-configuration` in all other cases).

