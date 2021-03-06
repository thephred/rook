/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package cephmgr

import (
	"path"
	"testing"

	etcd "github.com/coreos/etcd/client"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

// ************************************************************************************************
// ************************************************************************************************
//
// unit test functions
//
// ********************************"****************************************************************
// ************************************************************************************************
func TestCephLeaders(t *testing.T) {
	factory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	leader := newLeader(factory, "")

	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}
	nodes["a"] = &inventory.NodeConfig{PublicIP: "1.2.3.4"}

	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Inventory:  inv,
		ConfigDir:  "/tmp",
	}

	// mock the agent responses that the deployments were successful to start mons and osds
	etcdClient.WatcherResponses["/rook/_notify/a/monitor/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/b/monitor/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/a/osd/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/b/osd/status"] = "succeeded"

	// trigger a refresh event
	refresh := clusterd.NewRefreshEvent()
	refresh.Context = context
	leader.HandleRefresh(refresh)

	assert.True(t, etcdClient.GetChildDirs("/rook/services/ceph/osd/desired").Equals(util.CreateSet([]string{"a"})))
	assert.Equal(t, "mon0", etcdClient.GetValue("/rook/services/ceph/monitor/desired/a/id"))
	assert.Equal(t, "1.2.3.4", etcdClient.GetValue("/rook/services/ceph/monitor/desired/a/ipaddress"))
	assert.Equal(t, "6790", etcdClient.GetValue("/rook/services/ceph/monitor/desired/a/port"))

	// trigger an add node event
	nodes["b"] = &inventory.NodeConfig{PublicIP: "2.3.4.5"}
	refresh.NodesAdded.Add("b")
	leader.HandleRefresh(refresh)

	assert.True(t, etcdClient.GetChildDirs("/rook/services/ceph/osd/desired").Equals(util.CreateSet([]string{"a", "b"})))
	assert.Equal(t, "myfsid", etcdClient.GetValue("/rook/services/ceph/fsid"))
	assert.Equal(t, "mykey", etcdClient.GetValue("/rook/services/ceph/_secrets/admin"))
}

func TestExtractDesiredDeviceNode(t *testing.T) {
	// valid path with node id
	node, err := extractNodeIDFromDesiredDevice("/rook/services/ceph/osd/desired/abc/device/sdb")
	assert.Nil(t, err)
	assert.Equal(t, "abc", node)

	// node id not found
	key := "/rook/services/ceph/osd/desired"
	node, err = extractNodeIDFromDesiredDevice(key)
	assert.NotNil(t, err)
	assert.Equal(t, "", node)

	// ensure the handle device changed event can run without crashing
	response := &etcd.Response{Action: "create", Node: &etcd.Node{Key: key}}
	handleDeviceChanged(response, nil)

}

func TestRefreshKeys(t *testing.T) {
	factory := &testceph.MockConnectionFactory{Fsid: "fsid", SecretKey: "key"}
	leader := newLeader(factory, "")
	keys := leader.RefreshKeys()
	assert.Equal(t, 1, len(keys))

	expected := path.Join(mon.CephKey, "osd", clusterd.DesiredKey)
	assert.Equal(t, expected, keys[0].Path)
}

func TestNewCephService(t *testing.T) {
	factory := &testceph.MockConnectionFactory{Fsid: "fsid", SecretKey: "key"}

	service := NewCephService(factory, "a,b,c", true, "root=default", "")
	assert.NotNil(t, service)
	assert.Equal(t, "/rook/services/ceph/osd/desired", service.Leader.RefreshKeys()[0].Path)
	assert.Equal(t, 2, len(service.Agents))
	assert.Equal(t, "monitor", service.Agents[0].Name())
	assert.Equal(t, "osd", service.Agents[1].Name())
}

func TestCreateClusterInfo(t *testing.T) {
	// generate the secret key from the factory
	factory := &testceph.MockConnectionFactory{Fsid: "fsid", SecretKey: "admin1"}
	info, err := mon.CreateClusterInfo(factory, "")
	assert.Nil(t, err)
	assert.Equal(t, "fsid", info.FSID)
	assert.Equal(t, "admin1", info.AdminSecret)
	assert.Equal(t, "rookcluster", info.Name)

	// specify the desired secret key
	factory = &testceph.MockConnectionFactory{Fsid: "fsid"}
	info, err = mon.CreateClusterInfo(factory, "mysupersecret")
	assert.Equal(t, "fsid", info.FSID)
	assert.Equal(t, "mysupersecret", info.AdminSecret)
}
