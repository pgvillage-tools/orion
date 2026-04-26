package integration

import cluster "github.com/pgvillage-tools/orion/api/v1"

var (
	newCluster      = cluster.New
	pitrCluster     = cluster.PITR
	existingCluster = cluster.ExistingCluster
	replica         = cluster.Replica
)
