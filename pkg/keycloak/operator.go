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

	"github.com/jimmidyson/keycloak-operator/pkg/k8sutil"

	"github.com/go-kit/kit/log"
	"k8s.io/client-go/1.5/kubernetes"
	apierrors "k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/v1"
	extensionsobj "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
)

const (
	TPRGroup      = "keycloak.org"
	TPRVersion    = "v1alpha1"
	TPRRealmKind  = "realms"
	TPRServerKind = "servers"

	tprRealm    = "realm." + TPRGroup
	tprKeycloak = "server." + TPRGroup
)

// Operator manages lify cycle of Prometheus deployments and
// monitoring configurations.
type Operator struct {
	kclient *kubernetes.Clientset
	logger  log.Logger

	host string
}

// New creates a new controller.
func New(client *kubernetes.Clientset, logger log.Logger) (*Operator, error) {
	return &Operator{
		kclient: client,
		logger:  logger,
	}, nil
}

// Run the controller.
func (c *Operator) Run(stopc <-chan struct{}) error {
	errChan := make(chan error)
	go func() {
		v, err := c.kclient.Discovery().ServerVersion()
		if err != nil {
			errChan <- fmt.Errorf("communicating with server failed: %s", err)
			return
		}
		c.logger.Log("msg", "connection established", "cluster-version", v)

		if err := c.createTPRs(); err != nil {
			errChan <- err
			return
		}
		errChan <- nil
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
		c.logger.Log("msg", "TPR API endpoints ready")
	case <-stopc:
		return nil
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
				{Name: TPRVersion},
			},
			Description: "Keycloak Realm",
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name: tprKeycloak,
			},
			Versions: []extensionsobj.APIVersion{
				{Name: TPRVersion},
			},
			Description: "Managed Keycloak server",
		},
	}
	tprClient := c.kclient.Extensions().ThirdPartyResources()

	for _, tpr := range tprs {
		_, err := tprClient.Create(tpr)
		if err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return err
			}
			c.logger.Log("msg", "TPR already exists", "tpr", tpr.Name)
		} else {
			c.logger.Log("msg", "TPR created", "tpr", tpr.Name)
		}
	}

	// We have to wait for the TPRs to be ready. Otherwise the initial watch may fail.
	err := k8sutil.WaitForTPRReady(c.kclient.CoreClient.GetRESTClient(), TPRGroup, TPRVersion, TPRServerKind)
	if err != nil {
		return err
	}
	return k8sutil.WaitForTPRReady(c.kclient.CoreClient.GetRESTClient(), TPRGroup, TPRVersion, TPRRealmKind)
}
