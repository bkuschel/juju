// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"github.com/juju/juju/container"
)

var (
	ContainerConfigFilename = containerConfigFilename
	ContainerDirFilesystem  = containerDirFilesystem
	GenerateNetworkConfig   = generateNetworkConfig
	DiscoverHostNIC         = &discoverHostNIC
	NetworkConfigTemplate   = networkConfigTemplate
	RestartSymlink          = restartSymlink
	ReleaseVersion          = &releaseVersion
	PreferFastLXC           = preferFastLXC
	InitProcessCgroupFile   = &initProcessCgroupFile
	RuntimeGOOS             = &runtimeGOOS
	WriteWgetTmpFile        = &writeWgetTmpFile
)

func GetCreateWithCloneValue(mgr container.Manager) bool {
	return mgr.(*containerManager).createWithClone
}

func WgetEnvironment(caCert []byte) ([]string, func(), error) {
	return wgetEnvironment(caCert)
}
