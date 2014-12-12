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
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/resource"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
)

// ObserverFunc makes an observation in the given namespace
// The provided store is initialized with each periodic synchronization of the supplied namespace
// It is useful for ensuring multiple client calls are not required to get the same data for each synchronization tick
type ObserverFunc func(store cache.Store, namespace string) (*resource.Quantity, error)

// ObserverFuncBinding associates an observer function with a group, rule, and resource
type ObserverFuncBinding struct {
	GroupBy      api.ResourceControllerGroupBy
	RuleType     api.ResourceControllerRuleType
	ResourceName api.ResourceName
	Func         ObserverFunc
}

// Observer is a plug-in that groups a set of ObserverFuncBindings
type Observer interface {
	ObserverFuncBindings() []ObserverFuncBinding
}

// Factory instantiates an Observer with a configured client
type Factory func(client.Interface) (Observer, error)
