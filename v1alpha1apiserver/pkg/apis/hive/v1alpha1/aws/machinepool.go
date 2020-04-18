package aws

// MachinePoolPlatform stores the configuration for a machine pool
// installed on AWS.
type MachinePoolPlatform struct {
	// Zones is list of availability zones that can be used.
	Zones []string `json:"zones,omitempty"`

	// InstanceType defines the ec2 instance type.
	// eg. m4-large
	InstanceType string `json:"type"`

	// EC2RootVolume defines the storage for ec2 instance.
	EC2RootVolume `json:"rootVolume"`
}

// EC2RootVolume defines the storage for an ec2 instance.
type EC2RootVolume struct {
	// IOPS defines the iops for the storage.
	IOPS int `json:"iops"`
	// Size defines the size of the storage.
	Size int `json:"size"`
	// Type defines the type of the storage.
	Type string `json:"type"`
}
