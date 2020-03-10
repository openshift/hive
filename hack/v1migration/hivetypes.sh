NAMESPACE_SCOPED_HIVE_TYPES=( checkpoints clusterdeployments clusterdeprovisionrequests clusterprovisions clusterstates dnsendpoints dnszones syncidentityproviders syncsetinstances syncsets )

CLUSTER_SCOPED_HIVE_TYPES=( clusterimagesets hiveconfigs selectorsyncidentityproviders selectorsyncsets )

HIVE_TYPES=( "${NAMESPACE_SCOPED_HIVE_TYPES[@]}" "${CLUSTER_SCOPED_HIVE_TYPES[@]}" )
