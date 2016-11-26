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
	"net/url"

	"github.com/jimmidyson/keycloak-operator/pkg/k8sutil"

	"github.com/go-kit/kit/log"
	"k8s.io/client-go/1.5/kubernetes"
	apierrors "k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/v1"
	extensionsobj "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/rest"
)

const (
	tprRealm    = "realm.keycloak.org"
	tprKeycloak = "server.keycloak.org"
)

// Operator manages lify cycle of Prometheus deployments and
// monitoring configurations.
type Operator struct {
	kclient *kubernetes.Clientset
	logger  log.Logger

	host string
}

// Config defines configuration parameters for the Operator.
type Config struct {
	Host        string
	TLSInsecure bool
	TLSConfig   rest.TLSClientConfig
}

// New creates a new controller.
func New(c Config, logger log.Logger) (*Operator, error) {
	cfg, err := newClusterConfig(c.Host, c.TLSInsecure, &c.TLSConfig)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Operator{
		kclient: client,
		logger:  logger,
		host:    cfg.Host,
	}, nil
}

// Run the controller.
func (c *Operator) Run(stopc <-chan struct{}) error {
	v, err := c.kclient.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("communicating with server failed: %s", err)
	}
	c.logger.Log("msg", "connection established", "cluster-version", v)

	if err := c.createTPRs(); err != nil {
		return err
	}

	<-stopc
	return nil
}

func (c *Operator) createTPRs() error {
	tprs := []*extensionsobj.ThirdPartyResource{
		{
			ObjectMeta: v1.ObjectMeta{
				Name: tprRealm,
			},
			Versions: []extensionsobj.APIVersion{
				{Name: "v1alpha1"},
			},
			Description: "Keycloak Realm",
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name: tprKeycloak,
			},
			Versions: []extensionsobj.APIVersion{
				{Name: "v1alpha1"},
			},
			Description: "Managed Keycloak server",
		},
	}
	tprClient := c.kclient.Extensions().ThirdPartyResources()

	for _, tpr := range tprs {
		if _, err := tprClient.Create(tpr); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		c.logger.Log("msg", "TPR created", "tpr", tpr.Name)
	}

	// We have to wait for the TPRs to be ready. Otherwise the initial watch may fail.
	err := k8sutil.WaitForMonitoringTPRReady(c.kclient.CoreClient.Client, c.host, "keycloaks")
	if err != nil {
		return err
	}
	return k8sutil.WaitForMonitoringTPRReady(c.kclient.CoreClient.Client, c.host, "realms")
}

func newClusterConfig(host string, tlsInsecure bool, tlsConfig *rest.TLSClientConfig) (*rest.Config, error) {
	var cfg *rest.Config
	var err error

	if len(host) == 0 {
		if cfg, err = rest.InClusterConfig(); err != nil {
			return nil, err
		}
	} else {
		cfg = &rest.Config{
			Host: host,
		}
		hostURL, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("error parsing host url %s : %v", host, err)
		}
		if hostURL.Scheme == "https" {
			cfg.TLSClientConfig = *tlsConfig
			cfg.Insecure = tlsInsecure
		}
	}
	cfg.QPS = 100
	cfg.Burst = 100

	return cfg, nil
}
