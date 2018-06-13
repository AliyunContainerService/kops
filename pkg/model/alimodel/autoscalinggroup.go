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
	"k8s.io/kops/pkg/model"
	"k8s.io/kops/pkg/model/defaults"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/alitasks"
)

const DefaultVolumeType = "cloud_ssd"
const DefaultInstanceType = "ecs.n2.medium"

// AutoscalingGroupModelBuilder configures AutoscalingGroup objects
type AutoscalingGroupModelBuilder struct {
	*ALIModelContext

	BootstrapScript   *model.BootstrapScript
	Lifecycle         *fi.Lifecycle
	SecurityLifecycle *fi.Lifecycle
}

var _ fi.ModelBuilder = &AutoscalingGroupModelBuilder{}

func (b *AutoscalingGroupModelBuilder) Build(c *fi.ModelBuilderContext) error {
	var err error
	for _, ig := range b.InstanceGroups {
		name := b.GetAutoscalingGroupName(ig)

		//Create AutoscalingGroup
		var autoscalingGroup *alitasks.AutoscalingGroup
		{
			// TODO: Should we adjust the default value here?
			minSize := 1
			maxSize := 1
			if ig.Spec.MinSize != nil {
				minSize = int(fi.Int32Value(ig.Spec.MinSize))
			} else if ig.Spec.Role == kops.InstanceGroupRoleNode {
				minSize = 2
			}
			if ig.Spec.MaxSize != nil {
				maxSize = int(fi.Int32Value(ig.Spec.MaxSize))
			} else if ig.Spec.Role == kops.InstanceGroupRoleNode {
				maxSize = 2
			}

			autoscalingGroup = &alitasks.AutoscalingGroup{
				Name:      s(name),
				Lifecycle: b.Lifecycle,
				MinSize:   i(minSize),
				MaxSize:   i(maxSize),
			}

			subnets, err := b.GatherSubnets(ig)
			if err != nil {
				return err
			}
			if len(subnets) == 0 {
				return fmt.Errorf("could not determine any subnets for InstanceGroup %q; subnets was %s", ig.ObjectMeta.Name, ig.Spec.Subnets)
			}
			for _, subnet := range subnets {
				autoscalingGroup.VSwitchs = append(autoscalingGroup.VSwitchs, b.LinkToVSwitch(subnet.Name))
			}

			if ig.Spec.Role == kops.InstanceGroupRoleMaster {
				autoscalingGroup.LoadBalancer = b.LinkLoadBalancer()
			}
			c.AddTask(autoscalingGroup)
		}

		// LaunchConfiguration
		var launchConfiguration *alitasks.LaunchConfiguration
		{
			volumeSize := fi.Int32Value(ig.Spec.RootVolumeSize)
			if volumeSize == 0 {
				volumeSize, err = defaults.DefaultInstanceGroupVolumeSize(ig.Spec.Role)
				if err != nil {
					return err
				}
			}
			volumeType := fi.StringValue(ig.Spec.RootVolumeType)

			if volumeType == "" {
				volumeType = DefaultVolumeType
			}

			instanceType := ig.Spec.MachineType
			if instanceType == "" {
				instanceType = DefaultInstanceType
			}

			tags, err := b.CloudTagsForInstanceGroup(ig)
			if err != nil {
				return fmt.Errorf("error building cloud tags: %v", err)
			}

			launchConfiguration = &alitasks.LaunchConfiguration{
				Name:             s(name),
				Lifecycle:        b.Lifecycle,
				AutoscalingGroup: b.LinkToAutoscalingGroup(ig),
				SecurityGroup:    b.LinkToSecurityGroup(ig.Spec.Role),
				RAMRole:          b.LinkToRAMRole(ig.Spec.Role),

				ImageId:            s(ig.Spec.Image),
				InstanceType:       s(instanceType),
				SystemDiskSize:     i(int(volumeSize)),
				SystemDiskCategory: s(volumeType),
				Tags:               tags,
			}

			if err != nil {
				return err
			} else {
				launchConfiguration.SSHKey = b.LinkToSSHKey()
			}
			if launchConfiguration.UserData, err = b.BootstrapScript.ResourceNodeUp(ig, b.Cluster); err != nil {
				return err
			}
		}
		c.AddTask(launchConfiguration)

	}

	return nil
}
