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
	"sync"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/golang/glog"
)

// All registered resource observer options.
var pluginsMutex sync.Mutex
var plugins = make(map[string]Factory)

// RegisterObserver registers an Observer plug-in with the system
func RegisterObserver(name string, factory Factory) {
	pluginsMutex.Lock()
	defer pluginsMutex.Unlock()

	_, found := plugins[name]
	if found {
		glog.Fatalf("Observer plugin with name: %q was registered twice", name)
	}

	glog.V(1).Infof("Registered Observer plugin with name: %q", name)
	plugins[name] = factory
}

// InitObservers instantiates each registered observer plug-in
func InitObservers(client client.Interface) []Observer {
	pluginsMutex.Lock()
	defer pluginsMutex.Unlock()

	observers := []Observer{}
	for pluginName, factory := range plugins {
		observer, err := factory(client)
		if err != nil {
			glog.Fatalf("Unable to initialize observer plugin %q with error %v", pluginName, err)
		}
		observers = append(observers, observer)
	}
	return observers
}
