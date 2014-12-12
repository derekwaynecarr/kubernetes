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
	"sync"

	"github.com/golang/glog"
)

// All registered resource observer options.
var pluginsMutex sync.Mutex
var plugins = make(map[string]AdmissionFunc)

// RegisterAdmissionFunc registers an AdmissionFunc plug-in
func RegisterAdmissionFunc(name string, admissionFunc AdmissionFunc) {
	pluginsMutex.Lock()
	defer pluginsMutex.Unlock()

	_, found := plugins[name]
	if found {
		glog.Fatalf("AdmissionFunc plugin with name: %q was registered twice", name)
	}

	glog.V(1).Infof("Registered AdmissionFunc plugin with name: %q", name)
	plugins[name] = admissionFunc
}

// GetAdmissionFuncs returns each registered plug-in
func GetAdmissionFuncs() []AdmissionFunc {
	pluginsMutex.Lock()
	defer pluginsMutex.Unlock()

	admissionFuncs := []AdmissionFunc{}
	for _, admissionFunc := range plugins {
		admissionFuncs = append(admissionFuncs, admissionFunc)
	}
	return admissionFuncs
}
