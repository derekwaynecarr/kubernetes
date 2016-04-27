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
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	cadvisorapi "github.com/google/cadvisor/info/v1"

	"k8s.io/kubernetes/pkg/api"
	apierrors "k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/api/unversioned"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/client/record"
	"k8s.io/kubernetes/pkg/cloudprovider"
	"k8s.io/kubernetes/pkg/kubelet/cadvisor"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/pkg/version"
)

const (
	// nodeStatusUpdateRetry specifies how many times kubelet retries when posting node status failed.
	nodeStatusUpdateRetry = 5
)

// NewManager returns a Manager.
func NewManager() Manager {
	return managerImpl{}
}

// managerImpl imlements Manager
type managerImpl struct {
	// cAdvisor used for container information.
	cadvisor cadvisor.Interface
	// clock is an interface that provides time related functionality in a way that makes it
	// easy to test the code.
	clock util.Clock
	// cloud provider interface
	cloud cloudprovider.Interface
	// Container runtime.
	containerRuntime kubecontainer.Runtime
	// Information about the ports which are opened by daemons on Node running this Kubelet server.
	daemonEndpoints *api.NodeDaemonEndpoints
	// the client to use to interface with the master.
	kubeClient clientset.Interface
	// holds the set of functions that are registered to set node status.
	NodeStatusFuncs
	// used to record events about the node
	recorder record.EventRecorder
	// Set to true to have the node register itself with the apiserver.
	registerNode        bool
	registerSchedulable bool
	// for internal book keeping; access only from within registerWithApiserver
	registrationCompleted bool
	// name of the node
	nodeName string
	// a list of node labels to register
	nodeLabels map[string]string
	// Cached MachineInfo returned by cadvisor.
	// Maximum Number of Pods which can be run by this Kubelet
	maxPods     int
	machineInfo *cadvisorapi.MachineInfo
	nodeRef     *api.ObjectReference
	hostname    string
	// reservation specifies resources which are reserved for non-pod usage, including kubernetes and
	// non-kubernetes system processes.
	reservation kubetypes.Reservation
}

// Start the node status manager.
func (m *managerImpl) Start(nodeStatusUpdateFrequency time.Duration) {
	// Start syncing node status immediately, this may set up things the runtime needs to run.
	go wait.Until(m.syncNodeStatus, nodeStatusUpdateFrequency, wait.NeverStop)
}

// syncNodeStatus should be called periodically from a goroutine.
// It synchronizes node status to master, registering the kubelet first if
// necessary.
func (m *managerImpl) syncNodeStatus() {
	if m.kubeClient == nil {
		return
	}
	if m.registerNode {
		// This will exit immediately if it doesn't need to do anything.
		m.registerWithApiserver()
	}
	if err := m.updateNodeStatus(); err != nil {
		glog.Errorf("Unable to update node status: %v", err)
	}
}

// updateNodeStatus updates node status to master with retries.
func (m *managerImpl) updateNodeStatus() error {
	for i := 0; i < nodeStatusUpdateRetry; i++ {
		if err := m.tryUpdateNodeStatus(); err != nil {
			glog.Errorf("Error updating node status, will retry: %v", err)
		} else {
			return nil
		}
	}
	return fmt.Errorf("update node status exceeds retry count")
}

// registerWithApiserver registers the node with the cluster master. It is safe
// to call multiple times, but not concurrently (m.registrationCompleted is
// not locked).
func (m *managerImpl) registerWithApiserver() {
	if m.registrationCompleted {
		return
	}
	step := 100 * time.Millisecond
	for {
		time.Sleep(step)
		step = step * 2
		if step >= 7*time.Second {
			step = 7 * time.Second
		}

		node, err := m.initialNodeStatus()
		if err != nil {
			glog.Errorf("Unable to construct api.Node object for kubelet: %v", err)
			continue
		}
		glog.V(2).Infof("Attempting to register node %s", node.Name)
		if _, err := m.kubeClient.Core().Nodes().Create(node); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				glog.V(2).Infof("Unable to register %s with the apiserver: %v", node.Name, err)
				continue
			}
			currentNode, err := m.kubeClient.Core().Nodes().Get(m.nodeName)
			if err != nil {
				glog.Errorf("error getting node %q: %v", m.nodeName, err)
				continue
			}
			if currentNode == nil {
				glog.Errorf("no node instance returned for %q", m.nodeName)
				continue
			}
			if currentNode.Spec.ExternalID == node.Spec.ExternalID {
				glog.Infof("Node %s was previously registered", node.Name)
				m.registrationCompleted = true
				return
			}
			glog.Errorf(
				"Previously %q had externalID %q; now it is %q; will delete and recreate.",
				m.nodeName, node.Spec.ExternalID, currentNode.Spec.ExternalID,
			)
			if err := m.kubeClient.Core().Nodes().Delete(node.Name, nil); err != nil {
				glog.Errorf("Unable to delete old node: %v", err)
			} else {
				glog.Errorf("Deleted old node object %q", m.nodeName)
			}
			continue
		}
		glog.Infof("Successfully registered node %s", node.Name)
		m.registrationCompleted = true
		return
	}
}

func (m *managerImpl) initialNodeStatus() (*api.Node, error) {
	node := &api.Node{
		ObjectMeta: api.ObjectMeta{
			Name:   m.nodeName,
			Labels: map[string]string{unversioned.LabelHostname: m.hostname},
		},
		Spec: api.NodeSpec{
			Unschedulable: !m.registerSchedulable,
		},
	}

	// @question: should this be place after the call to the cloud provider? which also applies labels
	for k, v := range m.nodeLabels {
		if cv, found := node.ObjectMeta.Labels[k]; found {
			glog.Warningf("the node label %s=%s will overwrite default setting %s", k, v, cv)
		}
		node.ObjectMeta.Labels[k] = v
	}

	if m.cloud != nil {
		instances, ok := m.cloud.Instances()
		if !ok {
			return nil, fmt.Errorf("failed to get instances from cloud provider")
		}

		// TODO(roberthbailey): Can we do this without having credentials to talk
		// to the cloud provider?
		// TODO: ExternalID is deprecated, we'll have to drop this code
		externalID, err := instances.ExternalID(m.nodeName)
		if err != nil {
			return nil, fmt.Errorf("failed to get external ID from cloud provider: %v", err)
		}
		node.Spec.ExternalID = externalID

		// TODO: We can't assume that the node has credentials to talk to the
		// cloudprovider from arbitrary nodes. At most, we should talk to a
		// local metadata server here.
		node.Spec.ProviderID, err = cloudprovider.GetInstanceProviderID(m.cloud, m.nodeName)
		if err != nil {
			return nil, err
		}

		instanceType, err := instances.InstanceType(m.nodeName)
		if err != nil {
			return nil, err
		}
		if instanceType != "" {
			glog.Infof("Adding node label from cloud provider: %s=%s", unversioned.LabelInstanceType, instanceType)
			node.ObjectMeta.Labels[unversioned.LabelInstanceType] = instanceType
		}
		// If the cloud has zone information, label the node with the zone information
		zones, ok := m.cloud.Zones()
		if ok {
			zone, err := zones.GetZone()
			if err != nil {
				return nil, fmt.Errorf("failed to get zone from cloud provider: %v", err)
			}
			if zone.FailureDomain != "" {
				glog.Infof("Adding node label from cloud provider: %s=%s", unversioned.LabelZoneFailureDomain, zone.FailureDomain)
				node.ObjectMeta.Labels[unversioned.LabelZoneFailureDomain] = zone.FailureDomain
			}
			if zone.Region != "" {
				glog.Infof("Adding node label from cloud provider: %s=%s", unversioned.LabelZoneRegion, zone.Region)
				node.ObjectMeta.Labels[unversioned.LabelZoneRegion] = zone.Region
			}
		}
	} else {
		node.Spec.ExternalID = m.hostname
	}
	if err := m.setNodeStatus(node); err != nil {
		return nil, err
	}
	return node, nil
}

func (m *managerImpl) setNodeStatusMachineInfo(node *api.Node) {
	// TODO: Post NotReady if we cannot get MachineInfo from cAdvisor. This needs to start
	// cAdvisor locally, e.g. for test-cmd.sh, and in integration test.
	info, err := m.machineInfo
	if err != nil {
		// TODO(roberthbailey): This is required for test-cmd.sh to pass.
		// See if the test should be updated instead.
		node.Status.Capacity = api.ResourceList{
			api.ResourceCPU:    *resource.NewMilliQuantity(0, resource.DecimalSI),
			api.ResourceMemory: resource.MustParse("0Gi"),
			api.ResourcePods:   *resource.NewQuantity(int64(m.maxPods), resource.DecimalSI),
		}
		glog.Errorf("Error getting machine info: %v", err)
	} else {
		node.Status.NodeInfo.MachineID = info.MachineID
		node.Status.NodeInfo.SystemUUID = info.SystemUUID
		node.Status.Capacity = cadvisor.CapacityFromMachineInfo(info)
		node.Status.Capacity[api.ResourcePods] = *resource.NewQuantity(
			int64(m.maxPods), resource.DecimalSI)
		if node.Status.NodeInfo.BootID != "" &&
			node.Status.NodeInfo.BootID != info.BootID {
			// TODO: This requires a transaction, either both node status is updated
			// and event is recorded or neither should happen, see issue #6055.
			m.recorder.Eventf(m.nodeRef, api.EventTypeWarning, kubecontainer.NodeRebooted,
				"Node %s has been rebooted, boot id: %s", m.nodeName, info.BootID)
		}
		node.Status.NodeInfo.BootID = info.BootID
	}

	// Set Allocatable.
	node.Status.Allocatable = make(api.ResourceList)
	for k, v := range node.Status.Capacity {
		value := *(v.Copy())
		if m.reservation.System != nil {
			value.Sub(m.reservation.System[k])
		}
		if m.reservation.Kubernetes != nil {
			value.Sub(m.reservation.Kubernetes[k])
		}
		if value.Amount != nil && value.Amount.Sign() < 0 {
			// Negative Allocatable resources don't make sense.
			value.Set(0)
		}
		node.Status.Allocatable[k] = value
	}
}

// Set versioninfo for the node.
func (m *managerImpl) setNodeStatusVersionInfo(node *api.Node) {
	verinfo, err := m.cadvisor.VersionInfo()
	if err != nil {
		glog.Errorf("Error getting version info: %v", err)
	} else {
		node.Status.NodeInfo.KernelVersion = verinfo.KernelVersion
		node.Status.NodeInfo.OSImage = verinfo.ContainerOsVersion

		runtimeVersion := "Unknown"
		if runtimeVer, err := m.containerRuntime.Version(); err == nil {
			runtimeVersion = runtimeVer.String()
		}
		node.Status.NodeInfo.ContainerRuntimeVersion = fmt.Sprintf("%s://%s", m.containerRuntime.Type(), runtimeVersion)

		node.Status.NodeInfo.KubeletVersion = version.Get().String()
		// TODO: kube-proxy might be different version from kubelet in the future
		node.Status.NodeInfo.KubeProxyVersion = version.Get().String()
	}

}

// Set daemonEndpoints for the node.
func (m *managerImpl) setNodeStatusDaemonEndpoints(node *api.Node) {
	node.Status.DaemonEndpoints = *m.daemonEndpoints
}

// Set images list fot this node
func (m *managerImpl) setNodeStatusImages(node *api.Node) {
	// Update image list of this node
	var imagesOnNode []api.ContainerImage
	containerImages, err := m.imageManager.GetImageList()
	if err != nil {
		glog.Errorf("Error getting image list: %v", err)
	} else {
		for _, image := range containerImages {
			imagesOnNode = append(imagesOnNode, api.ContainerImage{
				Names:     image.RepoTags,
				SizeBytes: image.Size,
			})
		}
	}
	node.Status.Images = imagesOnNode
}

// Set status for the node.
func (m *managerImpl) setNodeStatusInfo(node *api.Node) {
	m.setNodeStatusMachineInfo(node)
	m.setNodeStatusVersionInfo(node)
	m.setNodeStatusDaemonEndpoints(node)
	m.setNodeStatusImages(node)
}

// Set Readycondition for the node.
func (m *managerImpl) setNodeReadyCondition(node *api.Node) {
	// NOTE(aaronlevy): NodeReady condition needs to be the last in the list of node conditions.
	// This is due to an issue with version skewed kubelet and master components.
	// ref: https://github.com/kubernetes/kubernetes/issues/16961
	currentTime := unversioned.NewTime(m.clock.Now())
	var newNodeReadyCondition api.NodeCondition
	if rs := m.runtimeState.errors(); len(rs) == 0 {
		newNodeReadyCondition = api.NodeCondition{
			Type:              api.NodeReady,
			Status:            api.ConditionTrue,
			Reason:            "KubeletReady",
			Message:           "kubelet is posting ready status",
			LastHeartbeatTime: currentTime,
		}
	} else {
		newNodeReadyCondition = api.NodeCondition{
			Type:              api.NodeReady,
			Status:            api.ConditionFalse,
			Reason:            "KubeletNotReady",
			Message:           strings.Join(rs, ","),
			LastHeartbeatTime: currentTime,
		}
	}

	// Record any soft requirements that were not met in the container manager.
	status := m.containerManager.Status()
	if status.SoftRequirements != nil {
		newNodeReadyCondition.Message = fmt.Sprintf("%s. WARNING: %s", newNodeReadyCondition.Message, status.SoftRequirements.Error())
	}

	readyConditionUpdated := false
	needToRecordEvent := false
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == api.NodeReady {
			if node.Status.Conditions[i].Status == newNodeReadyCondition.Status {
				newNodeReadyCondition.LastTransitionTime = node.Status.Conditions[i].LastTransitionTime
			} else {
				newNodeReadyCondition.LastTransitionTime = currentTime
				needToRecordEvent = true
			}
			node.Status.Conditions[i] = newNodeReadyCondition
			readyConditionUpdated = true
			break
		}
	}
	if !readyConditionUpdated {
		newNodeReadyCondition.LastTransitionTime = currentTime
		node.Status.Conditions = append(node.Status.Conditions, newNodeReadyCondition)
	}
	if needToRecordEvent {
		if newNodeReadyCondition.Status == api.ConditionTrue {
			m.recordNodeStatusEvent(api.EventTypeNormal, kubecontainer.NodeReady)
		} else {
			m.recordNodeStatusEvent(api.EventTypeNormal, kubecontainer.NodeNotReady)
		}
	}
}

// Set OODcondition for the node.
func (m *managerImpl) setNodeOODCondition(node *api.Node) {
	currentTime := unversioned.NewTime(m.clock.Now())
	var nodeOODCondition *api.NodeCondition

	// Check if NodeOutOfDisk condition already exists and if it does, just pick it up for update.
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == api.NodeOutOfDisk {
			nodeOODCondition = &node.Status.Conditions[i]
		}
	}

	newOODCondition := false
	// If the NodeOutOfDisk condition doesn't exist, create one.
	if nodeOODCondition == nil {
		nodeOODCondition = &api.NodeCondition{
			Type:   api.NodeOutOfDisk,
			Status: api.ConditionUnknown,
		}
		// nodeOODCondition cannot be appended to node.Status.Conditions here because it gets
		// copied to the slice. So if we append nodeOODCondition to the slice here none of the
		// updates we make to nodeOODCondition below are reflected in the slice.
		newOODCondition = true
	}

	// Update the heartbeat time irrespective of all the conditions.
	nodeOODCondition.LastHeartbeatTime = currentTime

	// Note: The conditions below take care of the case when a new NodeOutOfDisk condition is
	// created and as well as the case when the condition already exists. When a new condition
	// is created its status is set to api.ConditionUnknown which matches either
	// nodeOODCondition.Status != api.ConditionTrue or
	// nodeOODCondition.Status != api.ConditionFalse in the conditions below depending on whether
	// the kubelet is out of disk or not.
	if m.isOutOfDisk() {
		if nodeOODCondition.Status != api.ConditionTrue {
			nodeOODCondition.Status = api.ConditionTrue
			nodeOODCondition.Reason = "KubeletOutOfDisk"
			nodeOODCondition.Message = "out of disk space"
			nodeOODCondition.LastTransitionTime = currentTime
			m.recordNodeStatusEvent(api.EventTypeNormal, "NodeOutOfDisk")
		}
	} else {
		if nodeOODCondition.Status != api.ConditionFalse {
			// Update the out of disk condition when the condition status is unknown even if we
			// are within the outOfDiskTransitionFrequency duration. We do this to set the
			// condition status correctly at kubelet startup.
			if nodeOODCondition.Status == api.ConditionUnknown || m.clock.Since(nodeOODCondition.LastTransitionTime.Time) >= m.outOfDiskTransitionFrequency {
				nodeOODCondition.Status = api.ConditionFalse
				nodeOODCondition.Reason = "KubeletHasSufficientDisk"
				nodeOODCondition.Message = "kubelet has sufficient disk space available"
				nodeOODCondition.LastTransitionTime = currentTime
				m.recordNodeStatusEvent(api.EventTypeNormal, "NodeHasSufficientDisk")
			} else {
				glog.Infof("Node condition status for OutOfDisk is false, but last transition time is less than %s", m.outOfDiskTransitionFrequency)
			}
		}
	}

	if newOODCondition {
		node.Status.Conditions = append(node.Status.Conditions, *nodeOODCondition)
	}
}

// Maintains Node.Spec.Unschedulable value from previous run of tryUpdateNodeStatus()
var oldNodeUnschedulable bool

// record if node schedulable change.
func (m *managerImpl) recordNodeSchdulableEvent(node *api.Node) {
	if oldNodeUnschedulable != node.Spec.Unschedulable {
		if node.Spec.Unschedulable {
			m.recordNodeStatusEvent(api.EventTypeNormal, kubecontainer.NodeNotSchedulable)
		} else {
			m.recordNodeStatusEvent(api.EventTypeNormal, kubecontainer.NodeSchedulable)
		}
		oldNodeUnschedulable = node.Spec.Unschedulable
	}
}

// setNodeStatus fills in the Status fields of the given Node, overwriting
// any fields that are currently set.
// TODO(madhusudancs): Simplify the logic for setting node conditions and
// refactor the node status condtion code out to a different file.
func (m *managerImpl) setNodeStatus(node *api.Node) error {
	for _, f := range m.setNodeStatusFuncs {
		if err := f(node); err != nil {
			return err
		}
	}
	return nil
}

// defaultNodeStatusFuncs is a factory that generates the default set of setNodeStatus funcs
func (m *managerImpl) defaultNodeStatusFuncs() []func(*api.Node) error {
	// initial set of node status update handlers, can be modified by Option's
	withoutError := func(f func(*api.Node)) func(*api.Node) error {
		return func(n *api.Node) error {
			f(n)
			return nil
		}
	}
	return []func(*api.Node) error{
		m.setNodeAddress,
		withoutError(m.setNodeStatusInfo),
		withoutError(m.setNodeOODCondition),
		withoutError(m.setNodeReadyCondition),
		withoutError(m.recordNodeSchdulableEvent),
	}
}

// SetNodeStatus returns a functional Option that adds the given node status update handler to the Kubelet
func SetNodeStatus(f func(*api.Node) error) Option {
	return func(k *Kubelet) {
		k.setNodeStatusFuncs = append(k.setNodeStatusFuncs, f)
	}
}

// tryUpdateNodeStatus tries to update node status to master. If ReconcileCBR0
// is set, this function will also confirm that cbr0 is configured correctly.
func (m *managerImpl) tryUpdateNodeStatus() error {
	node, err := m.kubeClient.Core().Nodes().Get(m.nodeName)
	if err != nil {
		return fmt.Errorf("error getting node %q: %v", m.nodeName, err)
	}
	if node == nil {
		return fmt.Errorf("no node instance returned for %q", m.nodeName)
	}
	// Flannel is the authoritative source of pod CIDR, if it's running.
	// This is a short term compromise till we get flannel working in
	// reservation mode.
	if m.flannelExperimentalOverlay {
		flannelPodCIDR := m.runtimeState.podCIDR()
		if node.Spec.PodCIDR != flannelPodCIDR {
			node.Spec.PodCIDR = flannelPodCIDR
			glog.Infof("Updating podcidr to %v", node.Spec.PodCIDR)
			if updatedNode, err := m.kubeClient.Core().Nodes().Update(node); err != nil {
				glog.Warningf("Failed to update podCIDR: %v", err)
			} else {
				// Update the node resourceVersion so the status update doesn't fail.
				node = updatedNode
			}
		}
	} else if m.reconcileCIDR {
		m.updatePodCIDR(node.Spec.PodCIDR)
	}

	if err := m.setNodeStatus(node); err != nil {
		return err
	}
	// Update the current status on the API server
	_, err = m.kubeClient.Core().Nodes().UpdateStatus(node)
	return err
}
