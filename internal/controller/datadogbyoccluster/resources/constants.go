// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

const (
	IndexerComponentName           = "indexer"
	SearcherComponentName          = "searcher"
	MetastoreComponentName         = "metastore"
	ControlPlaneComponentName      = "control-plane"
	JanitorComponentName           = "janitor"
	ReadOnlyMetastoreComponentName = "read-only-metastore"
	CompactorComponentName         = "compactor"
)

const (
	quickwitIndexerServiceName            = "indexer"
	quickwitSearcherServiceName           = "searcher"
	quickwitMetastoreServiceName          = "metastore"
	quickwitControlPlaneServiceName       = "control_plane"
	quickwitJanitorServiceName            = "janitor"
	quickwitReadOnlyMetastoreServiceName  = "metastore_read_replica"
	quickwitCompactorServiceName          = "compactor"
	quickwitReadOnlyMetastoreURIEnvName   = "QW_METASTORE_READ_REPLICA_URI"
	quickwitUseReadOnlyMetastoreConfigKey = "use_metastore_read_replica"
)

const (
	quickwitDirectory  = "/quickwit/"
	nodeConfigFileName = "node.yaml"
	nodeConfigPath     = quickwitDirectory + nodeConfigFileName
	defaultDataPath    = quickwitDirectory + "qwdata"
)

const (
	restPort      int32 = 7280
	grpcPort      int32 = 7281
	gossipPort    int32 = 7282
	cloudpremPort int32 = 7283
	healthPort    int32 = 7284
)

const (
	appName                  = "cloudprem"
	configChecksumAnnotation = "checksum/config"
	defaultClusterDomain     = "cluster.local"
	bytesPerGiB              = 1024 * 1024 * 1024
)
