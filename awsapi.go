package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	asg "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	xfnaws "github.com/giantswarm/xfnlib/pkg/auth/aws"
)

// EC2API Describes the functions required to access data on the AWS EC2 api
type AwsEc2Api interface {
	DescribeLaunchTemplates(ctx context.Context,
		params *ec2.DescribeLaunchTemplatesInput,
		optFns ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplatesOutput, error)

	DescribeLaunchTemplateVersions(ctx context.Context,
		params *ec2.DescribeLaunchTemplateVersionsInput,
		optFns ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplateVersionsOutput, error)
}

// DescribeLaunchTemplateVersions Get the EC2 Launch template versions for a given launch template
func DescribeLaunchTemplateVersions(c context.Context, api AwsEc2Api, input *ec2.DescribeLaunchTemplateVersionsInput) (*ec2.DescribeLaunchTemplateVersionsOutput, error) {
	return api.DescribeLaunchTemplateVersions(c, input)
}

// DescribeLaunchTemplates Find launch templates for a given nodegroup
func DescribeLaunchTemplates(c context.Context, api AwsEc2Api, input *ec2.DescribeLaunchTemplatesInput) (*ec2.DescribeLaunchTemplatesOutput, error) {
	return api.DescribeLaunchTemplates(c, input)
}

// EKSNodegroupAPI describes the AWS functions required by this composition function
// in order to track nodegroup objects for the desired cluster
type AwsEksApi interface {
	ListNodegroups(ctx context.Context,
		params *eks.ListNodegroupsInput,
		optFns ...func(*eks.Options)) (*eks.ListNodegroupsOutput, error)

	DescribeNodegroup(ctx context.Context,
		params *eks.DescribeNodegroupInput,
		optFns ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error)
}

// GetNodegroups Get the nodegroups attached to the provided cluster
func GetNodegroups(c context.Context, api AwsEksApi, input *eks.ListNodegroupsInput) (*eks.ListNodegroupsOutput, error) {
	return api.ListNodegroups(c, input)

}

// DescribeNodegroup Describe a single nodegroup
func DescribeNodegroup(c context.Context, api AwsEksApi, input *eks.DescribeNodegroupInput) (*eks.DescribeNodegroupOutput, error) {
	return api.DescribeNodegroup(c, input)
}

// AutoscalingAPI presents functions required for reading autoscaling groups from AWS
type AwsAsgApi interface {
	DescribeAutoScalingGroups(ctx context.Context,
		params *asg.DescribeAutoScalingGroupsInput,
		optFns ...func(*asg.Options)) (*asg.DescribeAutoScalingGroupsOutput, error)
}

// GetAutoScalingGroups Get the autoscaling group(s) for a given nodegroup
func GetAutoScalingGroups(c context.Context, api AwsAsgApi, input *asg.DescribeAutoScalingGroupsInput) (*asg.DescribeAutoScalingGroupsOutput, error) {
	return api.DescribeAutoScalingGroups(c, input)
}

var (
	getEc2Client = func(cfg aws.Config) AwsEc2Api {
		return ec2.NewFromConfig(cfg)
	}

	getEksClient = func(cfg aws.Config) AwsEksApi {
		return eks.NewFromConfig(cfg)
	}

	getAsgClient = func(cfg aws.Config) AwsAsgApi {
		return asg.NewFromConfig(cfg)
	}

	awsConfig = func(region, provider *string) (aws.Config, error) {
		return xfnaws.Config(region, provider)
	}
)
