//  Copyright 2016 Red Hat, Inc.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package keycloak

import (
	"fmt"

	"github.com/jimmidyson/keycloak-operator/pkg/spec"

	"k8s.io/client-go/1.5/pkg/api/resource"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/util/intstr"
)

func makeDeployment(k spec.Keycloak, old *v1beta1.Deployment) *v1beta1.Deployment {
	// TODO(fabxc): is this the right point to inject defaults?
	// Ideally we would do it before storing but that's currently not possible.
	// Potentially an update handler on first insertion.

	if k.Spec.BaseImage == "" {
		k.Spec.BaseImage = "jboss/keycloak"
	}
	if k.Spec.Version == "" {
		k.Spec.Version = "2.4.0.Final"
	}
	if k.Spec.Replicas < 1 {
		k.Spec.Replicas = 1
	}

	if k.Spec.Resources.Requests == nil {
		k.Spec.Resources.Requests = v1.ResourceList{}
	}
	if _, ok := k.Spec.Resources.Requests[v1.ResourceMemory]; !ok {
		k.Spec.Resources.Requests[v1.ResourceMemory] = resource.MustParse("256Mi")
	}

	deployment := &v1beta1.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name: k.Name,
		},
		Spec: makeDeploymentSpec(k),
	}
	if old != nil {
		deployment.Annotations = old.Annotations
	}
	return deployment
}

func makeDeploymentService(k *spec.Keycloak) *v1.Service {
	svc := &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name: "keycloak",
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "None",
			Ports: []v1.ServicePort{
				{
					Name:       "https",
					Port:       443,
					TargetPort: intstr.FromString("https"),
				},
			},
			Selector: map[string]string{
				"app": "keycloak",
			},
		},
	}
	return svc
}

func makeDeploymentSpec(k spec.Keycloak) v1beta1.DeploymentSpec {
	return v1beta1.DeploymentSpec{
		Replicas: &k.Spec.Replicas,
		Template: v1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{
					"app":      "keycloak",
					"keycloak": k.Name,
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:  "keycloak",
						Image: fmt.Sprintf("%s:%s", k.Spec.BaseImage, k.Spec.Version),
						Ports: []v1.ContainerPort{
							{
								Name:          "https",
								ContainerPort: 8443,
								Protocol:      v1.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}
