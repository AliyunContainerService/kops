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

const (
	NatType = "Nat"
)

//go:generate fitask -type=EIP
type EIP struct {
	Name      *string
	Lifecycle *fi.Lifecycle

	Region       *string
	AllocationId *string
	IpAddress    *string
	NatGateway   *NatGateway
	Available    *bool
}

var _ fi.CompareWithID = &EIP{}

func (e *EIP) CompareWithID() *string {
	return e.Name
}

func (e *EIP) Find(c *fi.Context) (*EIP, error) {
	if e.NatGateway == nil || e.NatGateway.ID == nil {
		glog.V(4).Infof("NatGateway / NatGatewayId not found for %s, skipping Find", fi.StringValue(e.Name))
		return nil, nil
	}

	cloud := c.Cloud.(aliup.ALICloud)
	describeEipAddressesArgs := &ecs.DescribeEipAddressesArgs{
		RegionId:               common.Region(cloud.Region()),
		AssociatedInstanceType: ecs.AssociatedInstanceTypeNat,
		AssociatedInstanceId:   fi.StringValue(e.NatGateway.ID),
	}

	eipAddresses, _, err := cloud.EcsClient().DescribeEipAddresses(describeEipAddressesArgs)
	if err != nil {
		return nil, fmt.Errorf("error finding EIPs: %v", err)
	}
	// Don't exist EIPs with specified NatGateway.
	if len(eipAddresses) == 0 {
		return nil, nil
	}
	if len(eipAddresses) > 1 {
		glog.V(4).Infof("The number of specified EIPs with the same NatGatewayId exceeds 1, eipName:%q", *e.Name)
	}

	glog.V(2).Infof("found matching EIPs: %q", *e.Name)

	actual := &EIP{}
	actual.IpAddress = fi.String(eipAddresses[0].IpAddress)
	actual.AllocationId = fi.String(eipAddresses[0].AllocationId)
	actual.Available = fi.Bool(eipAddresses[0].Status == ecs.EipStatusAvailable)
	if eipAddresses[0].InstanceId != "" {
		actual.NatGateway = &NatGateway{
			ID: fi.String(eipAddresses[0].InstanceId),
		}
	}
	// Ignore "system" fields
	actual.Lifecycle = e.Lifecycle
	actual.Name = e.Name
	actual.Region = e.Region
	e.AllocationId = actual.AllocationId
	glog.V(4).Infof("found matching EIP %v", actual)
	return actual, nil
}

func (e *EIP) Run(c *fi.Context) error {
	return fi.DefaultDeltaRunMethod(e, c)
}

func (_ *EIP) CheckChanges(a, e, changes *EIP) error {
	if a == nil {
		if e.Region == nil {
			return fi.RequiredField("Region")
		}
	}
	return nil
}

func (_ *EIP) RenderALI(t *aliup.ALIAPITarget, a, e, changes *EIP) error {

	if a == nil {
		glog.V(2).Infof("Creating new EIP for NatGateway:%q", fi.StringValue(e.NatGateway.Name))

		allocateEipAddressArgs := &ecs.AllocateEipAddressArgs{
			RegionId: common.Region(t.Cloud.Region()),
		}

		eipAddress, allocationId, err := t.Cloud.EcsClient().AllocateEipAddress(allocateEipAddressArgs)
		if err != nil {
			return fmt.Errorf("error creating eip: %v", err)
		}
		e.IpAddress = fi.String(eipAddress)
		e.AllocationId = fi.String(allocationId)
		e.Available = fi.Bool(true)
	}

	if fi.BoolValue(a.Available) {
		err := t.Cloud.EcsClient().AssociateEipAddress(fi.StringValue(e.AllocationId), fi.StringValue(e.NatGateway.ID))
		if err != nil {
			return fmt.Errorf("error associating eip to natGateway: %v", err)
		}
	}

	return nil
}

type terraformEip struct {
}

type terraformEipAssociation struct {
	InstanceID   *terraform.Literal `json:"instance_id,omitempty"`
	AllocationID *terraform.Literal `json:"allocation_id,omitempty"`
}

func (_ *EIP) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *EIP) error {
	tf := &terraformEip{}
	err := t.RenderResource("alicloud_eip", *e.Name, tf)
	if err != nil {
		return err
	}

	associationtf := &terraformEipAssociation{
		InstanceID:   e.NatGateway.TerraformLink(),
		AllocationID: e.TerraformLink(),
	}

	return t.RenderResource("alicloud_eip_association", *e.Name+"_asso", associationtf)
}

func (e *EIP) TerraformLink() *terraform.Literal {
	return terraform.LiteralProperty("alicloud_eip", *e.Name, "id")
}
