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

package container

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/admission"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	apierrors "github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/resourcecontroller"
	"github.com/GoogleCloudPlatform/kubernetes/plugin/pkg/admission/resourcelimits"
)

func init() {
	resourcelimits.RegisterAdmissionFunc("ResourceLimitsContainer", admissionFunc)
}

func admissionFunc(a admission.Attributes, input *api.ResourceController, observation *api.ResourceObservation, client client.Interface) (bool, error) {
	if a.GetOperation() == "DELETE" {
		return false, nil
	}

	if a.GetKind() != "pods" {
		return false, nil
	}

	allowedByGroup, _ := resourcecontroller.AllowedAndAllocated(&input.Status)
	groupRules := allowedByGroup[api.ResourceControllerGroupByContainer]
	if groupRules == nil {
		return false, nil
	}

	obj := a.GetObject()
	pod := obj.(*api.Pod)
	for ruleType, resources := range groupRules {
		for name, quantity := range resources {
			for _, container := range pod.Spec.Containers {
				switch ruleType {
				case api.ResourceControllerRuleTypeMax:
					switch name {
					case "Memory":
						if container.Memory.Value() > quantity.Value() {
							return false, apierrors.NewForbidden(
								a.GetKind(),
								pod.Name,
								fmt.Errorf("Unable to %v pod, container %v requests %v memory that is greater than the max: %v",
									a.GetOperation(),
									container.Name,
									container.Memory.String(),
									quantity.String()))
						}
					case "CPU":
						if container.CPU.MilliValue() > quantity.MilliValue() {
							return false, apierrors.NewForbidden(
								a.GetKind(),
								pod.Name,
								fmt.Errorf("Unable to %v pod, container %v requests %v cpu that is greater than the max: %v",
									a.GetOperation(),
									container.Name,
									container.CPU.String(),
									quantity.String()))
						}
					}
				case api.ResourceControllerRuleTypeMin:
					switch name {
					case "CPU":
						if container.CPU.MilliValue() < quantity.MilliValue() {
							return false, apierrors.NewForbidden(
								a.GetKind(),
								pod.Name,
								fmt.Errorf("Unable to %v pod, container %v requests %v cpu that is less than the min: %v",
									a.GetOperation(),
									container.Name,
									container.Memory.String(),
									quantity.String()))
						}
					case "Memory":
						if container.Memory.Value() < quantity.Value() {
							return false, apierrors.NewForbidden(
								a.GetKind(),
								pod.Name,
								fmt.Errorf("Unable to %v pod, container %v requests %v memory that is less than the min: %v",
									a.GetOperation(),
									container.Name,
									container.Memory.String(),
									quantity.Value()))
						}
					}
				}
			}
		}
	}
	return false, nil
}
