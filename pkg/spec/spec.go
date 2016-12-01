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

package spec

import (
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
)

// Keycloak defines a Keycloak deployment.
type Keycloak struct {
	unversioned.TypeMeta `json:",inline"`
	v1.ObjectMeta        `json:"metadata,omitempty"`
	Spec                 KeycloakSpec `json:"spec"`
}

// KeycloakList is a list of Keycloak.
type KeycloakList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty"`

	Items []*Keycloak `json:"items"`
}

// KeycloakSpec holds specification parameters of a Keycloak deployment.
type KeycloakSpec struct {
	RealmSelector *unversioned.LabelSelector `json:"realmSelector"`
	Version       string                     `json:"version"`
	BaseImage     string                     `json:"baseImage"`
	Replicas      int32                      `json:"replicas"`
	Resources     v1.ResourceRequirements    `json:"resources"`
}
