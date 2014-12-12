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

package resourcelimits

import (
	"fmt"
	"io"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/admission"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	apierrors "github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/meta"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
)

func init() {
	admission.RegisterPlugin("ResourceLimits", func(client client.Interface, config io.Reader) (admission.Interface, error) {
		return &limits{client: client, admissionFuncs: GetAdmissionFuncs()}, nil
	})
}

type limits struct {
	client         client.Interface
	admissionFuncs []AdmissionFunc
}

func (l *limits) Admit(a admission.Attributes) (err error) {

	name := "Unknown"
	if a.GetObject() != nil {
		name, _ = meta.NewAccessor().Name(a.GetObject())
	}

	list, err := l.client.ResourceControllers(a.GetNamespace()).List(labels.Everything())
	if err != nil {
		return apierrors.NewForbidden(a.GetKind(), name, fmt.Errorf("Unable to %s %s at this time because there was an error enforcing admission control", a.GetOperation(), a.GetKind()))
	}

	for _, controller := range list.Items {

		// construct an observation
		resourceObservation := api.ResourceObservation{
			ObjectMeta: api.ObjectMeta{
				Name:            controller.Name,
				Namespace:       controller.Namespace,
				ResourceVersion: controller.ResourceVersion},
			Status: api.ResourceControllerStatus{},
		}
		resourceObservation.Status.Allowed = make([]api.ResourceControllerGroup, len(controller.Status.Allowed), len(controller.Status.Allowed))
		resourceObservation.Status.Allocated = make([]api.ResourceControllerGroup, len(controller.Status.Allowed), len(controller.Status.Allowed))
		copy(resourceObservation.Status.Allowed, controller.Status.Allowed)
		copy(resourceObservation.Status.Allocated, controller.Status.Allocated)

		// invoke each registered admissionFunc
		dirty := false
		for _, admissionFunc := range l.admissionFuncs {
			funcDirty, err := admissionFunc(a, &controller, &resourceObservation, l.client)
			if err != nil {
				return err
			}
			dirty = dirty || funcDirty
		}

		if dirty {
			err = l.client.ResourceObservations(resourceObservation.Namespace).Create(&resourceObservation)
			if err != nil {
				return apierrors.NewForbidden(a.GetKind(), name, fmt.Errorf("Unable to %s %s at this time because there was an error enforcing admission control", a.GetOperation(), a.GetKind()))
			}
		}
	}
	return nil
}
