package eksconfig

// Instance represents an EC2 instance.
type Instance struct {
	ImageID                string                  `json:"image-id,omitempty"`
	InstanceID             string                  `json:"instance-id,omitempty"`
	InstanceType           string                  `json:"instance-type,omitempty"`
	KeyName                string                  `json:"key-name,omitempty"`
	Placement              EC2Placement            `json:"placement,omitempty"`
	PrivateDNSName         string                  `json:"private-dns-name,omitempty"`
	PrivateIP              string                  `json:"private-ip,omitempty"`
	PublicDNSName          string                  `json:"public-dns-name,omitempty"`
	PublicIP               string                  `json:"public-ip,omitempty"`
	EC2State               EC2State                `json:"state,omitempty"`
	SubnetID               string                  `json:"subnet-id,omitempty"`
	VPCID                  string                  `json:"vpc-id,omitempty"`
	EC2BlockDeviceMappings []EC2BlockDeviceMapping `json:"block-device-mappings,omitempty"`
	EBSOptimized           bool                    `json:"ebs-optimized"`
	RootDeviceName         string                  `json:"root-device-name,omitempty"`
	RootDeviceType         string                  `json:"root-device-type,omitempty"`
	SecurityGroups         []EC2SecurityGroup      `json:"security-groups,omitempty"`
}

// EC2Placement defines EC2 placement.
type EC2Placement struct {
	AvailabilityZone string `json:"availability-zone,omitempty"`
	Tenancy          string `json:"tenancy,omitempty"`
}

// EC2State defines an EC2 state.
type EC2State struct {
	Code int64  `json:"code,omitempty"`
	Name string `json:"name,omitempty"`
}

// EC2BlockDeviceMapping defines a block device mapping.
type EC2BlockDeviceMapping struct {
	DeviceName string `json:"device-name,omitempty"`
	EBS        EBS    `json:"ebs,omitempty"`
}

// EBS defines an EBS volume.
type EBS struct {
	DeleteOnTermination bool   `json:"delete-on-termination,omitempty"`
	Status              string `json:"status,omitempty"`
	VolumeID            string `json:"volume-id,omitempty"`
}

// EC2SecurityGroup defines a security group.
type EC2SecurityGroup struct {
	GroupName string `json:"group-name,omitempty"`
	GroupID   string `json:"group-id,omitempty"`
}
