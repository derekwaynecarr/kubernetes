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

package resourcecontroller

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/registry/generic"
	etcdgeneric "github.com/GoogleCloudPlatform/kubernetes/pkg/registry/generic/etcd"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/registry/resourceobservation"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/tools"
)

type Registry interface {
	generic.Registry
	resourceobservation.Registry
}

// registry implements custom changes to generic.Etcd.
type registry struct {
	*etcdgeneric.Etcd
}

// Create stores the object with a ttl, so that events don't stay in the system forever.
func (r *registry) ApplyObservation(ctx api.Context, observation *api.ResourceObservation) error {
	// Get the current resource controller
	obj, err := r.Get(ctx, observation.Name)
	if err != nil {
		return err
	}

	if len(observation.ResourceVersion) == 0 {
		return fmt.Errorf("A resource observation must have a resourceVersion specified to ensure atomic updates")
	}

	// set the status
	ctrl := obj.(*api.ResourceController)
	ctrl.ResourceVersion = observation.ResourceVersion
	ctrl.Status = observation.Status
	return r.Update(ctx, ctrl.Name, ctrl)
}

// NewEtcdRegistry returns a registry which will store ResourceControllers in the given
// EtcdHelper.
func NewEtcdRegistry(h tools.EtcdHelper) Registry {
	return &registry{
		Etcd: &etcdgeneric.Etcd{
			NewFunc:      func() runtime.Object { return &api.ResourceController{} },
			NewListFunc:  func() runtime.Object { return &api.ResourceControllerList{} },
			EndpointName: "resourceController",
			KeyRootFunc:  func(ctx api.Context) string { return etcdgeneric.NamespaceKeyRootFunc(ctx, "resourceController") },
			KeyFunc: func(ctx api.Context, id string) (string, error) {
				return etcdgeneric.NamespaceKeyFunc(ctx, "resourceController", id)
			},
			Helper: h,
		},
	}
}
