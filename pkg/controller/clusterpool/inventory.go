package clusterpool

const (
	ClusterNamesAnnotationKey = "hive.openshift.io/cluster-names"
)

// YOU ARE HERE
// Struct for the name: namespace dict
// Methods to:
// - find an unclaimed name
// - claim a name: takes "no" args, updates the map (and pushes to server?), returns (name, namespace)
// - release a name: updates the map (and pushes to server?)
// - (this is a weird one) reconcile claimed-but-unused names? (Will need the cdCollection.)