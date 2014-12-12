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
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
)

// ResourceControllersNamespacer has methods to work with ResourceController resources in a namespace
type ResourceControllersNamespacer interface {
	ResourceControllers(namespace string) ResourceControllerInterface
}

// ResourceControllerInterface has methods to work with ResourceController resources.
type ResourceControllerInterface interface {
	List(selector labels.Selector) (*api.ResourceControllerList, error)
	Get(name string) (*api.ResourceController, error)
	Create(ctrl *api.ResourceController) (*api.ResourceController, error)
	Update(ctrl *api.ResourceController) (*api.ResourceController, error)
	Delete(name string) error
	Watch(label, field labels.Selector, resourceVersion string) (watch.Interface, error)
}

// resourceControllers implements ResourceControllersNamespacer interface
type resourceControllers struct {
	r  *Client
	ns string
}

// newResourceControllers returns a PodsClient
func newResourceControllers(c *Client, namespace string) *resourceControllers {
	return &resourceControllers{c, namespace}
}

// List takes a selector, and returns the list of resource controllers that match that selector.
func (c *resourceControllers) List(selector labels.Selector) (result *api.ResourceControllerList, err error) {
	result = &api.ResourceControllerList{}
	err = c.r.Get().Namespace(c.ns).Resource("resourceControllers").SelectorParam("labels", selector).Do().Into(result)
	return
}

// Get returns information about a particular resource controller.
func (c *resourceControllers) Get(name string) (result *api.ResourceController, err error) {
	result = &api.ResourceController{}
	err = c.r.Get().Namespace(c.ns).Resource("resourceControllers").Name(name).Do().Into(result)
	return
}

// Create creates a new resource controller.
func (c *resourceControllers) Create(controller *api.ResourceController) (result *api.ResourceController, err error) {
	result = &api.ResourceController{}
	err = c.r.Post().Namespace(c.ns).Resource("resourceControllers").Body(controller).Do().Into(result)
	return
}

// Update updates an existing resource controller.
func (c *resourceControllers) Update(controller *api.ResourceController) (result *api.ResourceController, err error) {
	result = &api.ResourceController{}
	if len(controller.ResourceVersion) == 0 {
		err = fmt.Errorf("invalid update object, missing resource version: %v", controller)
		return
	}
	err = c.r.Put().Namespace(c.ns).Resource("resourceControllers").Name(controller.Name).Body(controller).Do().Into(result)
	return
}

// Delete deletes an existing resource controller.
func (c *resourceControllers) Delete(name string) error {
	return c.r.Delete().Namespace(c.ns).Resource("resourceControllers").Name(name).Do().Error()
}

// Watch returns a watch.Interface that watches the requested controllers.
func (c *resourceControllers) Watch(label, field labels.Selector, resourceVersion string) (watch.Interface, error) {
	return c.r.Get().
		Prefix("watch").
		Namespace(c.ns).
		Resource("resourceControllers").
		Param("resourceVersion", resourceVersion).
		SelectorParam("labels", label).
		SelectorParam("fields", field).
		Watch()
}
