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

package alitasks

import (
	"fmt"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	"github.com/golang/glog"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/aliup"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"
)

//go:generate fitask -type=NatGateway
type NatGateway struct {
	Name      *string
	Lifecycle *fi.Lifecycle

	VPC       *VPC
	Region *string
	ID     *string
	Shared *bool
}

var _ fi.CompareWithID = &NatGateway{}

func (e *NatGateway) CompareWithID() *string {
	return e.ID
}

func (e *NatGateway) Find(c *fi.Context) (*NatGateway, error) {
	cloud := c.Cloud.(aliup.ALICloud)

	request := &ecs.DescribeNatGatewaysArgs{
		RegionId: common.Region(cloud.Region()),
	}

	if fi.StringValue(e.ID) != "" {
		request.NatGatewayId = fi.StringValue(e.ID)
	}

	natGateways, _, err := cloud.VpcClient().DescribeNatGateways(request)

	if err != nil {
		return nil, fmt.Errorf("error listing NatGateways: %v", err)
	}

	if fi.BoolValue(e.Shared) {
		if len(natGateways) != 1 {
			return nil, fmt.Errorf("found multiple NatGateways for %q", fi.StringValue(e.ID))
		} else {
			actual := &NatGateway{
				ID:        fi.String(natGateways[0].NatGatewayId),
				Name:      fi.String(natGateways[0].Name),
				Region:    fi.String(cloud.Region()),
				Shared:    e.Shared,
				Lifecycle: e.Lifecycle,
			}
			e.ID = actual.ID
			glog.V(4).Infof("found matching NatGateway %v", actual)
			return actual, nil
		}
	}

	if natGateways == nil || len(natGateways) == 0 {
		return nil, nil
	}

	for _, n := range natGateways {
		if n.Name == fi.StringValue(e.Name) {
			actual := &NatGateway{
				ID:        fi.String(n.NatGatewayId),
				Name:      fi.String(n.Name),
				Region:    fi.String(cloud.Region()),
				Shared:    e.Shared,
				Lifecycle: e.Lifecycle,
			}
			e.ID = actual.ID
			glog.V(4).Infof("found matching NatGateway %v", actual)
			return actual, nil
		}
	}

	return nil, nil
}

func (s *NatGateway) CheckChanges(a, e, changes *NatGateway) error {
	if a == nil {
		if e.Name == nil {
			return fi.RequiredField("Name")
		}
	} else {
		if changes.VPC.ID != nil {
			return fi.CannotChangeField("VPC")
		}
	}

	return nil
}

func (e *NatGateway) Run(c *fi.Context) error {
	return fi.DefaultDeltaRunMethod(e, c)
}

func (_ *NatGateway) RenderALI(t *aliup.ALIAPITarget, a, e, changes *NatGateway) error {

	if fi.BoolValue(e.Shared) && a == nil {
		return fmt.Errorf("NatGateway with id %q not found", fi.StringValue(e.ID))
	}

	if a == nil {
		if e.ID != nil && fi.StringValue(e.ID) != "" {
			glog.V(2).Infof("Shared NatGateway with NatGatewayID: %q", *e.ID)
			return nil
		}

		request := &ecs.CreateNatGatewayArgs{
			RegionId:  common.Region(t.Cloud.Region()),
			VpcId: fi.StringValue(e.VPC.ID),
			Name: fi.StringValue(e.Name),
		}

		response, err := t.Cloud.VpcClient().CreateNatGateway(request)
		if err != nil {
			return fmt.Errorf("error creating NatGateway: %v", err)
		}
		e.ID = fi.String(response.NatGatewayId)
	}
	return nil
}

type terraformNatGateway struct {
	Name *string `json:"name,omitempty"`
	VpcId *string `json:"vpc_id,omitempty"`
}

func (_ *NatGateway) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *NatGateway) error {
	if err := t.AddOutputVariable("id", e.TerraformLink()); err != nil {
		return err
	}

	tf := &terraformNatGateway{
		Name: e.Name,
		VpcId: e.VPC.ID,
	}

	return t.RenderResource("alicloud_nat_gateway", *e.Name, tf)
}

func (e *NatGateway) TerraformLink() *terraform.Literal {
	return terraform.LiteralProperty("alicloud_nat_gateway", *e.Name, "id")
}
