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
	"time"

	"github.com/jimmidyson/keycloak-operator/pkg/analytics"
	"github.com/jimmidyson/keycloak-operator/pkg/k8sutil"
	"github.com/jimmidyson/keycloak-operator/pkg/queue"
	"github.com/jimmidyson/keycloak-operator/pkg/spec"

	"github.com/go-kit/kit/log"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	apierrors "k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/v1"
	extensionsobj "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	utilruntime "k8s.io/client-go/1.5/pkg/util/runtime"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
)

const (
	TPRGroup      = "keycloak.org"
	TPRVersion    = "v1alpha1"
	TPRRealmKind  = "keycloakrealms"
	TPRServerKind = "keycloakservers"

	tprRealm    = "keycloak-realm." + TPRGroup
	tprKeycloak = "keycloak-server." + TPRGroup
)

// Operator manages lify cycle of Prometheus deployments and
// monitoring configurations.
type Operator struct {
	kclient        *kubernetes.Clientset
	keycloakClient *rest.RESTClient
	logger         log.Logger

	serverInf     cache.SharedIndexInformer
	deploymentInf cache.SharedIndexInformer

	queue *queue.Queue
}

// New creates a new controller.
func New(cfg *rest.Config, logger log.Logger) (*Operator, error) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	keycloakClient, err := NewKeycloakRESTClient(*cfg)
	if err != nil {
		return nil, err
	}

	c := &Operator{
		keycloakClient: keycloakClient,
		kclient:        client,
		logger:         logger,
		queue:          queue.New(),
	}

	c.serverInf = cache.NewSharedIndexInformer(
		NewKeycloakListWatch(c.keycloakClient),
		&spec.Keycloak{},
		resyncPeriod,
		cache.Indexers{},
	)
	c.deploymentInf = cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(c.kclient.Extensions().GetRESTClient(), "deployments", api.NamespaceAll, nil),
		&extensionsobj.Deployment{},
		resyncPeriod,
		cache.Indexers{},
	)

	c.serverInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleAddKeycloak,
		DeleteFunc: c.handleDeleteKeycloak,
		UpdateFunc: c.handleUpdateKeycloak,
	})
	c.deploymentInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		// TODO(fabxc): only enqueue Keycloak an affected deployment belonged to.
		AddFunc: func(d interface{}) {
			c.logger.Log("msg", "addDeployment", "trigger", "depl add")
			c.handleAddDeployment(d)
		},
		DeleteFunc: func(d interface{}) {
			c.logger.Log("msg", "deleteDeployment", "trigger", "depl delete")
			c.handleDeleteDeployment(d)
		},
		UpdateFunc: func(old, cur interface{}) {
			c.logger.Log("msg", "updateDeployment", "trigger", "depl update")
			c.handleUpdateDeployment(old, cur)
		},
	})

	return c, nil
}

// Run the controller.
func (c *Operator) Run(stopc <-chan struct{}) error {
	defer c.queue.ShutDown()

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

	go c.worker()

	go c.serverInf.Run(stopc)
	go c.deploymentInf.Run(stopc)

	<-stopc
	return nil
}

func (c *Operator) keyFunc(obj interface{}) (string, bool) {
	k, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		c.logger.Log("msg", "creating key failed", "err", err)
		return k, false
	}
	return k, true
}

func (c *Operator) handleAddKeycloak(obj interface{}) {
	key, ok := c.keyFunc(obj)
	if !ok {
		return
	}

	analytics.KeycloakCreated()
	c.logger.Log("msg", "Keycloak added", "key", key)
	c.enqueue(key)
}

func (c *Operator) handleDeleteKeycloak(obj interface{}) {
	key, ok := c.keyFunc(obj)
	if !ok {
		return
	}

	analytics.KeycloakDeleted()
	c.logger.Log("msg", "Keycloak deleted", "key", key)
	c.enqueue(key)
}

func (c *Operator) handleUpdateKeycloak(old, cur interface{}) {
	// oldp := old.(*spec.Prometheus)
	// curp := cur.(*spec.Prometheus)

	key, ok := c.keyFunc(cur)
	if !ok {
		return
	}

	c.logger.Log("msg", "Keycloak updated", "key", key)
	c.enqueue(key)
}

// enqueue adds a key to the queue. If obj is a key already it gets added directly.
// Otherwise, the key is extracted via keyFunc.
func (c *Operator) enqueue(obj interface{}) {
	if obj == nil {
		return
	}

	key, ok := obj.(string)
	if !ok {
		key, ok = c.keyFunc(obj)
		if !ok {
			return
		}
	}

	c.queue.Add(key)
}

// worker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *Operator) worker() {
	for {
		key, quit := c.queue.Get()
		if quit {
			return
		}
		if err := c.sync(key.(string)); err != nil {
			utilruntime.HandleError(fmt.Errorf("reconciliation failed, re-enqueueing: %s", err))
			// We only mark the item as done after waiting. In the meantime
			// other items can be processed but the same item won't be processed again.
			// This is a trivial form of rate-limiting that is sufficient for our throughput
			// and latency expectations.
			go func() {
				time.Sleep(3 * time.Second)
				c.queue.Done(key)
			}()
			continue
		}

		c.queue.Done(key)
	}
}

func (c *Operator) keycloakForDeployment(obj interface{}) *spec.Keycloak {
	key, ok := c.keyFunc(obj)
	if !ok {
		return nil
	}
	// Namespace/Name are one-to-one so the key will find the respective Keycloak resource.
	k, exists, err := c.serverInf.GetStore().GetByKey(key)
	if err != nil {
		c.logger.Log("msg", "Keycloak lookup failed", "err", err)
		return nil
	}
	if !exists {
		return nil
	}
	return k.(*spec.Keycloak)
}

func (c *Operator) handleDeleteDeployment(obj interface{}) {
	if d := c.keycloakForDeployment(obj); d != nil {
		c.enqueue(d)
	}
}

func (c *Operator) handleAddDeployment(obj interface{}) {
	if d := c.keycloakForDeployment(obj); d != nil {
		c.enqueue(d)
	}
}

func (c *Operator) handleUpdateDeployment(oldo, curo interface{}) {
	old := oldo.(*extensionsobj.Deployment)
	cur := curo.(*extensionsobj.Deployment)

	c.logger.Log("msg", "update handler", "old", old.ResourceVersion, "cur", cur.ResourceVersion)

	// Periodic resync may resend the deployment without changes in-between.
	// Also breaks loops created by updating the resource ourselves.
	if old.ResourceVersion == cur.ResourceVersion {
		return
	}

	// Wake up Keycloak resource the deployment belongs to.
	if k := c.keycloakForDeployment(cur); k != nil {
		c.enqueue(k)
	}
}

func (c *Operator) sync(key string) error {
	c.logger.Log("msg", "reconcile keycloak", "key", key)

	obj, exists, err := c.serverInf.GetIndexer().GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		return c.destroyKeycloak(key)
	}

	k := obj.(*spec.Keycloak)

	// Create governing service if it doesn't exist.
	svcClient := c.kclient.Core().Services(k.Namespace)
	if _, err = svcClient.Create(makeDeploymentService(k)); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create deployment service: %s", err)
	}

	deploymentClient := c.kclient.Extensions().Deployments(k.Namespace)
	// Ensure we have a Deployment running Keycloak deployed.
	obj, exists, err = c.deploymentInf.GetIndexer().GetByKey(key)
	if err != nil {
		return err
	}

	if !exists {
		if _, err := deploymentClient.Create(makeDeployment(*k, nil)); err != nil {
			return fmt.Errorf("create deployment: %s", err)
		}
		return nil
	}
	if _, err := deploymentClient.Update(makeDeployment(*k, obj.(*extensionsobj.Deployment))); err != nil {
		return err
	}

	return nil
}

func (c *Operator) destroyKeycloak(key string) error {
	obj, exists, err := c.deploymentInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	deployment := obj.(*extensionsobj.Deployment)

	// Only necessary until GC is properly implemented in both Kubernetes and OpenShift.
	scaleClient := c.kclient.Extensions().Scales(deployment.Namespace)
	if _, err := scaleClient.Update("deployment", &extensionsobj.Scale{
		ObjectMeta: v1.ObjectMeta{
			Namespace: deployment.Namespace,
			Name:      deployment.Name,
		},
		Spec: extensionsobj.ScaleSpec{
			Replicas: 0,
		},
	}); err != nil {
		return err
	}

	deploymentClient := c.kclient.Extensions().Deployments(deployment.Namespace)
	currentGeneration := deployment.Generation
	if err := wait.PollInfinite(1*time.Second, func() (bool, error) {
		updatedDeployment, err := deploymentClient.Get(deployment.Name)
		if err != nil {
			return false, err
		}
		return updatedDeployment.Status.ObservedGeneration >= currentGeneration &&
			updatedDeployment.Status.Replicas == 0, nil
	}); err != nil {
		return err
	}

	// Let's get ready for proper GC by ensuring orphans are not left behind.
	orphan := false
	return deploymentClient.Delete(deployment.ObjectMeta.Name, &api.DeleteOptions{OrphanDependents: &orphan})
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
