# kubenetmon

## What is kubenetmon?
`kubenetmon` is a service built and used at [ClickHouse](https://clickhouse.com) for Kubernetes data transfer metering in all 3 major cloud providers: AWS, GCP, and Azure.

`kubenetmon` is packaged as a Helm chart with a Docker image. The chart is available at [https://kubenetmon.clickhouse.tech/index.yaml](https://kubenetmon.clickhouse.tech/index.yaml). See below for detailed usage instructions.

## What can kubenetmon be used for?
At ClickHouse Cloud, we use `kubenetmon` to meter data transfer of all of our workloads running in Kubernetes. With the data `kubenetmon` collects and stores in ClickHouse, we are able to answer questions such as:
1. How much cross-Availability Zone traffic are our workloads sending and which workloads are the largest talkers?
2. How much traffic are we sending to S3?
3. Which workloads open outbound connections, and which workloads only receive inbound connections?
4. Are gRPC connections of our internal clients balanced across internal server replicas?
5. What are our throughput needs and are we at risk of exhausting instance bandwidth limits imposed on us by CSPs?

## How does kubenetmon work?
### Components
`kubenetmon` consists of two components:
- `kubenetmon-agent` is a DaemonSet that collects information about connections on a node and forwards connection records to `kubenetmon-server` over gRPC. `kubenetmon-agent` gets connection information from Linux's conntrack (if you can use `iptables` with your CNI, you can use `kubenetmon`).
- `kubenetmon-server` is a ReplicaSet that watches the state of the Kubernetes cluster, attributes connection records to Kubernetes workloads, and inserts the records into ClickHouse.

The final component, ClickHouse, which we use as the destination of our data and an analytics engine, can be self-hosted or run in [ClickHouse Cloud](clickhouse.cloud).

## Using kubenetmon
`kubenetmon` comes in two Helm charts, `kubenetmon-server` and `kubenetmon-agent`. Both use the same Docker image. Starting with `kubenetmon` is very easy.

First, create a ClickHouse service in [ClickHouse Cloud](clickhouse.cloud). You can try it out for free with a $300 credit! In the new service (or an existing one, if you are already running ClickHouse), create `default.network_flows_0` table with this query (you can also find it in `test/network_flows_0.sql`):
```
CREATE TABLE default.network_flows_0
(
    `date` Date CODEC(Delta(2), ZSTD(1)),
    `intervalStartTime` DateTime CODEC(Delta(4), ZSTD(1)),
    `intervalSeconds` UInt16 CODEC(Delta(2), ZSTD(1)),
    `environment` LowCardinality(String) CODEC(ZSTD(1)),
    `proto` LowCardinality(String) CODEC(ZSTD(1)),
    `connectionClass` LowCardinality(String) CODEC(ZSTD(1)),
    `connectionFlags` Map(LowCardinality(String), Bool) CODEC(ZSTD(1)),
    `direction` Enum('out' = 1, 'in' = 2) CODEC(ZSTD(1)),
    `localCloud` LowCardinality(String) CODEC(ZSTD(1)),
    `localRegion` LowCardinality(String) CODEC(ZSTD(1)),
    `localCluster` LowCardinality(String) CODEC(ZSTD(1)),
    `localCell` LowCardinality(String) CODEC(ZSTD(1)),
    `localAvailabilityZone` LowCardinality(String) CODEC(ZSTD(1)),
    `localNode` String CODEC(ZSTD(1)),
    `localInstanceID` String CODEC(ZSTD(1)),
    `localNamespace` LowCardinality(String) CODEC(ZSTD(1)),
    `localPod` String CODEC(ZSTD(1)),
    `localIPv4` IPv4 CODEC(Delta(4), ZSTD(1)),
    `localPort` UInt16 CODEC(Delta(2), ZSTD(1)),
    `localApp` String CODEC(ZSTD(1)),
    `remoteCloud` LowCardinality(String) CODEC(ZSTD(1)),
    `remoteRegion` LowCardinality(String) CODEC(ZSTD(1)),
    `remoteCluster` LowCardinality(String) CODEC(ZSTD(1)),
    `remoteCell` LowCardinality(String) CODEC(ZSTD(1)),
    `remoteAvailabilityZone` LowCardinality(String) CODEC(ZSTD(1)),
    `remoteNode` String CODEC(ZSTD(1)),
    `remoteInstanceID` String CODEC(ZSTD(1)),
    `remoteNamespace` LowCardinality(String) CODEC(ZSTD(1)),
    `remotePod` String CODEC(ZSTD(1)),
    `remoteIPv4` IPv4 CODEC(Delta(4), ZSTD(1)),
    `remotePort` UInt16 CODEC(Delta(2), ZSTD(1)),
    `remoteApp` String CODEC(ZSTD(1)),
    `remoteCloudService` LowCardinality(String) CODEC(ZSTD(1)),
    `bytes` UInt64 CODEC(Delta(8), ZSTD(1)),
    `packets` UInt64 CODEC(Delta(8), ZSTD(1))
)
ENGINE = SummingMergeTree((bytes, packets))
PARTITION BY date
PRIMARY KEY (date, intervalStartTime, direction, proto, localApp, remoteApp, localPod, remotePod)
ORDER BY (date, intervalStartTime, direction, proto, localApp, remoteApp, localPod, remotePod, intervalSeconds, environment, connectionClass, connectionFlags, localCloud, localRegion, localCluster, localCell, localAvailabilityZone, localNode, localInstanceID, localNamespace, localIPv4, localPort, remoteCloud, remoteRegion, remoteCluster, remoteCell, remoteAvailabilityZone, remoteNode, remoteInstanceID, remoteNamespace, remoteIPv4, remotePort, remoteCloudService)
TTL intervalStartTime + toIntervalDay(90)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;
```

All you now need is a Kubernetes cluster where you want to meter data transfer.

(**Optional**) If you don't have a test k8s cluster, you can spin up a `kind` cluster using config in this repository like so:
```
kind create cluster --config=test/kind-config.yaml
```

(**Optional**) And if you don't have many workloads running in the cluster, you can install some mock services:
```
helm repo add podinfo https://stefanprodan.github.io/podinfo
helm upgrade --install --wait backend --namespace default --set redis.enabled=true podinfo/podinfo
helm upgrade --install --wait frontend --namespace default --set redis.enabled=true podinfo/podinfo
```

Next, we create two namespaces:
```
kubectl create namespace kubenetmon-server
kubectl create namespace kubenetmon-agent
```

Let's add this Helm repository:
```
helm repo add kubenetmon https://kubenetmon.clickhouse.tech
```

We now install `kubenetmon-server`. `kubenetmon-server` expects an environment, cluster name, cloud provider name (`aws`, `gcp`, or `azure`), and region, so we provide these. We are also going to supply connection credentials for our ClickHouse instance:
```
helm install kubenetmon-server kubenetmon/kubenetmon-server \
--namespace kubenetmon-server \
--set region=us-west-2 \
--set cluster=cluster \
--set environment=development \
--set cloud=aws \
--set inserter.endpoint=pwlo4fffj6.eu-west-2.aws.clickhouse.cloud:9440 \
--set inserter.username=default \
--set inserter.password=etK0~PWR7DgRA \
--set deployment.replicaCount=1
```

We see from the logs that the replica started and connected to our ClickHouse instance:
```
➜  ~ kubectl logs -f kubenetmon-server-6d5ff494fb-wvs8d -n kubenetmon-server
{"level":"info","time":"2025-01-23T20:55:10Z","message":"GOMAXPROCS: 2\n"}
{"level":"info","time":"2025-01-23T20:55:10Z","message":"GOMEMLIMIT: 2634022912\n"}
{"level":"info","time":"2025-01-23T20:55:10Z","message":"There are currently 18 pods, 5 nodes, and 3 services in the cluster cluster!"}
{"level":"info","time":"2025-01-23T20:55:12Z","message":"RemoteLabeler initialized with 43806 prefixes"}
{"level":"info","time":"2025-01-23T20:55:12Z","message":"Beginning to serve metrics on port :8883/metrics\n"}
{"level":"info","time":"2025-01-23T20:55:12Z","message":"Beginning to serve flowHandlerServer on port :8884\n"}
```

All that's left is to deploy `kubenetmon-agent`, a DaemonSet that will track
connections on nodes. `kubenetmon-agent` relies on Linux's `conntrack` reporting
byte and packet counters; by default, this feature is most likely disabled, so
you need to enable it on the nodes with:
```
/bin/echo "1" > /proc/sys/net/netfilter/nf_conntrack_acct
```
**This is an important step, don't skip it!**

For example, to test getting data transfer information from all nodes in the kind cluster, you can run:
```
for node in $(kubectl get nodes -o name); do
  kubectl node-shell ${node##node/} -- /bin/sh -c '/bin/echo "1" > /proc/sys/net/netfilter/nf_conntrack_acct'
done
```

Nodes are now ready to host `kubenetmon-agent`, so let's install it.
```
helm install kubenetmon-agent kubenetmon/kubenetmon-agent --namespace kubenetmon-agent
```

Let's check the logs:
```
➜  ~ kubectl logs -f kubenetmon-agent-6dsnr -n kubenetmon-agent
{"level":"info","time":"2025-01-23T21:00:14Z","message":"GOMAXPROCS: 1\n"}
{"level":"info","time":"2025-01-23T21:00:14Z","message":"GOMEMLIMIT: 268435456\n"}
{"level":"info","time":"2025-01-23T21:00:14Z","message":"Creating kubenetmon-server (kubenetmon-server.kubenetmon-server.svc.cluster.local:8884) gRPC client"}
{"level":"info","time":"2025-01-23T21:00:14Z","message":"Connected to kubenetmon-server"}
{"level":"info","time":"2025-01-23T21:00:14Z","message":"Confirmed that kubenetmon can retrieve conntrack packet and byte counters"}
{"level":"info","time":"2025-01-23T21:00:14Z","message":"Beginning to serve metrics on port :8883/metrics\n"}
{"level":"info","time":"2025-01-23T21:00:14Z","message":"Starting flow collector"}
{"level":"info","time":"2025-01-23T21:00:14Z","message":"Starting collection loop with 5s interval"}
{"level":"info","time":"2025-01-23T21:00:14Z","message":"24 datapoints were accepted through the last stream"}
{"level":"info","time":"2025-01-23T21:00:19Z","message":"1 datapoints were accepted through the last stream"}
{"level":"info","time":"2025-01-23T21:00:24Z","message":"2 datapoints were accepted through the last stream"}
{"level":"info","time":"2025-01-23T21:00:29Z","message":"3 datapoints were accepted through the last stream"}
{"level":"info","time":"2025-01-23T21:00:34Z","message":"3 datapoints were accepted through the last stream"}
```

If you see log lines such as `conntrack is reporting empty flow counters`, this means you didn't enable conntrack counters with `sysctl` as above. `kubenetmon-agent` performs a sanity check on startup to confirm that the counters are enabled; you can disable the check with `--set configuration.skipConntrackSanityCheck=true`, but in this case you won't get any data.

If we check `kubenetmon-server` logs again, we'll see it's sending data to ClickHouse:
```
Inserted batch due to reaching max batch age
```

Let's now run a query in ClickHouse Cloud against our `network_flows_0` table:
```
SELECT localPod, remotePod, connectionClass, formatReadableSize(sum(bytes))
FROM default.network_flows_0
WHERE date = today() AND intervalStartTime > NOW() - INTERVAL 10 MINUTES AND direction = 'out'
GROUP BY localPod, remotePod, connectionClass
ORDER BY sum(bytes) DESC;
```

```

+-----------------------------------------+-----------------------------------------+-----------------+--------------------------------+
|                localPod                 |                remotePod                | connectionClass | formatReadableSize(sum(bytes)) |
+-----------------------------------------+-----------------------------------------+-----------------+--------------------------------+
| kubenetmon-server-6d5ff494fb-wvs8d      | ''                                      | INTER_REGION    | 166.71 KiB                     |
| kubenetmon-server-6d5ff494fb-wvs8d      | ''                                      | INTRA_VPC       | 19.24 KiB                      |
| backend-podinfo-redis-5d6c77b77c-t5vfh  | ''                                      | INTRA_VPC       | 2.46 KiB                       |
| frontend-podinfo-redis-546897f5bc-hqsml | ''                                      | INTRA_VPC       | 2.46 KiB                       |
| frontend-podinfo-5b58f98bbf-bfsw6       | frontend-podinfo-redis-546897f5bc-hqsml | INTRA_VPC       | 2.06 KiB                       |
| backend-podinfo-7fc7494945-pcj8h        | backend-podinfo-redis-5d6c77b77c-t5vfh  | INTRA_VPC       | 2.05 KiB                       |
| frontend-podinfo-redis-546897f5bc-hqsml | frontend-podinfo-5b58f98bbf-bfsw6       | INTRA_VPC       | 865.00 B                       |
+-----------------------------------------+-----------------------------------------+-----------------+--------------------------------+
```

Looks like `kubenetmon-server` is sending some data to a different AWS region. This is accurate, because for this experiment we configured a ClickHouse instance in AWS and configured `kubenetmon-server` to think it's running in AWS us-west-2.

## Testing
To run integration tests, run `make integration-test`. For unit tests, run `make test`. Note that unit tests for `kubenetmon-agent` can only be run on Linux (you need netlink for it).

## Contributing
To contribute, simply open a pull request against `main` in the repository. Changes are very welcome.

## Notes
1. `kubenetmon` does not meter traffic from pods with host network.
2. For a connection between two pods in the VPC, `kubenetmon-agent` will see each packet twice – once at the sender and once at the receiver. Use the `direction` filter if you want to filter for just one observation point.
3. `kubenetmon` can't automatically detect NLBs, NAT Gateways, etc (you are welcome to contribute these changes!), but if these are easily identifiable in your infrastructure (for example, with a dedicated IP range), you can modify the code to populate the `connectionFlags` field as you see fit to record arbitrary data about your connections.
