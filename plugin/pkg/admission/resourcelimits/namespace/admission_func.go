/*
Copyright 2014 Google Inc. All rights reserved.

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

package namespace

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/admission"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	apierrors "github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/meta"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/resource"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/resourcecontroller"
	"github.com/GoogleCloudPlatform/kubernetes/plugin/pkg/admission/resourcelimits"
)

func init() {
	resourcelimits.RegisterAdmissionFunc("ResourceLimitsNamespace", admissionFunc)
}

var kindsToProcess = map[string]bool{
	"pods":                   true,
	"services":               true,
	"replicationControllers": true,
}
var resourceNameToMessage = map[api.ResourceName]string{
	"Pods":                   "Limited to %v pods in namespace %v",
	"Services":               "Limited to %v services in namespace %v",
	"ReplicationControllers": "Limited to %v replication controllers in namespace %v",
}
var kindToMaxKindResourceName = map[string]api.ResourceName{
	"pods":                   "Pods",
	"services":               "Services",
	"replicationControllers": "ReplicationControllers",
}

func makeObservation(status *api.ResourceControllerStatus, resourceName api.ResourceName, newQuantity *resource.Quantity) {
	_, observedAllocatedByGroup := resourcecontroller.AllowedAndAllocated(status)
	observedAllocatedGroupRules := observedAllocatedByGroup[api.ResourceControllerGroupByNamespace]
	observedAllocatedGroupRulesMax := observedAllocatedGroupRules[api.ResourceControllerRuleTypeMax]
	observedAllocatedGroupRulesMax[resourceName] = *newQuantity
}

func admissionFunc(a admission.Attributes, input *api.ResourceController, observation *api.ResourceObservation, client client.Interface) (bool, error) {
	groupBy := api.ResourceControllerGroupByNamespace
	dirty := false

	if a.GetOperation() == "DELETE" {
		return dirty, nil
	}

	_, found := kindsToProcess[a.GetKind()]
	if !found {
		return dirty, nil
	}

	allowedByGroup, allocatedByGroup := resourcecontroller.AllowedAndAllocated(&input.Status)
	allowedGroupRules := allowedByGroup[groupBy]
	if allowedGroupRules == nil {
		return dirty, nil
	}
	allocatedGroupRules := allocatedByGroup[groupBy]
	if allowedGroupRules == nil {
		return dirty, nil
	}

	name, err := meta.NewAccessor().Name(a.GetObject())
	if err != nil {
		name = "Unknown"
	}

	// TODO: handle Update
	if a.GetOperation() == "CREATE" {

		// Handles not being able to create more than X of a Kind
		allowedGroupRulesMax := allowedGroupRules[api.ResourceControllerRuleTypeMax]
		allocatedGroupRulesMax := allocatedGroupRules[api.ResourceControllerRuleTypeMax]
		if allowedGroupRulesMax != nil {
			resourceName := kindToMaxKindResourceName[a.GetKind()]
			msg := "Limited to %s %s in namespace %s"

			limit, exists := allowedGroupRulesMax[resourceName]
			if exists {

				observed, observationExists := allocatedGroupRulesMax[resourceName]
				if !observationExists {
					return dirty, apierrors.NewForbidden(a.GetKind(), name, fmt.Errorf("Unable to admit resource, waiting for resource observation to complete."))
				}

				if observed.Value() >= limit.Value() {
					return dirty, apierrors.NewForbidden(
						a.GetKind(),
						name,
						fmt.Errorf(msg, limit.String(), a.GetKind(), input.Namespace))
				} else {
					// increment the allocated observation
					makeObservation(&observation.Status, resourceName, resource.NewQuantity(observed.Value()+int64(1), resource.DecimalSI))
					dirty = true
				}
			}
		}

		if a.GetKind() == "pods" {
			cpuLimit, cpuExists := allowedGroupRulesMax["CPU"]
			memLimit, memExists := allowedGroupRulesMax["Memory"]

			obj := a.GetObject()
			pod := obj.(*api.Pod)
			// compute local usage to this pod
			if cpuExists || memExists {
				podCPU := int64(0)
				podMem := int64(0)
				for _, container := range pod.Spec.Containers {
					podCPU = podCPU + container.CPU.MilliValue()
					podMem = podMem + container.Memory.Value()
				}

				if cpuExists {
					cpuObservation, cpuObservationExists := allocatedGroupRulesMax["CPU"]
					if !cpuObservationExists {
						return dirty, apierrors.NewForbidden(a.GetKind(), name, fmt.Errorf("Unable to admit resource, waiting for resource observation to complete."))
					}

					if cpuObservation.MilliValue()+podCPU >= cpuLimit.MilliValue() {
						return dirty, apierrors.NewForbidden(
							a.GetKind(),
							name,
							fmt.Errorf("Limit to %v CPU in namespace %v", cpuLimit.String(), input.Namespace))
					} else {
						makeObservation(&observation.Status, "CPU", resource.NewMilliQuantity(cpuObservation.MilliValue()+podCPU, resource.DecimalSI))
						dirty = true
					}
				}

				if memExists {
					memObservation, memObservationExists := allocatedGroupRulesMax["Memory"]
					if !memObservationExists {
						return dirty, apierrors.NewForbidden(a.GetKind(), name, fmt.Errorf("Unable to admit resource, waiting for resource observation to complete."))
					}

					if memObservation.Value()+podMem >= memLimit.Value() {
						// convert runtime object and get its name
						return dirty, apierrors.NewForbidden(
							a.GetKind(),
							name,
							fmt.Errorf("Limit to %v memory in namespace %v", memLimit.String(), input.Namespace))
					} else {
						makeObservation(&observation.Status, "Memory", resource.NewQuantity(memObservation.Value()+podMem, resource.DecimalSI))
						dirty = true
					}
				}
			}
		}
	}

	return dirty, nil
}
