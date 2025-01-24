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
