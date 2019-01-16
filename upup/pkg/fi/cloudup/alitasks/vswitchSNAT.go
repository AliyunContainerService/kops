package alitasks

import (
	"fmt"

	"github.com/golang/glog"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/aliup"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"
)

//go:generate fitask -type=VSwitchSNAT
type VSwitchSNAT struct {
	Name      *string
	Lifecycle *fi.Lifecycle
	ID        *string

	VSwitch     *VSwitch
	NatGateway  *NatGateway
	SnatTableId *string
	// Shared is set if this is a shared VSwitch
	Shared *bool
}

var _ fi.CompareWithID = &VSwitchSNAT{}

func (v *VSwitchSNAT) CompareWithID() *string {
	return v.Name
}

func (v *VSwitchSNAT) Find(c *fi.Context) (*VSwitchSNAT, error) {
	if v.VSwitch == nil || v.VSwitch.VSwitchId == nil {
		glog.V(4).Infof("VSwitch / VSwitchId not found for %s, skipping Find", fi.StringValue(v.Name))
		return nil, nil
	}
	if v.NatGateway == nil || v.NatGateway.ID == nil {
		glog.V(4).Infof("NatGateway / NatGatewayId not found for %s, skipping Find", fi.StringValue(v.Name))
		return nil, nil
	}
	cloud := c.Cloud.(aliup.ALICloud)

	describeNatGatewaysArgs := &ecs.DescribeNatGatewaysArgs{
		RegionId:     common.Region(cloud.Region()),
		NatGatewayId: fi.StringValue(v.NatGateway.ID),
	}

	natGateways, _, err := cloud.EcsClient().DescribeNatGateways(describeNatGatewaysArgs)
	if err != nil {
		return nil, fmt.Errorf("error listing NatGateways: %v", err)
	}
	if len(natGateways) == 0 {
		return nil, nil
	}

	if len(natGateways[0].SnatTableIds.SnatTableId) != 1 {
		return nil, fmt.Errorf("found multiple SnatTables for %q", natGateways[0].NatGatewayId)
	}

	describeSnatTableEntriesArgs := &ecs.DescribeSnatTableEntriesArgs{
		RegionId:    common.Region(cloud.Region()),
		SnatTableId: natGateways[0].SnatTableIds.SnatTableId[0],
	}
	snatTableEntries, _, err := cloud.EcsClient().DescribeSnatTableEntries(describeSnatTableEntriesArgs)
	if err != nil {
		return nil, fmt.Errorf("error listing snatTableEntries: %v", err)
	}
	if len(snatTableEntries) == 0 {
		return nil, nil
	}

	actual := &VSwitchSNAT{}
	actual.VSwitch = v.VSwitch
	actual.NatGateway = &NatGateway{ID: fi.String(natGateways[0].NatGatewayId)}
	actual.SnatTableId = fi.String(natGateways[0].SnatTableIds.SnatTableId[0])
	v.SnatTableId = actual.SnatTableId

	for _, snatEntry := range snatTableEntries {
		if snatEntry.SourceVSwitchId == fi.StringValue(v.VSwitch.VSwitchId) {
			actual.ID = fi.String(snatEntry.SnatEntryId)
		}
	}

	// Prevent spurious changes
	actual.Shared = v.Shared
	actual.Name = v.Name
	actual.Lifecycle = v.Lifecycle

	return actual, nil
}

func (v *VSwitchSNAT) Run(c *fi.Context) error {
	return fi.DefaultDeltaRunMethod(v, c)
}

func (v *VSwitchSNAT) CheckChanges(a, e, changes *VSwitchSNAT) error {
	if e.VSwitch == nil {
		return fi.RequiredField("VPC")
	}

	if e.NatGateway == nil {
		return fi.RequiredField("CIDRBlock")
	}

	if a != nil && changes != nil {
		if changes.VSwitch != nil {
			return fi.CannotChangeField("VSwitch")
		}

		if changes.NatGateway != nil {
			return fi.CannotChangeField("NatGateway")
		}
	}
	return nil
}

func (_ *VSwitchSNAT) RenderALI(t *aliup.ALIAPITarget, a, e, changes *VSwitchSNAT) error {

	if a == nil {
		return fmt.Errorf("VSwitchSNAT:%q target VSwitch or target SnatGateway does not found", fi.StringValue(e.Name))
	}

	if a.ID == nil {
		createSnatEntryArgs := &ecs.CreateSnatEntryArgs{
			RegionId:        common.Region(t.Cloud.Region()),
			SnatTableId:     fi.StringValue(a.SnatTableId),
			SourceVSwitchId: fi.StringValue(a.VSwitch.VSwitchId),
		}
		resp, err := t.Cloud.EcsClient().CreateSnatEntry(createSnatEntryArgs)
		if err != nil {
			return fmt.Errorf("error creating SnatEntry: %v,%v", err, createSnatEntryArgs)
		}
		e.SnatTableId = fi.String(resp.SnatEntryId)
	}
	return nil
}

type terraformVSwitchSNAT struct {
	SnatTableId *string            `json:"snat_table_id,omitempty"`
	VSwitchId   *terraform.Literal `json:"source_vswitch_id,omitempty"`
}

func (_ *VSwitchSNAT) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *VSwitchSNAT) error {
	tf := &terraformVSwitchSNAT{
		SnatTableId: e.SnatTableId,
		VSwitchId:   e.VSwitch.TerraformLink(),
	}

	return t.RenderResource("alicloud_snat_entry", *e.Name, tf)
}
