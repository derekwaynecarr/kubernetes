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
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api/resource"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client/cache"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/resourcecontroller"
)

func init() {
	resourcecontroller.RegisterObserver("ResourceLimits", func(client client.Interface) (resourcecontroller.Observer, error) {
		return &observer{client: client}, nil
	})
}

type observer struct {
	client client.Interface
}

func (o *observer) pods(store cache.Store, namespace string) (*api.PodList, error) {
	obj, exists := store.Get("pods")
	if exists {
		items := obj.(*api.PodList)
		return items, nil
	}
	items, err := o.client.Pods(namespace).List(labels.Everything())
	store.Add("pods", items)
	return items, err
}

func (o *observer) services(store cache.Store, namespace string) (*api.ServiceList, error) {
	obj, exists := store.Get("services")
	if exists {
		items := obj.(*api.ServiceList)
		return items, nil
	}
	items, err := o.client.Services(namespace).List(labels.Everything())
	store.Add("services", items)
	return items, err
}

func (o *observer) replicationControllers(store cache.Store, namespace string) (*api.ReplicationControllerList, error) {
	obj, exists := store.Get("replicationControllers")
	if exists {
		items := obj.(*api.ReplicationControllerList)
		return items, nil
	}
	items, err := o.client.ReplicationControllers(namespace).List(labels.Everything())
	store.Add("replicationControllers", items)
	return items, err
}

// return observer func bindings for namespace scope
func (o *observer) namespaceObserverFuncBindings() []resourcecontroller.ObserverFuncBinding {
	observerFuncBindings := []resourcecontroller.ObserverFuncBinding{}

	groupBy := api.ResourceControllerGroupByNamespace

	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "CPU",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for _, item := range items.Items {
				for _, container := range item.Spec.Containers {
					val = val + container.CPU.MilliValue()
				}
			}
			return resource.NewMilliQuantity(int64(val), resource.DecimalSI), nil
		},
	})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "Memory",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for _, item := range items.Items {
				for _, container := range item.Spec.Containers {
					val = val + container.Memory.Value()
				}
			}
			return resource.NewQuantity(int64(val), resource.DecimalSI), nil
		},
	})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "Pods",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			return resource.NewQuantity(int64(len(items.Items)), resource.DecimalSI), nil
		},
	})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "Services",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.services(store, namespace)
			if err != nil {
				return nil, err
			}
			return resource.NewQuantity(int64(len(items.Items)), resource.DecimalSI), nil
		},
	})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "ReplicationControllers",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.replicationControllers(store, namespace)
			if err != nil {
				return nil, err
			}
			return resource.NewQuantity(int64(len(items.Items)), resource.DecimalSI), nil
		},
	})
	return observerFuncBindings
}

func (o *observer) containerObserverFuncBindings() []resourcecontroller.ObserverFuncBinding {
	observerFuncBindings := []resourcecontroller.ObserverFuncBinding{}

	groupBy := api.ResourceControllerGroupByContainer

	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "Memory",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for _, item := range items.Items {
				for _, container := range item.Spec.Containers {
					if container.Memory.Value() > val {
						val = container.Memory.Value()
					}
				}
			}
			return resource.NewQuantity(int64(val), resource.DecimalSI), nil
		},
	})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMin,
		ResourceName: "Memory",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for i, item := range items.Items {
				for j, container := range item.Spec.Containers {
					if container.Memory.Value() < val || (i == 0 && j == 0) {
						val = container.Memory.Value()
					}
				}
			}
			return resource.NewQuantity(int64(val), resource.DecimalSI), nil
		}})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "CPU",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for _, item := range items.Items {
				for _, container := range item.Spec.Containers {
					if container.CPU.MilliValue() > val {
						val = container.CPU.MilliValue()
					}
				}
			}
			return resource.NewMilliQuantity(int64(val), resource.DecimalSI), nil
		},
	})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMin,
		ResourceName: "CPU",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for i, item := range items.Items {
				for j, container := range item.Spec.Containers {
					if container.CPU.MilliValue() < val || (i == 0 && j == 0) {
						val = container.CPU.MilliValue()
					}
				}
			}
			return resource.NewMilliQuantity(int64(val), resource.DecimalSI), nil
		}})
	return observerFuncBindings
}

func (o *observer) podObserverFuncBindings() []resourcecontroller.ObserverFuncBinding {
	observerFuncBindings := []resourcecontroller.ObserverFuncBinding{}

	groupBy := api.ResourceControllerGroupByPod

	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "CPU",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for _, item := range items.Items {
				internalVal := int64(0)
				for _, container := range item.Spec.Containers {
					internalVal = internalVal + container.CPU.MilliValue()
				}
				if internalVal > val {
					val = internalVal
				}
			}
			return resource.NewMilliQuantity(int64(val), resource.DecimalSI), nil
		}})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMin,
		ResourceName: "CPU",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for index, item := range items.Items {
				internalVal := int64(0)
				for _, container := range item.Spec.Containers {
					internalVal = internalVal + container.CPU.MilliValue()
				}
				if index == 0 || internalVal < val {
					val = internalVal
				}
			}
			return resource.NewMilliQuantity(int64(val), resource.DecimalSI), nil
		}})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "Memory",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for _, item := range items.Items {
				internalVal := int64(0)
				for _, container := range item.Spec.Containers {
					internalVal = internalVal + container.Memory.Value()
				}
				if internalVal > val {
					val = internalVal
				}
			}
			return resource.NewQuantity(int64(val), resource.DecimalSI), nil
		}})
	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      groupBy,
		RuleType:     api.ResourceControllerRuleTypeMin,
		ResourceName: "Memory",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.pods(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for index, item := range items.Items {
				internalVal := int64(0)
				for _, container := range item.Spec.Containers {
					internalVal = internalVal + container.Memory.Value()
				}
				if index == 0 || internalVal < val {
					val = internalVal
				}
			}
			return resource.NewQuantity(int64(val), resource.DecimalSI), nil
		}})

	return observerFuncBindings
}

func (o *observer) replicationControllerObserverFuncBindings() []resourcecontroller.ObserverFuncBinding {
	observerFuncBindings := []resourcecontroller.ObserverFuncBinding{}

	observerFuncBindings = append(observerFuncBindings, resourcecontroller.ObserverFuncBinding{
		GroupBy:      api.ResourceControllerGroupByReplicationController,
		RuleType:     api.ResourceControllerRuleTypeMax,
		ResourceName: "Replicas",
		Func: func(store cache.Store, namespace string) (*resource.Quantity, error) {
			items, err := o.replicationControllers(store, namespace)
			if err != nil {
				return nil, err
			}
			val := int64(0)
			for _, item := range items.Items {
				internalVal := int64(item.Spec.Replicas)
				if internalVal > val {
					val = internalVal
				}
			}
			return resource.NewQuantity(int64(val), resource.DecimalSI), nil
		}})

	return observerFuncBindings
}

func (o *observer) ObserverFuncBindings() []resourcecontroller.ObserverFuncBinding {
	observerFuncBindings := []resourcecontroller.ObserverFuncBinding{}

	observerFuncBindings = append(observerFuncBindings, o.namespaceObserverFuncBindings()...)
	observerFuncBindings = append(observerFuncBindings, o.containerObserverFuncBindings()...)
	observerFuncBindings = append(observerFuncBindings, o.podObserverFuncBindings()...)
	observerFuncBindings = append(observerFuncBindings, o.replicationControllerObserverFuncBindings()...)

	return observerFuncBindings
}
