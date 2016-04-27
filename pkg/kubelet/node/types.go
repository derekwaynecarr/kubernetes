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

package node

import (
	"time"

	"k8s.io/kubernetes/pkg/api"
)

// NodeStatusFunc is a function that modifes the specified node's status.
type NodeStatusFunc func(node *api.Node) error

// NodeStatusTarget maintains a list of funcs to invoke when defining node status.
type NodeStatusTarget interface {
	// AddNodeStatusFunc adds the specified function.
	AddNodeStatusFunc(nodeStatusFunc NodeStatusFunc)
}

// NodeStatusFuncs maintains a list of funcs to invoke for node status.
type NodeStatusFuncs []NodeStatusFunc

// AddNodeStatusFunc adds the specified function.
func (f *NodeStatusFuncs) AddNodeStatusFunc(a NodeStatusFunc) {
	*f = append(*f, a)
}

// Manager is responsible for updating node status at specified frequency.
type Manager interface {
	NodeStatusTarget

	// Start the node status manager.
	// nodeStatusUpdateFrequency specifies how often kubelet posts node status to master.
	// Note: be cautious when changing the constant, it must work with nodeMonitorGracePeriod
	// in nodecontroller. There are several constraints:
	// 1. nodeMonitorGracePeriod must be N times more than nodeStatusUpdateFrequency, where
	//    N means number of retries allowed for kubelet to post node status. It is pointless
	//    to make nodeMonitorGracePeriod be less than nodeStatusUpdateFrequency, since there
	//    will only be fresh values from Kubelet at an interval of nodeStatusUpdateFrequency.
	//    The constant must be less than podEvictionTimeout.
	// 2. nodeStatusUpdateFrequency needs to be large enough for kubelet to generate node
	//    status. Kubelet may fail to update node status reliably if the value is too small,
	//    as it takes time to gather all necessary node information.
	Start(nodeStatusUpdateFrequency time.Duration)
}
