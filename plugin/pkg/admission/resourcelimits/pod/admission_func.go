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

package pod

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/admission"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	apierrors "github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/resource"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/resourcecontroller"
	"github.com/GoogleCloudPlatform/kubernetes/plugin/pkg/admission/resourcelimits"
)

func init() {
	resourcelimits.RegisterAdmissionFunc("ResourceLimitsPod", admissionFunc)
}

func admissionFunc(a admission.Attributes, input *api.ResourceController, observation *api.ResourceObservation, client client.Interface) (bool, error) {
	dirty := false

	if a.GetOperation() == "DELETE" {
		return dirty, nil
	}

	if a.GetKind() != "pods" {
		return dirty, nil
	}

	allowedByGroup, _ := resourcecontroller.AllowedAndAllocated(&input.Status)
	groupRules := allowedByGroup[api.ResourceControllerGroupByPod]
	if groupRules == nil {
		return dirty, nil
	}

	obj := a.GetObject()
	pod := obj.(*api.Pod)

	memoryUsage := int64(0)
	cpuUsage := int64(0)
	for _, container := range pod.Spec.Containers {
		memoryUsage = memoryUsage + container.Memory.Value()
		cpuUsage = cpuUsage + container.CPU.MilliValue()
	}
	memoryQuantity := resource.NewQuantity(memoryUsage, resource.BinarySI)
	cpuQuantity := resource.NewMilliQuantity(cpuUsage, resource.DecimalSI)

	for ruleType, resources := range groupRules {
		for name, quantity := range resources {
			switch ruleType {
			case api.ResourceControllerRuleTypeMax:
				switch name {
				case "Memory":
					if memoryUsage > quantity.Value() {
						return dirty, apierrors.NewForbidden(
							a.GetKind(),
							pod.Name,
							fmt.Errorf("Memory usage %s is greater than the max: %v",
								memoryQuantity.String(),
								quantity.String()))
					}
				case "CPU":
					if cpuQuantity.MilliValue() > quantity.MilliValue() {
						return dirty, apierrors.NewForbidden(
							a.GetKind(),
							pod.Name,
							fmt.Errorf("CPU usage %s is greater than the max: %v",
								cpuQuantity.String(),
								quantity.String()))
					}
				}
			case api.ResourceControllerRuleTypeMin:
				switch name {
				case "Memory":
					if memoryUsage < quantity.Value() {
						return dirty, apierrors.NewForbidden(
							a.GetKind(),
							pod.Name,
							fmt.Errorf("Memory usage %s is less than the min: %v",
								memoryQuantity.String(),
								quantity.String()))
					}
				case "CPU":
					if cpuQuantity.MilliValue() < quantity.MilliValue() {
						return dirty, apierrors.NewForbidden(
							a.GetKind(),
							pod.Name,
							fmt.Errorf("CPU usage %v is less than the min: %v",
								cpuQuantity.MilliValue(),
								quantity.MilliValue()))
					}
				}
			}
		}
	}
	return dirty, nil
}
