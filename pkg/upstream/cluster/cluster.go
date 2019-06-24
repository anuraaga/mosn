/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cluster

import (
	"sofastack.io/sofa-mosn/pkg/api/v2"
	"sofastack.io/sofa-mosn/pkg/log"
	"sofastack.io/sofa-mosn/pkg/mtls"
	"sofastack.io/sofa-mosn/pkg/types"
	"sofastack.io/sofa-mosn/pkg/upstream/healthcheck"
	"sofastack.io/sofa-mosn/pkg/utils"
)

func NewCluster(clusterConfig v2.Cluster) types.Cluster {
	// TODO: support cluster type registered
	return newSimpleCluster(clusterConfig)
}

// simpleCluster is an implementation of types.Cluster
type simpleCluster struct {
	info          *clusterInfo
	healthChecker types.HealthChecker
	lbInstance    types.LoadBalancer // load balancer used for this cluster
	hostSet       *hostSet
}

func newSimpleCluster(clusterConfig v2.Cluster) *simpleCluster {
	info := &clusterInfo{
		name:                 clusterConfig.Name,
		clusterType:          clusterConfig.ClusterType,
		maxRequestsPerConn:   clusterConfig.MaxRequestPerConn,
		connBufferLimitBytes: clusterConfig.ConnBufferLimitBytes,
		stats:                newClusterStats(clusterConfig.Name),
		lbSubsetInfo:         NewLBSubsetInfo(&clusterConfig.LBSubSetConfig), // new subset load balancer info
		lbType:               types.LoadBalancerType(clusterConfig.LbType),
		resourceManager:      NewResourceManager(clusterConfig.CirBreThresholds),
	}
	// host set
	hostSet := &hostSet{}
	// load balance
	var lb types.LoadBalancer
	if info.lbSubsetInfo.IsEnabled() {
		lb = NewSubsetLoadBalancer(info.lbType, hostSet, info.stats, info.lbSubsetInfo)
	} else {
		lb = NewLoadBalancer(info.lbType, hostSet)
	}
	// tls mng
	mgr, err := mtls.NewTLSClientContextManager(&clusterConfig.TLS, info)
	if err != nil {
		log.DefaultLogger.Errorf("[upstream] [cluster] [new cluster] create tls context manager failed, %v", err)
	}
	info.tlsMng = mgr
	cluster := &simpleCluster{
		info:       info,
		lbInstance: lb,
		hostSet:    hostSet,
	}
	// health check
	if clusterConfig.HealthCheck.ServiceName != "" {
		log.DefaultLogger.Infof("[upstream] [cluster] [new cluster] cluster %s have health check", clusterConfig.Name)
		cluster.healthChecker = healthcheck.CreateHealthCheck(clusterConfig.HealthCheck, cluster)
		// add default call backs, for change host healthy status
		cluster.healthChecker.AddHostCheckCompleteCb(func(host types.Host, changedState bool, isHealthy bool) {
			if changedState {
				hostSet.refreshHealthHosts(host)
			}
		})
		hostSet.AdddMemberUpdateCb(cluster.healthChecker.OnClusterMemberUpdate)
		utils.GoWithRecover(func() {
			cluster.healthChecker.Start()
		}, nil)
	}
	return cluster

}

func (sc *simpleCluster) UpdateHosts(newHosts []types.Host) {
	sc.hostSet.UpdateHosts(newHosts)
}

func (sc *simpleCluster) RemoveHosts(addrs []string) {
	sc.hostSet.RemoveHosts(addrs)
}

func (sc *simpleCluster) Info() types.ClusterInfo {
	return sc.info
}

func (sc *simpleCluster) HostSet() types.HostSet {
	return sc.hostSet
}

func (sc *simpleCluster) LBInstance() types.LoadBalancer {
	return sc.lbInstance
}

func (sc *simpleCluster) AddHealthCheckCallbacks(cb types.HealthCheckCb) {
	if sc.healthChecker != nil {
		sc.healthChecker.AddHostCheckCompleteCb(cb)
	}
}

type clusterInfo struct {
	name                 string
	clusterType          v2.ClusterType
	lbType               types.LoadBalancerType // if use subset lb , lbType is used as inner LB algorithm for choosing subset's host
	connBufferLimitBytes uint32
	maxRequestsPerConn   uint32
	resourceManager      types.ResourceManager
	stats                types.ClusterStats
	lbSubsetInfo         types.LBSubsetInfo
	tlsMng               types.TLSContextManager
}

func (ci *clusterInfo) Name() string {
	return ci.name
}

func (ci *clusterInfo) ClusterType() v2.ClusterType {
	return ci.clusterType
}

func (ci *clusterInfo) LbType() types.LoadBalancerType {
	return ci.lbType
}

func (ci *clusterInfo) ConnBufferLimitBytes() uint32 {
	return ci.connBufferLimitBytes
}

func (ci *clusterInfo) MaxRequestsPerConn() uint32 {
	return ci.maxRequestsPerConn
}

func (ci *clusterInfo) Stats() types.ClusterStats {
	return ci.stats
}

func (ci *clusterInfo) ResourceManager() types.ResourceManager {
	return ci.resourceManager
}

func (ci *clusterInfo) TLSMng() types.TLSContextManager {
	return ci.tlsMng
}

func (ci *clusterInfo) LbSubsetInfo() types.LBSubsetInfo {
	return ci.lbSubsetInfo
}
