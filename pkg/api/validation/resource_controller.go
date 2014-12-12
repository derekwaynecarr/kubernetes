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

package validation

import (
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	errs "github.com/GoogleCloudPlatform/kubernetes/pkg/api/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
)

// ValidateResourceController makes sure that the resource controller makes sense.
func ValidateResourceController(controller *api.ResourceController) errs.ValidationErrorList {
	allErrs := errs.ValidationErrorList{}
	if len(controller.Name) == 0 {
		allErrs = append(allErrs, errs.NewFieldRequired("name", controller.Name))
	}
	if !util.IsDNSSubdomain(controller.Namespace) {
		allErrs = append(allErrs, errs.NewFieldInvalid("namespace", controller.Namespace, ""))
	}
	allErrs = append(allErrs, validateLabels(controller.Labels, "labels")...)
	return allErrs
}
