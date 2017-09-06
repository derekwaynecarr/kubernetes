// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package crio

import (
	"flag"
	"fmt"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/google/cadvisor/container"
	"github.com/google/cadvisor/container/libcontainer"
	"github.com/google/cadvisor/fs"
	info "github.com/google/cadvisor/info/v1"
	"github.com/google/cadvisor/manager/watcher"

	"github.com/golang/glog"
)

var ArgCrioEndpoint = flag.String("crio", "unix:///var/run/crio.sock", "crio endpoint")

// The namespace under which crio aliases are unique.
const CrioNamespace = "crio"

// Regexp that identifies docker cgroups, containers started with
// --cgroup-parent have another prefix than 'docker'
var crioCgroupRegexp = regexp.MustCompile(`([a-z0-9]{64})`)

var (
	// Basepath to all container specific information that libcontainer stores.
	crioRootDir string

	crioRootDirOnce sync.Once
)

func RootDir() string {
	crioRootDirOnce.Do(func() {
		// TODO: is there a crio info call we should make
		crioRootDir = "/var/lib/containers"
	})
	return crioRootDir
}

type storageDriver string

const (
	// TODO add full set of supported drivers in future..
	overlayStorageDriver  storageDriver = "overlay"
	overlay2StorageDriver storageDriver = "overlay2"
)

type crioFactory struct {
	machineInfoFactory info.MachineInfoFactory

	storageDriver storageDriver
	storageDir    string

	// Information about the mounted cgroup subsystems.
	cgroupSubsystems libcontainer.CgroupSubsystems

	// Information about mounted filesystems.
	fsInfo fs.FsInfo

	ignoreMetrics container.MetricSet
}

func (self *crioFactory) String() string {
	return CrioNamespace
}

func (self *crioFactory) NewContainerHandler(name string, inHostNamespace bool) (handler container.ContainerHandler, err error) {
	// TODO if we have a crio-client, configure it here
	// TODO are there any env vars we need to white list, if so, do it here...
	metadataEnvs := []string{}
	handler, err = newCrioContainerHandler(
		name,
		self.machineInfoFactory,
		self.fsInfo,
		self.storageDriver,
		self.storageDir,
		&self.cgroupSubsystems,
		inHostNamespace,
		metadataEnvs,
		self.ignoreMetrics,
	)
	return
}

// Returns the CRIO ID from the full container name.
func ContainerNameToCrioId(name string) string {
	id := path.Base(name)

	if matches := crioCgroupRegexp.FindStringSubmatch(id); matches != nil {
		return matches[1]
	}

	return id
}

// isContainerName returns true if the cgroup with associated name
// corresponds to a crio container.
func isContainerName(name string) bool {
	// always ignore .mount cgroup even if associated with crio and delegate to systemd
	if strings.HasSuffix(name, ".mount") {
		return false
	}
	return crioCgroupRegexp.MatchString(path.Base(name))
}

// crio handles all containers under /crio
func (self *crioFactory) CanHandleAndAccept(name string) (bool, bool, error) {
	glog.Infof("CRIO CAN HANDLE AND ACCEPT: %v", name)
	if strings.HasPrefix(path.Base(name), "crio-conman") {
		glog.Info("SKIPPING CRIO-CONMON")
	}
	if !strings.HasPrefix(path.Base(name), CrioNamespace) {
		return false, false, nil
	}
	// if the container is not associated with docker, we can't handle it or accept it.
	if !isContainerName(name) {
		return false, false, nil
	}
	glog.Infof("CRIO HANDLE AND ACCEPT: %v", name)
	// TODO should we call equivalent of a crio info to be sure its really ours
	// and to know if the container is running...
	return true, true, nil
}

func (self *crioFactory) DebugInfo() map[string][]string {
	return map[string][]string{}
}

var (
	version_regexp_string    = `(\d+)\.(\d+)\.(\d+)`
	version_re               = regexp.MustCompile(version_regexp_string)
	apiversion_regexp_string = `(\d+)\.(\d+)`
	apiversion_re            = regexp.MustCompile(apiversion_regexp_string)
)

// Register root container before running this function!
func Register(factory info.MachineInfoFactory, fsInfo fs.FsInfo, ignoreMetrics container.MetricSet) error {
	// TODO initialize any client we will use to speak to crio
	// runcom mrunal -- ideally, we read /etc/crio/crio.conf here so we know how machine is configured
	// i.e. what is the storage driver, etc.
	// TODO determine crio version so we can work differently w/ future versions if needed
	cgroupSubsystems, err := libcontainer.GetCgroupSubsystems()
	if err != nil {
		return fmt.Errorf("failed to get cgroup subsystems: %v", err)
	}

	// TODO: FIX ME mrunal / runcom so this is read from crio.conf
	storageDriver := overlayStorageDriver
	storageDir := RootDir()

	glog.Infof("Registering CRI-O factory")
	f := &crioFactory{
		cgroupSubsystems:   cgroupSubsystems,
		fsInfo:             fsInfo,
		machineInfoFactory: factory,
		storageDriver:      storageDriver,
		storageDir:         storageDir,
		ignoreMetrics:      ignoreMetrics,
	}

	container.RegisterContainerHandlerFactory(f, []watcher.ContainerWatchSource{watcher.Raw})
	return nil
}
