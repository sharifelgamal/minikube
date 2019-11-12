/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"net"

	"github.com/blang/semver"
	"k8s.io/minikube/pkg/util"
)

// Profile represents a minikube profile
type Profile struct {
	Name   string
	Config []*MachineConfig
}

// MachineConfig contains the parameters used to start a cluster.
type MachineConfig struct {
	Name                string
	KeepContext         bool // used by start and profile command to or not to switch kubectl's current context
	EmbedCerts          bool // used by kubeconfig.Setup
	MinikubeISO         string
	Memory              int
	CPUs                int
	DiskSize            int
	VMDriver            string
	ContainerRuntime    string
	HyperkitVpnKitSock  string   // Only used by the Hyperkit driver
	HyperkitVSockPorts  []string // Only used by the Hyperkit driver
	DockerEnv           []string // Each entry is formatted as KEY=VALUE.
	InsecureRegistry    []string
	RegistryMirror      []string
	HostOnlyCIDR        string // Only used by the virtualbox driver
	HypervVirtualSwitch string
	KVMNetwork          string             // Only used by the KVM driver
	KVMQemuURI          string             // Only used by kvm2
	KVMGPU              bool               // Only used by kvm2
	KVMHidden           bool               // Only used by kvm2
	Downloader          util.ISODownloader `json:"-"`
	DockerOpt           []string           // Each entry is formatted as KEY=VALUE.
	DisableDriverMounts bool               // Only used by virtualbox
	NFSShare            []string
	NFSSharesRoot       string
	UUID                string // Only used by hyperkit to restore the mac address
	NoVTXCheck          bool   // Only used by virtualbox
	DNSProxy            bool   // Only used by virtualbox
	HostDNSResolver     bool   // Only used by virtualbox
	KubernetesConfig    KubernetesConfig
}

// KubernetesConfig contains the parameters used to configure the VM Kubernetes.
type KubernetesConfig struct {
	KubernetesVersion string
	NodeIP            string
	NodePort          int
	NodeName          string
	APIServerName     string
	APIServerNames    []string
	APIServerIPs      []net.IP
	DNSDomain         string
	ContainerRuntime  string
	CRISocket         string
	NetworkPlugin     string
	FeatureGates      string
	ServiceCIDR       string
	ImageRepository   string
	ExtraOptions      ExtraOptionSlice
	BootstrapToken    string

	ShouldLoadCachedImages bool
	EnableDefaultCNI       bool
}

// VersionedExtraOption holds information on flags to apply to a specific range
// of versions
type VersionedExtraOption struct {
	// Special Cases:
	//
	// If LessThanOrEqual and GreaterThanOrEqual are both nil, the flag will be applied
	// to all versions
	//
	// If LessThanOrEqual == GreaterThanOrEqual, the flag will only be applied to that
	// specific version

	// The flag and component that will be set
	Option ExtraOption

	// This flag will only be applied to versions before or equal to this version
	// If it is the default value, it will have no upper bound on versions the
	// flag is applied to
	LessThanOrEqual semver.Version

	// The flag will only be applied to versions after or equal to this version
	// If it is the default value, it will have no lower bound on versions the
	// flag is applied to
	GreaterThanOrEqual semver.Version
}
