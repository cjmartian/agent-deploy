// Package mocks provides mock implementations of AWS SDK interfaces for testing.
package mocks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// EC2Mock is a mock implementation of awsclient.EC2API for testing.
// Each method can be overridden via the corresponding Func field.
// If a Func is nil, the method returns a default success response.
type EC2Mock struct {
	// VPC operations
	CreateVpcFunc         func(ctx context.Context, params *ec2.CreateVpcInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error)
	ModifyVpcAttributeFunc func(ctx context.Context, params *ec2.ModifyVpcAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyVpcAttributeOutput, error)
	DeleteVpcFunc         func(ctx context.Context, params *ec2.DeleteVpcInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error)

	// Internet Gateway operations
	CreateInternetGatewayFunc func(ctx context.Context, params *ec2.CreateInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateInternetGatewayOutput, error)
	AttachInternetGatewayFunc func(ctx context.Context, params *ec2.AttachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.AttachInternetGatewayOutput, error)
	DetachInternetGatewayFunc func(ctx context.Context, params *ec2.DetachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DetachInternetGatewayOutput, error)
	DeleteInternetGatewayFunc func(ctx context.Context, params *ec2.DeleteInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteInternetGatewayOutput, error)

	// Availability Zone operations
	DescribeAvailabilityZonesFunc func(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)

	// Subnet operations
	CreateSubnetFunc         func(ctx context.Context, params *ec2.CreateSubnetInput, optFns ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error)
	ModifySubnetAttributeFunc func(ctx context.Context, params *ec2.ModifySubnetAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifySubnetAttributeOutput, error)
	DeleteSubnetFunc         func(ctx context.Context, params *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error)

	// Route Table operations
	CreateRouteTableFunc     func(ctx context.Context, params *ec2.CreateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteTableOutput, error)
	CreateRouteFunc          func(ctx context.Context, params *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error)
	AssociateRouteTableFunc  func(ctx context.Context, params *ec2.AssociateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.AssociateRouteTableOutput, error)
	DisassociateRouteTableFunc func(ctx context.Context, params *ec2.DisassociateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DisassociateRouteTableOutput, error)
	DescribeRouteTablesFunc  func(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
	DeleteRouteTableFunc     func(ctx context.Context, params *ec2.DeleteRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteTableOutput, error)

	// NAT Gateway operations
	CreateNatGatewayFunc    func(ctx context.Context, params *ec2.CreateNatGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateNatGatewayOutput, error)
	DeleteNatGatewayFunc    func(ctx context.Context, params *ec2.DeleteNatGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteNatGatewayOutput, error)
	DescribeNatGatewaysFunc func(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error)

	// Elastic IP operations
	AllocateAddressFunc func(ctx context.Context, params *ec2.AllocateAddressInput, optFns ...func(*ec2.Options)) (*ec2.AllocateAddressOutput, error)
	ReleaseAddressFunc  func(ctx context.Context, params *ec2.ReleaseAddressInput, optFns ...func(*ec2.Options)) (*ec2.ReleaseAddressOutput, error)

	// Security Group operations
	CreateSecurityGroupFunc          func(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngressFunc func(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	DeleteSecurityGroupFunc          func(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)

	// Tracking fields for verification
	CreateVpcCalls    int
	DeleteVpcCalls    int
	CreateSubnetCalls int
	DeleteSubnetCalls int
}

// VPC operations

func (m *EC2Mock) CreateVpc(ctx context.Context, params *ec2.CreateVpcInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error) {
	m.CreateVpcCalls++
	if m.CreateVpcFunc != nil {
		return m.CreateVpcFunc(ctx, params, optFns...)
	}
	return &ec2.CreateVpcOutput{
		Vpc: &ec2types.Vpc{
			VpcId:     aws.String("vpc-mock-12345"),
			CidrBlock: params.CidrBlock,
		},
	}, nil
}

func (m *EC2Mock) ModifyVpcAttribute(ctx context.Context, params *ec2.ModifyVpcAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyVpcAttributeOutput, error) {
	if m.ModifyVpcAttributeFunc != nil {
		return m.ModifyVpcAttributeFunc(ctx, params, optFns...)
	}
	return &ec2.ModifyVpcAttributeOutput{}, nil
}

func (m *EC2Mock) DeleteVpc(ctx context.Context, params *ec2.DeleteVpcInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error) {
	m.DeleteVpcCalls++
	if m.DeleteVpcFunc != nil {
		return m.DeleteVpcFunc(ctx, params, optFns...)
	}
	return &ec2.DeleteVpcOutput{}, nil
}

// Internet Gateway operations

func (m *EC2Mock) CreateInternetGateway(ctx context.Context, params *ec2.CreateInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateInternetGatewayOutput, error) {
	if m.CreateInternetGatewayFunc != nil {
		return m.CreateInternetGatewayFunc(ctx, params, optFns...)
	}
	return &ec2.CreateInternetGatewayOutput{
		InternetGateway: &ec2types.InternetGateway{
			InternetGatewayId: aws.String("igw-mock-12345"),
		},
	}, nil
}

func (m *EC2Mock) AttachInternetGateway(ctx context.Context, params *ec2.AttachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.AttachInternetGatewayOutput, error) {
	if m.AttachInternetGatewayFunc != nil {
		return m.AttachInternetGatewayFunc(ctx, params, optFns...)
	}
	return &ec2.AttachInternetGatewayOutput{}, nil
}

func (m *EC2Mock) DetachInternetGateway(ctx context.Context, params *ec2.DetachInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DetachInternetGatewayOutput, error) {
	if m.DetachInternetGatewayFunc != nil {
		return m.DetachInternetGatewayFunc(ctx, params, optFns...)
	}
	return &ec2.DetachInternetGatewayOutput{}, nil
}

func (m *EC2Mock) DeleteInternetGateway(ctx context.Context, params *ec2.DeleteInternetGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteInternetGatewayOutput, error) {
	if m.DeleteInternetGatewayFunc != nil {
		return m.DeleteInternetGatewayFunc(ctx, params, optFns...)
	}
	return &ec2.DeleteInternetGatewayOutput{}, nil
}

// Availability Zone operations

func (m *EC2Mock) DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	if m.DescribeAvailabilityZonesFunc != nil {
		return m.DescribeAvailabilityZonesFunc(ctx, params, optFns...)
	}
	return &ec2.DescribeAvailabilityZonesOutput{
		AvailabilityZones: []ec2types.AvailabilityZone{
			{ZoneName: aws.String("us-east-1a"), State: ec2types.AvailabilityZoneStateAvailable},
			{ZoneName: aws.String("us-east-1b"), State: ec2types.AvailabilityZoneStateAvailable},
		},
	}, nil
}

// Subnet operations

func (m *EC2Mock) CreateSubnet(ctx context.Context, params *ec2.CreateSubnetInput, optFns ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error) {
	m.CreateSubnetCalls++
	if m.CreateSubnetFunc != nil {
		return m.CreateSubnetFunc(ctx, params, optFns...)
	}
	return &ec2.CreateSubnetOutput{
		Subnet: &ec2types.Subnet{
			SubnetId:         aws.String("subnet-mock-" + string(rune('a'+m.CreateSubnetCalls-1))),
			VpcId:            params.VpcId,
			CidrBlock:        params.CidrBlock,
			AvailabilityZone: params.AvailabilityZone,
		},
	}, nil
}

func (m *EC2Mock) ModifySubnetAttribute(ctx context.Context, params *ec2.ModifySubnetAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifySubnetAttributeOutput, error) {
	if m.ModifySubnetAttributeFunc != nil {
		return m.ModifySubnetAttributeFunc(ctx, params, optFns...)
	}
	return &ec2.ModifySubnetAttributeOutput{}, nil
}

func (m *EC2Mock) DeleteSubnet(ctx context.Context, params *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error) {
	m.DeleteSubnetCalls++
	if m.DeleteSubnetFunc != nil {
		return m.DeleteSubnetFunc(ctx, params, optFns...)
	}
	return &ec2.DeleteSubnetOutput{}, nil
}

// Route Table operations

func (m *EC2Mock) CreateRouteTable(ctx context.Context, params *ec2.CreateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteTableOutput, error) {
	if m.CreateRouteTableFunc != nil {
		return m.CreateRouteTableFunc(ctx, params, optFns...)
	}
	return &ec2.CreateRouteTableOutput{
		RouteTable: &ec2types.RouteTable{
			RouteTableId: aws.String("rtb-mock-12345"),
			VpcId:        params.VpcId,
		},
	}, nil
}

func (m *EC2Mock) CreateRoute(ctx context.Context, params *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error) {
	if m.CreateRouteFunc != nil {
		return m.CreateRouteFunc(ctx, params, optFns...)
	}
	return &ec2.CreateRouteOutput{Return: aws.Bool(true)}, nil
}

func (m *EC2Mock) AssociateRouteTable(ctx context.Context, params *ec2.AssociateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.AssociateRouteTableOutput, error) {
	if m.AssociateRouteTableFunc != nil {
		return m.AssociateRouteTableFunc(ctx, params, optFns...)
	}
	return &ec2.AssociateRouteTableOutput{
		AssociationId: aws.String("rtbassoc-mock-12345"),
	}, nil
}

func (m *EC2Mock) DisassociateRouteTable(ctx context.Context, params *ec2.DisassociateRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DisassociateRouteTableOutput, error) {
	if m.DisassociateRouteTableFunc != nil {
		return m.DisassociateRouteTableFunc(ctx, params, optFns...)
	}
	return &ec2.DisassociateRouteTableOutput{}, nil
}

func (m *EC2Mock) DescribeRouteTables(ctx context.Context, params *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	if m.DescribeRouteTablesFunc != nil {
		return m.DescribeRouteTablesFunc(ctx, params, optFns...)
	}
	return &ec2.DescribeRouteTablesOutput{
		RouteTables: []ec2types.RouteTable{},
	}, nil
}

func (m *EC2Mock) DeleteRouteTable(ctx context.Context, params *ec2.DeleteRouteTableInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteTableOutput, error) {
	if m.DeleteRouteTableFunc != nil {
		return m.DeleteRouteTableFunc(ctx, params, optFns...)
	}
	return &ec2.DeleteRouteTableOutput{}, nil
}

// NAT Gateway operations

func (m *EC2Mock) CreateNatGateway(ctx context.Context, params *ec2.CreateNatGatewayInput, optFns ...func(*ec2.Options)) (*ec2.CreateNatGatewayOutput, error) {
	if m.CreateNatGatewayFunc != nil {
		return m.CreateNatGatewayFunc(ctx, params, optFns...)
	}
	return &ec2.CreateNatGatewayOutput{
		NatGateway: &ec2types.NatGateway{
			NatGatewayId: aws.String("nat-mock-12345"),
			State:        ec2types.NatGatewayStateAvailable,
		},
	}, nil
}

func (m *EC2Mock) DeleteNatGateway(ctx context.Context, params *ec2.DeleteNatGatewayInput, optFns ...func(*ec2.Options)) (*ec2.DeleteNatGatewayOutput, error) {
	if m.DeleteNatGatewayFunc != nil {
		return m.DeleteNatGatewayFunc(ctx, params, optFns...)
	}
	return &ec2.DeleteNatGatewayOutput{
		NatGatewayId: params.NatGatewayId,
	}, nil
}

func (m *EC2Mock) DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	if m.DescribeNatGatewaysFunc != nil {
		return m.DescribeNatGatewaysFunc(ctx, params, optFns...)
	}
	// Default: return NAT gateway in available state
	return &ec2.DescribeNatGatewaysOutput{
		NatGateways: []ec2types.NatGateway{{
			NatGatewayId: aws.String("nat-mock-12345"),
			State:        ec2types.NatGatewayStateAvailable,
		}},
	}, nil
}

// Elastic IP operations

func (m *EC2Mock) AllocateAddress(ctx context.Context, params *ec2.AllocateAddressInput, optFns ...func(*ec2.Options)) (*ec2.AllocateAddressOutput, error) {
	if m.AllocateAddressFunc != nil {
		return m.AllocateAddressFunc(ctx, params, optFns...)
	}
	return &ec2.AllocateAddressOutput{
		AllocationId: aws.String("eipalloc-mock-12345"),
		PublicIp:     aws.String("1.2.3.4"),
	}, nil
}

func (m *EC2Mock) ReleaseAddress(ctx context.Context, params *ec2.ReleaseAddressInput, optFns ...func(*ec2.Options)) (*ec2.ReleaseAddressOutput, error) {
	if m.ReleaseAddressFunc != nil {
		return m.ReleaseAddressFunc(ctx, params, optFns...)
	}
	return &ec2.ReleaseAddressOutput{}, nil
}

// Security Group operations

func (m *EC2Mock) CreateSecurityGroup(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	if m.CreateSecurityGroupFunc != nil {
		return m.CreateSecurityGroupFunc(ctx, params, optFns...)
	}
	return &ec2.CreateSecurityGroupOutput{
		GroupId: aws.String("sg-mock-12345"),
	}, nil
}

func (m *EC2Mock) AuthorizeSecurityGroupIngress(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	if m.AuthorizeSecurityGroupIngressFunc != nil {
		return m.AuthorizeSecurityGroupIngressFunc(ctx, params, optFns...)
	}
	return &ec2.AuthorizeSecurityGroupIngressOutput{Return: aws.Bool(true)}, nil
}

func (m *EC2Mock) DeleteSecurityGroup(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
	if m.DeleteSecurityGroupFunc != nil {
		return m.DeleteSecurityGroupFunc(ctx, params, optFns...)
	}
	return &ec2.DeleteSecurityGroupOutput{}, nil
}
