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
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
)

func TestValidateResourceController(t *testing.T) {
	table := []struct {
		*api.ResourceController
		valid bool
	}{
		{
			&api.ResourceController{
				ObjectMeta: api.ObjectMeta{
					Name:      "test1",
					Namespace: "foo",
				},
			},
			true,
		}, {
			&api.ResourceController{
				ObjectMeta: api.ObjectMeta{
					Name:      "",
					Namespace: "foobar",
				},
			},
			false,
		}, {
			&api.ResourceController{
				ObjectMeta: api.ObjectMeta{
					Name:      "test3",
					Namespace: "",
				},
			},
			false,
		},
	}

	for _, item := range table {
		if e, a := item.valid, len(ValidateResourceController(item.ResourceController)) == 0; e != a {
			t.Errorf("%v: expected %v, got %v", item.ResourceController.Name, e, a)
		}
	}
}
