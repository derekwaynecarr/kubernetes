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

package replicationcontroller

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
	resourcelimits.RegisterAdmissionFunc("ResourceLimitsReplicationController", admissionFunc)
}

func admissionFunc(a admission.Attributes, input *api.ResourceController, observation *api.ResourceObservation, client client.Interface) (bool, error) {
	if a.GetOperation() == "DELETE" {
		return false, nil
	}

	if a.GetKind() != "replicationControllers" {
		return false, nil
	}

	allowedByGroup, _ := resourcecontroller.AllowedAndAllocated(&input.Status)
	groupRules := allowedByGroup[api.ResourceControllerGroupByReplicationController]
	if groupRules == nil {
		return false, nil
	}

	obj := a.GetObject()
	replicationController := obj.(*api.ReplicationController)
	replicas := int64(replicationController.Spec.Replicas)

	for ruleType, resources := range groupRules {
		for name, quantity := range resources {
			switch ruleType {
			case api.ResourceControllerRuleTypeMax:
				switch name {
				case "Replicas":
					if replicas > quantity.Value() {
						return false, apierrors.NewForbidden(a.GetKind(), replicationController.Name,
							fmt.Errorf("Replicas %v is greater than the max: %v",
								replicas,
								quantity.String()))
					}
				}
			}
		}
	}
	return false, nil
}
