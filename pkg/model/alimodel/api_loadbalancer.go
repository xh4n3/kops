/*
Copyright 2018 The Kubernetes Authors.

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

package alimodel

import (
	"fmt"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/dns"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/alitasks"
	"k8s.io/kops/upup/pkg/fi/fitasks"
)

const LoadBalancerListenerStatus = "running"
const LoadBalancerListenerBandwidth = -1

// APILoadBalancerModelBuilder builds a LoadBalancer for accessing the API
type APILoadBalancerModelBuilder struct {
	*ALIModelContext
	Lifecycle *fi.Lifecycle
}

var _ fi.ModelBuilder = &APILoadBalancerModelBuilder{}

func (b *APILoadBalancerModelBuilder) Build(c *fi.ModelBuilderContext) error {
	// Configuration where an ELB fronts the API
	if !b.UseLoadBalancerForAPI() {
		return nil
	}

	lbSpec := b.Cluster.Spec.API.LoadBalancer
	if lbSpec == nil {
		// Skipping API ELB creation; not requested in Spec
		return nil
	}

	switch lbSpec.Type {
	case kops.LoadBalancerTypeInternal:
		return fmt.Errorf("internal LoadBalancers are not yet supported by kops on ALI")
	case kops.LoadBalancerTypePublic:
		// pass
	default:
		return fmt.Errorf("unhandled LoadBalancer type %q", lbSpec.Type)
	}

	// Create LoadBalancer for API ELB
	var loadbalancer *alitasks.LoadBalancer
	{

		loadbalancer = &alitasks.LoadBalancer{
			Name:      s("api." + b.ClusterName()),
			Lifecycle: b.Lifecycle,
		}

		switch lbSpec.Type {
		case kops.LoadBalancerTypeInternal:
			return fmt.Errorf("Don't support internal loadbalancer type: %q", lbSpec.Type)
		case kops.LoadBalancerTypePublic:
			loadbalancer.AddressType = s("internet")
		default:
			return fmt.Errorf("unknown loadbalancer Type: %q", lbSpec.Type)
		}

		c.AddTask(loadbalancer)
	}

	// Create LoadBalancerListener for API ELB
	// TODO: Health check
	var loadbalancerlistener *alitasks.LoadBalancerListener
	{
		loadBalancerListenerStatus := LoadBalancerListenerStatus
		loadBalancerListenerBandwidth := LoadBalancerListenerBandwidth
		loadbalancerlistener = &alitasks.LoadBalancerListener{
			Name:              s("api." + b.ClusterName()),
			Lifecycle:         b.Lifecycle,
			LoadBalancer:      loadbalancer,
			ListenerStatus:    s(loadBalancerListenerStatus),
			ListenerPort:      i(443),
			BackendServerPort: i(443),
			Bandwidth:         i(loadBalancerListenerBandwidth),
		}
		c.AddTask(loadbalancerlistener)
	}

	// Create LoadBalancerWhiteList for API ELB
	var loadbalancerwhiteList *alitasks.LoadBalancerWhiteList
	{
		sourceItems := ""
		for _, cidr := range b.Cluster.Spec.KubernetesAPIAccess {
			sourceItems = sourceItems + cidr + ","
		}
		loadbalancerwhiteList = &alitasks.LoadBalancerWhiteList{
			Name:                 s("api." + b.ClusterName()),
			Lifecycle:            b.Lifecycle,
			LoadBalancer:         loadbalancer,
			LoadBalancerListener: loadbalancerlistener,
			SourceItems:          s(sourceItems),
		}
		c.AddTask(loadbalancerwhiteList)
	}

	// TODO: Should this be removed?
	// Temporarily do not know the role of the following function
	if dns.IsGossipHostname(b.Cluster.Name) || b.UsePrivateDNS() {
		// Ensure the ELB hostname is included in the TLS certificate,
		// if we're not going to use an alias for it
		// TODO: I don't love this technique for finding the task by name & modifying it
		masterKeypairTask, found := c.Tasks["Keypair/master"]
		if !found {
			return fmt.Errorf("keypair/master task not found")
		}
		masterKeypair := masterKeypairTask.(*fitasks.Keypair)
		masterKeypair.AlternateNameTasks = append(masterKeypair.AlternateNameTasks, loadbalancer)
	}

	return nil

}
