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
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/validation"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/apiserver"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/registry/generic"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/watch"
)

// REST implements the RESTStorage interface for a ResourceControllers
type REST struct {
	registry generic.Registry
}

type RESTConfig struct {
	Registry generic.Registry
}

// NewREST returns a new REST.
func NewREST(registry generic.Registry) *REST {
	return &REST{
		registry: registry,
	}
}

func (rs *REST) Create(ctx api.Context, obj runtime.Object) (<-chan apiserver.RESTResult, error) {
	resourceController := obj.(*api.ResourceController)
	if !api.ValidNamespace(ctx, &resourceController.ObjectMeta) {
		return nil, errors.NewConflict("resourceController", resourceController.Namespace, fmt.Errorf("ResourceController.Namespace does not match the provided context"))
	}
	api.FillObjectMetaSystemFields(ctx, &resourceController.ObjectMeta)
	if len(resourceController.Name) == 0 {
		resourceController.Name = string(resourceController.UID)
	}
	if errs := validation.ValidateResourceController(resourceController); len(errs) > 0 {
		return nil, errors.NewInvalid("resourceController", resourceController.Name, errs)
	}
	return apiserver.MakeAsync(func() (runtime.Object, error) {
		err := rs.registry.Create(ctx, resourceController.Name, resourceController)
		if err != nil {
			return nil, err
		}
		return rs.registry.Get(ctx, resourceController.Name)
	}), nil
}

func (rs *REST) Delete(ctx api.Context, name string) (<-chan apiserver.RESTResult, error) {
	return apiserver.MakeAsync(func() (runtime.Object, error) {
		return &api.Status{Status: api.StatusSuccess}, rs.registry.Delete(ctx, name)
	}), nil
}

func (rs *REST) Get(ctx api.Context, name string) (runtime.Object, error) {
	obj, err := rs.registry.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	resourceController, ok := obj.(*api.ResourceController)
	if !ok {
		return nil, fmt.Errorf("invalid object type")
	}
	return resourceController, err
}

func (rs *REST) List(ctx api.Context, label, field labels.Selector) (runtime.Object, error) {
	return rs.registry.List(ctx, &generic.SelectionPredicate{label, field, rs.getAttrs})
}

func (rs *REST) Watch(ctx api.Context, label, field labels.Selector, resourceVersion string) (watch.Interface, error) {
	return rs.registry.Watch(ctx, &generic.SelectionPredicate{label, field, rs.getAttrs}, resourceVersion)
}

func (*REST) New() runtime.Object {
	return &api.ResourceController{}
}

func (*REST) NewList() runtime.Object {
	return &api.ResourceControllerList{}
}

// Update updates a ResourceController, but only the Spec fields are editable
func (rs *REST) Update(ctx api.Context, obj runtime.Object) (<-chan apiserver.RESTResult, error) {

	// validate the incoming object
	resourceController := obj.(*api.ResourceController)
	if !api.ValidNamespace(ctx, &resourceController.ObjectMeta) {
		return nil, errors.NewConflict("resourceController", resourceController.Namespace, fmt.Errorf("ResourceController.Namespace does not match the provided context"))
	}
	if errs := validation.ValidateResourceController(resourceController); len(errs) > 0 {
		return nil, errors.NewInvalid("resourceController", resourceController.Name, errs)
	}

	// look for the previous version of the object
	prevObj, err := rs.registry.Get(ctx, resourceController.Name)
	if err != nil {
		return nil, err
	}

	// copy the incoming value to the existing value, make sure we set the incoming resource version to the version we will persist
	prevController := prevObj.(*api.ResourceController)
	prevController.ResourceVersion = resourceController.ResourceVersion
	prevController.Spec = resourceController.Spec

	// persist
	return apiserver.MakeAsync(func() (runtime.Object, error) {
		if err := rs.registry.Update(ctx, prevController.Name, prevController); err != nil {
			return nil, err
		}
		return rs.registry.Get(ctx, resourceController.Name)
	}), nil
}

func (rs *REST) getAttrs(obj runtime.Object) (objLabels, objFields labels.Set, err error) {
	return labels.Set{}, labels.Set{}, nil
}
