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
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/golang/glog"
)

// ResourceManager is responsible for observing changes to the system and recording usage observations with latest state
type ResourceManager struct {
	kubeClient client.Interface
	syncTime   <-chan time.Time

	// To allow injection of syncUsage for testing.
	syncHandler func(controller api.ResourceController) error

	// used to make observations in given group
	observerFuncs map[string]ObserverFunc
}

// NewResourceManager creates a new ResourceManager with specified client and list of observers
func NewResourceManager(kubeClient client.Interface, observers []Observer) *ResourceManager {

	// build the map of observations funcs that can make an observation
	observerFuncs := make(map[string]ObserverFunc)
	for _, observer := range observers {
		bindings := observer.ObserverFuncBindings()
		for _, binding := range bindings {
			observerFuncs[observerFuncKey(binding.GroupBy, binding.RuleType, binding.ResourceName)] = binding.Func
		}
	}

	rm := &ResourceManager{
		kubeClient:    kubeClient,
		observerFuncs: observerFuncs,
	}

	// set the synchronization handler
	rm.syncHandler = rm.syncResourceController
	return rm
}

// Run begins watching and syncing.
func (rm *ResourceManager) Run(period time.Duration) {
	rm.syncTime = time.Tick(period)
	go util.Forever(func() { rm.synchronize() }, period)
}

func (rm *ResourceManager) synchronize() {
	var resourceControllers []api.ResourceController
	list, err := rm.kubeClient.ResourceControllers(api.NamespaceAll).List(labels.Everything())
	if err != nil {
		glog.Errorf("Synchronization error: %v (%#v)", err, err)
	}
	resourceControllers = list.Items
	wg := sync.WaitGroup{}
	wg.Add(len(resourceControllers))
	for ix := range resourceControllers {
		go func(ix int) {
			defer wg.Done()
			glog.V(4).Infof("periodic sync of %v.%v", resourceControllers[ix].Namespace, resourceControllers[ix].Name)
			err := rm.syncHandler(resourceControllers[ix])
			if err != nil {
				glog.Errorf("Error synchronizing: %v", err)
			}
		}(ix)
	}
	wg.Wait()
}

// observerFuncKey generates an unique key to map an ObserverFunc
func observerFuncKey(groupBy api.ResourceControllerGroupBy, ruleType api.ResourceControllerRuleType, name api.ResourceName) string {
	s := []string{string(groupBy), string(ruleType), string(name)}
	return strings.Join(s, ".")
}

// syncResourceController runs a complete sync of current status
func (rm *ResourceManager) syncResourceController(controller api.ResourceController) error {
	// Create a resource observation that is used relative to the viewed controller resource version
	resourceObservation := api.ResourceObservation{
		ObjectMeta: api.ObjectMeta{
			Name:            controller.Name,
			Namespace:       controller.Namespace,
			ResourceVersion: controller.ResourceVersion},
		Status: api.ResourceControllerStatus{},
	}
	resourceObservation.Status.Allowed = make([]api.ResourceControllerGroup, len(controller.Spec.Allowed), len(controller.Spec.Allowed))
	resourceObservation.Status.Allocated = make([]api.ResourceControllerGroup, len(controller.Spec.Allowed), len(controller.Spec.Allowed))
	copy(resourceObservation.Status.Allowed, controller.Spec.Allowed)

	// prevAllocatedStatus is what we previously recorded as usage, we will use it to compare with our latest observations
	_, prevAllocatedStatus := AllowedAndAllocated(&controller.Status)

	// dirty tracks if the observed status differs from the previous observation, if so, we send a new observation with latest status
	// if this is our first observation, it will be dirty by default, since we need to make an observation
	dirty := controller.Status.Allowed == nil || controller.Status.Allocated == nil

	for index, group := range resourceObservation.Status.Allowed {

		// create a store that can hold cached data so observer functions do not need to fetch the same data multiple times per synch loop
		// for example, multiple observations may require a listing of all pods in a namespace, and we do not want to fetch them multiple
		// times
		store := cache.NewStore()

		// latest observation is what is computed now
		latestObservation := api.ResourceControllerGroup{GroupBy: group.GroupBy, RuleType: group.RuleType, Resources: api.ResourceList{}}

		// for each named resource [cpu, memory, etc], make an observation used registered observer function
		for name, _ := range group.Resources {

			// if this resource requires an update observation, make one now
			observerFunc := rm.observerFuncs[observerFuncKey(group.GroupBy, group.RuleType, name)]
			if observerFunc != nil {

				quantity, err := observerFunc(store, controller.Namespace)
				if err != nil {
					return err
				}
				latestObservation.Resources[name] = *quantity

				// compare what we previously thought was used with what was just observed
				prevQuantity := prevAllocatedStatus[group.GroupBy][group.RuleType][name]
				dirty = dirty || (quantity.Value() != prevQuantity.Value())

			}
		}
		// add it to the status
		resourceObservation.Status.Allocated[index] = latestObservation
	}

	if dirty {
		return rm.kubeClient.ResourceObservations(resourceObservation.Namespace).Create(&resourceObservation)
	}
	return nil
}

type ResourceControllerRuleTypeToResourceList map[api.ResourceControllerRuleType]api.ResourceList

// AllowedAndAllocated is a utility function to group controller status allowed and allocated resource control groups by group -> rule -> resource
func AllowedAndAllocated(status *api.ResourceControllerStatus) (map[api.ResourceControllerGroupBy]ResourceControllerRuleTypeToResourceList, map[api.ResourceControllerGroupBy]ResourceControllerRuleTypeToResourceList) {
	allowedGroupBy := make(map[api.ResourceControllerGroupBy]ResourceControllerRuleTypeToResourceList)
	allocatedGroupBy := make(map[api.ResourceControllerGroupBy]ResourceControllerRuleTypeToResourceList)

	for _, allowed := range status.Allowed {
		_, found := allowedGroupBy[allowed.GroupBy]
		if !found {
			allowedGroupBy[allowed.GroupBy] = make(ResourceControllerRuleTypeToResourceList)
		}
		allowedGroupBy[allowed.GroupBy][allowed.RuleType] = allowed.Resources
	}
	for _, allocated := range status.Allocated {
		_, found := allocatedGroupBy[allocated.GroupBy]
		if !found {
			allocatedGroupBy[allocated.GroupBy] = make(ResourceControllerRuleTypeToResourceList)
		}
		allocatedGroupBy[allocated.GroupBy][allocated.RuleType] = allocated.Resources
	}
	return allowedGroupBy, allocatedGroupBy
}
