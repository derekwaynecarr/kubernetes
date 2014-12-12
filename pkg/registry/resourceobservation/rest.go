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

package resourceobservation

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/apiserver"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
)

// REST implements the RESTStorage interface for resource observations. When observations are written, it
// updates the status of a ResourceController.Status object.
type REST struct {
	registry Registry
}

// NewREST creates a new REST backed by the given registry.
func NewREST(registry Registry) *REST {
	return &REST{
		registry: registry,
	}
}

// New returns a new resource observation object
func (*REST) New() runtime.Object {
	return &api.ResourceObservation{}
}

// Create attempts to make the assignment indicated by the binding it recieves.
func (b *REST) Create(ctx api.Context, obj runtime.Object) (<-chan apiserver.RESTResult, error) {
	resourceObservation, ok := obj.(*api.ResourceObservation)
	if !ok {
		return nil, fmt.Errorf("incorrect type: %#v", obj)
	}
	return apiserver.MakeAsync(func() (runtime.Object, error) {
		if err := b.registry.ApplyObservation(ctx, resourceObservation); err != nil {
			return nil, err
		}
		return &api.Status{Status: api.StatusSuccess}, nil
	}), nil
}
