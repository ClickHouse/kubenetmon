# kubenetmon

## What is kubenetmon?
`kubenetmon` is a service built and used at [ClickHouse](clickhouse.com) for Kubernetes data transfer metering in all 3 major cloud providers: AWS, GCP, and Azure.

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
- `kubenetmon-agent` is a DaemonSet that collects information about connections on a node and forwards connection records to `kubenetmon-server` over gRPC.
- `kubenetmon-server` is a ReplicaSet that watches the state of the Kubernetes cluster, attributes connection records to Kubernetes workloads, and inserts the records into ClickHouse.

The final component, ClickHouse, which we use as the destination of our data and an analytics engine, can be self-hosted or run in [ClickHouse Cloud](clickhouse.cloud).

### Implementation
`kubenetmon-agent` gets connection information from Linux's conntrack (if you can use `iptables`, you can use `kubenetmon`). At ClickHouse Cloud, we use it alongside Cilium's legacy host-routing.

## Using kubenetmon
TODO

## Testing
TODO

## Contributing
TODO

## Notes
TODO
