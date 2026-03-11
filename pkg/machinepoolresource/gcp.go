package machinepoolresource

import (
	"strings"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
)

const (
	// DefaultGCPInstanceType is the default GCP instance type for MachinePool.
	DefaultGCPInstanceType = "n1-standard-4"
	defaultGCPDiskSizeGB   = 128
	defaultGCPDiskType     = "pd-ssd"
)

// GCPOptions holds all GCP MachinePool.Spec.Platform.GCP fields.
type GCPOptions struct {
	Zones                  []string
	InstanceType           string
	OSDiskType             string
	OSDiskSizeGB           int64
	EncryptionKeyName      string
	EncryptionKeyKeyRing   string
	EncryptionKeyProjectID string
	EncryptionKeyLocation  string
	KMSKeyServiceAccount   string
	NetworkProjectID       string
	SecureBoot             string
	OnHostMaintenance      string
	ServiceAccount         string
	UserTags               []hivev1gcp.UserTag
	Tags                   []string
}

// FillPlatform sets mp.Spec.Platform.GCP from o.
func (o *GCPOptions) FillPlatform(mp *hivev1.MachinePool) {
	if o == nil {
		return
	}
	instanceType := o.InstanceType
	if instanceType == "" {
		instanceType = DefaultGCPInstanceType
	}
	diskSize := o.OSDiskSizeGB
	if diskSize <= 0 {
		diskSize = defaultGCPDiskSizeGB
	}
	diskType := o.OSDiskType
	if diskType == "" {
		diskType = defaultGCPDiskType
	}
	osDisk := hivev1gcp.OSDisk{
		DiskType:   diskType,
		DiskSizeGB: diskSize,
	}
	if o.EncryptionKeyName != "" && o.EncryptionKeyKeyRing != "" && o.EncryptionKeyLocation != "" {
		ref := &hivev1gcp.EncryptionKeyReference{
			KMSKey: &hivev1gcp.KMSKeyReference{
				Name:      o.EncryptionKeyName,
				KeyRing:   o.EncryptionKeyKeyRing,
				ProjectID: o.EncryptionKeyProjectID,
				Location:  o.EncryptionKeyLocation,
			},
		}
		if o.KMSKeyServiceAccount != "" {
			ref.KMSKeyServiceAccount = o.KMSKeyServiceAccount
		}
		osDisk.EncryptionKey = ref
	}

	plat := &hivev1gcp.MachinePool{
		Zones:             o.Zones,
		InstanceType:      instanceType,
		OSDisk:            osDisk,
		NetworkProjectID:  o.NetworkProjectID,
		SecureBoot:        o.SecureBoot,
		OnHostMaintenance: o.OnHostMaintenance,
		ServiceAccount:    o.ServiceAccount,
		UserTags:          o.UserTags,
		Tags:              o.Tags,
	}
	mp.Spec.Platform.GCP = plat
}

var _ PlatformFiller = (*GCPOptions)(nil)

// ParseGCPUserTags converts "parentID:key=value" strings into hivev1/gcp UserTag slice.
// Malformed entries are skipped.
func ParseGCPUserTags(ss []string) []hivev1gcp.UserTag {
	if len(ss) == 0 {
		return nil
	}
	out := make([]hivev1gcp.UserTag, 0, len(ss))
	for _, s := range ss {
		colon := strings.Index(s, ":")
		if colon < 0 {
			continue
		}
		parentID := strings.TrimSpace(s[:colon])
		rest := s[colon+1:]
		eq := strings.Index(rest, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(rest[:eq])
		value := strings.TrimSpace(rest[eq+1:])
		out = append(out, hivev1gcp.UserTag{ParentID: parentID, Key: key, Value: value})
	}
	return out
}
