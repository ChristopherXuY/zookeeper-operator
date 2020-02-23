// Copyright 2018 The zookeeper-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"
	"fmt"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	api "zookeeper-operator/apis/zookeeper/v1alpha1"
	"zookeeper-operator/cluster"
	"zookeeper-operator/generated/clientset/versioned"
	zkInformer "zookeeper-operator/generated/informers/externalversions/zookeeper/v1alpha1"
	"zookeeper-operator/util"
	"zookeeper-operator/util/k8sutil"

	"github.com/sirupsen/logrus"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	kwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

var initRetryWaitTime = 30 * time.Second

type Event struct {
	Type   kwatch.EventType
	Object *api.ZookeeperCluster
}

type Controller struct {
	logger *logrus.Entry
	Config

	ctx      context.Context
	clusters map[string]*util.Tuple

	eventQueue workqueue.RateLimitingInterface
	zkInformer zkInformer.ZookeeperClusterInformer
}

type Config struct {
	Namespace      string
	ClusterWide    bool
	ServiceAccount string
	KubeCli        kubernetes.Interface
	KubeExtCli     apiextensionsclient.Interface
	ZookeeperCRCli versioned.Interface
	CreateCRD      bool
}

func New(cfg Config, ctx context.Context) *Controller {
	return &Controller{
		logger: logrus.WithField("pkg", "controller"),

		Config:   cfg,
		ctx:      ctx,
		clusters: make(map[string]*util.Tuple),
	}
}

// ProcessNextWorkItem processes next item in queue by syncHandler
func (c *Controller) processNextWorkItem() bool {
	obj, quit := c.eventQueue.Get()
	if quit {
		return false
	}
	defer c.eventQueue.Done(obj)

	key := obj.(string)
	forget, err := c.syncHandler(key)
	if err == nil {
		if forget {
			c.eventQueue.Forget(key)
		}
		return true
	}

	utilruntime.HandleError(fmt.Errorf("Error syncing job: %v", err))
	c.eventQueue.AddRateLimited(key)

	return true
}

func (c *Controller) syncHandler(key string) (bool, error) {
	ns, name, err := cache.SplitMetaNamespaceKey(key)

	if err != nil {
		return false, err
	}
	if len(ns) == 0 || len(name) == 0 {
		return false, fmt.Errorf("invalid zookeeper cluster key %q: either namespace or name is missing", key)
	}
	sharedCluster, err := c.zkInformer.Lister().ZookeeperClusters(ns).Get(name)
	sharedCluster = sharedCluster.DeepCopy()
	zkCluster := cluster.New(c.makeClusterConfig(), sharedCluster)
	start := time.Now()

	if sharedCluster.Status.Phase == api.ClusterPhaseNone {
		err = zkCluster.Create()
		if err != nil {
			return false, err
		}
	}
	rerr := zkCluster.Sync()

	cluster.ReconcileHistogram.WithLabelValues(sharedCluster.Name).Observe(time.Since(start).Seconds())

	if rerr != nil {
		cluster.ReconcileFailed.WithLabelValues(rerr.Error()).Inc()
		if cluster.IsFatalError(rerr) {
			sharedCluster.Status.SetReason(rerr.Error())
			c.logger.Errorf("cluster failed: %v", rerr)
			zkCluster.ReportFailedStatus()
		}
		return false, rerr
	}

	if !zkCluster.IsFinished() {
		c.eventQueue.AddAfter(key, 30*time.Second)
	}
	return true, nil
}

func (c *Controller) makeClusterConfig() cluster.Config {
	return cluster.Config{
		KubeCli:        c.Config.KubeCli,
		ZookeeperCRCli: c.Config.ZookeeperCRCli,
	}
}

func (c *Controller) initCRD() error {
	err := k8sutil.CreateCRD(c.KubeExtCli, api.ZookeeperClusterCRDName, api.ZookeeperClusterResourceKind, api.ZookeeperClusterResourcePlural, "zookeeper")
	if err != nil {
		return fmt.Errorf("failed to create CRD: %v", err)
	}
	return k8sutil.WaitCRDReady(c.KubeExtCli, api.ZookeeperClusterCRDName)
}