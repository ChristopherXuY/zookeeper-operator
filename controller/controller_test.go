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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	api "zookeeper-operator/apis/zookeeper/v1alpha1"
	"zookeeper-operator/zkcluster"
)

func TestHandleClusterEventUpdateFailedCluster(t *testing.T) {
	c := New(Config{})

	clus := &api.ZookeeperCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Status: api.ClusterStatus{
			Phase: api.ClusterPhaseFailed,
		},
	}
	e := &event{
		Type:   watch.Modified,
		Object: clus,
	}
	_, err := c.handleClusterEvent(e)
	prefix := "ignore failed zkcluster"
	if !strings.HasPrefix(err.Error(), prefix) {
		t.Errorf("expect err='%s...', get=%v", prefix, err)
	}
}

func TestHandleClusterEventDeleteFailedCluster(t *testing.T) {
	c := New(Config{})
	name := "tests"
	clus := &api.ZookeeperCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: api.ClusterStatus{
			Phase: api.ClusterPhaseFailed,
		},
	}
	e := &event{
		Type:   watch.Deleted,
		Object: clus,
	}

	c.clusters[name] = &zkcluster.Cluster{}

	if _, err := c.handleClusterEvent(e); err != nil {
		t.Fatal(err)
	}

	if c.clusters[name] != nil {
		t.Errorf("failed zkcluster not cleaned up after delete event, zkcluster struct: %v", c.clusters[name])
	}
}

func TestHandleClusterEventClusterwide(t *testing.T) {
	c := New(Config{ClusterWide: true})

	clus := &api.ZookeeperCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				"zookeeper.database.apache.com/scope": "clusterwide",
			},
		},
	}
	e := &event{
		Type:   watch.Modified,
		Object: clus,
	}
	if ignored, _ := c.handleClusterEvent(e); ignored {
		t.Errorf("zkcluster shouldn't be ignored")
	}
}

func TestHandleClusterEventClusterwideIgnored(t *testing.T) {
	c := New(Config{ClusterWide: true})

	clus := &api.ZookeeperCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	e := &event{
		Type:   watch.Modified,
		Object: clus,
	}
	if ignored, _ := c.handleClusterEvent(e); !ignored {
		t.Errorf("zkcluster should be ignored")
	}
}

func TestHandleClusterEventNamespacedIgnored(t *testing.T) {
	c := New(Config{})

	clus := &api.ZookeeperCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			Annotations: map[string]string{
				"zookeeper.database.apache.com/scope": "clusterwide",
			},
		},
	}
	e := &event{
		Type:   watch.Modified,
		Object: clus,
	}
	if ignored, _ := c.handleClusterEvent(e); !ignored {
		t.Errorf("zkcluster should be ignored")
	}
}
