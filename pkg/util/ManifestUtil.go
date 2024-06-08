/*
 * Copyright (c) 2024. Devtron Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package util

import (
	"github.com/devtron-labs/common-lib/utils/k8s/commonBean"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func GetPorts(manifest *unstructured.Unstructured, gvk schema.GroupVersionKind) []int64 {
	ports := make([]int64, 0)
	if gvk.Kind == commonBean.ServiceKind {
		ports = append(ports, getPortsFromService(manifest)...)
	}
	if gvk.Kind == commonBean.EndPointsSlice {
		ports = append(ports, getPortsFromEndPointsSlice(manifest)...)
	}
	if gvk.Kind == commonBean.EndpointsKind {
		ports = append(ports, getPortsFromEndpointsKind(manifest)...)
	}
	return ports
}

func getPortsFromService(manifest *unstructured.Unstructured) []int64 {
	var ports []int64
	if manifest.Object["spec"] != nil {
		spec := manifest.Object["spec"].(map[string]interface{})
		if spec["ports"] != nil {
			portList := spec["ports"].([]interface{})
			for _, portItem := range portList {
				if portItem.(map[string]interface{}) != nil {
					_portNumber := portItem.(map[string]interface{})["port"]
					portNumber := _portNumber.(int64)
					if portNumber != 0 {
						ports = append(ports, portNumber)
					}
				}
			}
		}
	}
	return ports
}

func getPortsFromEndPointsSlice(manifest *unstructured.Unstructured) []int64 {
	var ports []int64
	if manifest.Object["ports"] != nil {
		endPointsSlicePorts := manifest.Object["ports"].([]interface{})
		for _, val := range endPointsSlicePorts {
			_portNumber := val.(map[string]interface{})["port"]
			portNumber := _portNumber.(int64)
			if portNumber != 0 {
				ports = append(ports, portNumber)
			}
		}
	}
	return ports
}

func getPortsFromEndpointsKind(manifest *unstructured.Unstructured) []int64 {
	var ports []int64
	if manifest.Object["subsets"] != nil {
		subsets := manifest.Object["subsets"].([]interface{})
		for _, subset := range subsets {
			subsetObj := subset.(map[string]interface{})
			if subsetObj != nil {
				portsIfs := subsetObj["ports"].([]interface{})
				for _, portsIf := range portsIfs {
					portsIfObj := portsIf.(map[string]interface{})
					if portsIfObj != nil {
						port := portsIfObj["port"].(int64)
						ports = append(ports, port)
					}
				}
			}
		}
	}
	return ports
}
