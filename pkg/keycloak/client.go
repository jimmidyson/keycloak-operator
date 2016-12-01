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
	"encoding/json"
	"time"

	"github.com/jimmidyson/keycloak-operator/pkg/spec"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/runtime/serializer"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
)

const resyncPeriod = 30 * time.Second

func NewKeycloakRESTClient(c rest.Config) (*rest.RESTClient, error) {
	c.APIPath = "/apis"
	c.GroupVersion = &unversioned.GroupVersion{
		Group:   TPRGroup,
		Version: TPRVersion,
	}
	// TODO(fabxc): is this even used with our custom list/watch functions?
	c.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}
	return rest.RESTClientFor(&c)
}

type keycloakDecoder struct {
	dec   *json.Decoder
	close func() error
}

func (d *keycloakDecoder) Close() {
	d.close()
}

func (d *keycloakDecoder) Decode() (action watch.EventType, object runtime.Object, err error) {
	var e struct {
		Type   watch.EventType
		Object spec.Keycloak
	}
	if err := d.dec.Decode(&e); err != nil {
		return watch.Error, nil, err
	}
	return e.Type, &e.Object, nil
}

// NewKeycloakListWatch returns a new ListWatch on the Keycloak resource.
func NewKeycloakListWatch(client *rest.RESTClient) *cache.ListWatch {
	return &cache.ListWatch{
		ListFunc: func(options api.ListOptions) (runtime.Object, error) {
			req := client.Get().
				Namespace(api.NamespaceAll).
				Resource(TPRServerKind).
				// VersionedParams(&options, api.ParameterCodec)
				FieldsSelectorParam(nil)

			b, err := req.DoRaw()
			if err != nil {
				return nil, err
			}
			var p spec.KeycloakList
			return &p, json.Unmarshal(b, &p)
		},
		WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
			r, err := client.Get().
				Prefix("watch").
				Namespace(api.NamespaceAll).
				Resource(TPRServerKind).
				// VersionedParams(&options, api.ParameterCodec).
				FieldsSelectorParam(nil).
				Stream()
			if err != nil {
				return nil, err
			}
			return watch.NewStreamWatcher(&keycloakDecoder{
				dec:   json.NewDecoder(r),
				close: r.Close,
			}), nil
		},
	}
}
