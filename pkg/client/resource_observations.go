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

package client

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
)

// ResourceObservationsNamespacer has methods to work with ResourceObservation resources in a namespace
type ResourceObservationsNamespacer interface {
	ResourceObservations(namespace string) ResourceObservationInterface
}

// ResourceObservationInterface has methods to work with ResourceObservation resources.
type ResourceObservationInterface interface {
	Create(obs *api.ResourceObservation) error
}

// resourceObservations implements ResourceObservationsNamespacer interface
type resourceObservations struct {
	r  *Client
	ns string
}

func newResourceObservations(c *Client, namespace string) *resourceObservations {
	return &resourceObservations{c, namespace}
}

// Create creates a new resource observation.
func (c *resourceObservations) Create(obs *api.ResourceObservation) error {
	return c.r.Post().Namespace(c.ns).Resource("resourceObservations").Body(obs).Do().Error()
}
