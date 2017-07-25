/*
Copyright 2016 The Kubernetes Authors.

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

package install

import (
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/quota"
	"k8s.io/kubernetes/pkg/quota/evaluator/core"
	"k8s.io/kubernetes/pkg/quota/generic"
)

// NewDynamicRegistry returns a registry of quota evaluators.
// If a shared informer factory is provided, it is used by evaluators
// rather than performing direct queries.
func NewDynamicRegistry(discoveryResourcesFn generic.DiscoveryResourcesFunc, kubeClient clientset.Interface, f informers.SharedInformerFactory) (quota.Registry, error) {
	resources, err := discoveryResourcesFn()
	if err != nil {
		return nil, err
	}

	// any resource that can be created and deleted can be managed by quota
	quotableGroupVersionResources := []schema.GroupKind{}
	for _, item := range resources {
		gv, err := schema.ParseGroupVersion(item.GroupVersion)
		if err != nil {
			glog.Errorf("Failed to parse GroupVersion %q, skipping: %v", item.GroupVersion, err)
			continue
		}

		for _, r := range item.APIResources {
			gvr := schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: r.Name}
			if !r.Namespaced {
				glog.V(6).Infof("Skipping resource %v because it is not namespaced.", gvr)
				continue
			}
			verbs := sets.NewString([]string(r.Verbs)...)
			if !verbs.Has("create") {
				glog.V(6).Infof("Skipping resource %v because it cannot be created.", gvr)
				continue
			}
			if !verbs.Has("delete") {
				glog.V(6).Infof("Skipping resource %v because it cannot be deleted.", gvr)
				continue
			}
			quotableGroupVersionResources = append(quotableGroupVersionResources, gvr)
		}
	}

	evaluators := map[schema.GroupKind]Evaluator{}
	for _, item := range quotableGroupVersionResources {

		if f != nil {
			genericInformer, err := f.ForResource(item)
			if err != nil {
				return nil, err
			}
			listFuncByNamespace := func(namespace string, options metav1.ListOptions) ([]runtime.Object, error) {
				lister := genericInformer.Lister().ByNamespace(namespace)
				return lister.List(labels.Everything())
			}
		} else {
		}
		// type ListFuncByNamespace func(namespace string, options metav1.ListOptions) ([]runtime.Object, error)

		evaluator := generic.ObjectCountEvaluator{
			AllowCreateOnUpdate: false,               // TODO: this is not discoverable
			InternalGroupKind:   api.Kind(""),        // TODO: need to have this from discovery above
			ResourceName:        "test",              // TODO: need to have this generated safely from discovery above
			ListFuncByNamespace: listFuncByNamespace, // TODO: need to create this from something
		}
		// TODo add to list of evaluators
	}

	// TODO: merge evaluators above with bespoke evaluators
	return core.NewRegistry(kubeClient, f), nil
}

// NewRegistry returns a registry of quota evaluators.
// If a shared informer factory is provided, it is used by evaluators rather than performing direct queries.
func NewRegistry(kubeClient clientset.Interface, f informers.SharedInformerFactory) quota.Registry {
	// TODO: when quota supports resources in other api groups, we will need to merge
	return core.NewRegistry(kubeClient, f)
}
