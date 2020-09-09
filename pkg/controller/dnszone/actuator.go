package dnszone

// Actuator interface is the interface that is used to add dns provider support to the dnszone controller.
type Actuator interface {
	// Create tells the actuator to make a zone in the dns provider.
	// The platform-specific DNSZone status fields will be populated (eg the zone name).
	Create() error

	// Delete tells the actuator to remove the zone from the dns provider.
	Delete() error

	// Exists queries if the zone is in the dns provider.
	Exists() (bool, error)

	// UpdateMetadata tells the actuator to update the zone's metadata in the dns provider.
	UpdateMetadata() error

	// GetNameServers returns a list of nameservers that service the zone in the dns provider.
	GetNameServers() ([]string, error)

	// Refresh signals to the actuator that it should get the latest version of the zone from the dns provider.
	// Refresh MUST be called before any other function is called by the actuator.
	// Refresh will update the DNSZone object's platform-specific status fields.
	Refresh() error

	// SetConditionsForError sets conditions on the dnszone given a specific error
	SetConditionsForError(err error) bool
}
