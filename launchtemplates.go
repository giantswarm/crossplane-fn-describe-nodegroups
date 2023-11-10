package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	infrav2 "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	expinfrav2 "sigs.k8s.io/cluster-api-provider-aws/v2/exp/api/v1beta2"
)

// EC2API Describes the functions required to access data on the AWS EC2 api
type EC2API interface {
	DescribeLaunchTemplates(ctx context.Context,
		params *ec2.DescribeLaunchTemplatesInput,
		optFns ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplatesOutput, error)

	DescribeLaunchTemplateVersions(ctx context.Context,
		params *ec2.DescribeLaunchTemplateVersionsInput,
		optFns ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplateVersionsOutput, error)
}

// Get the EC2 Launch template versions for a given launch template
func DescribeLaunchTemplateVersions(c context.Context, api EC2API, input *ec2.DescribeLaunchTemplateVersionsInput) (*ec2.DescribeLaunchTemplateVersionsOutput, error) {
	return api.DescribeLaunchTemplateVersions(c, input)
}

func DescribeLaunchTemplates(c context.Context, api EC2API, input *ec2.DescribeLaunchTemplatesInput) (*ec2.DescribeLaunchTemplatesOutput, error) {
	return api.DescribeLaunchTemplates(c, input)
}

func getLaunchTemplate(base *types.LaunchTemplateSpecification, client *ec2.Client) (*expinfrav2.AWSLaunchTemplate, error) {
	if base == nil {
		// NOOP here
		return nil, nil
	}

	var (
		res      *ec2.DescribeLaunchTemplateVersionsOutput
		template expinfrav2.AWSLaunchTemplate
		err      error
	)

	input := ec2.DescribeLaunchTemplateVersionsInput{
		LaunchTemplateId: base.Id,
		Versions: []string{
			*base.Version,
		},
	}

	if res, err = DescribeLaunchTemplateVersions(context.TODO(), client, &input); err != nil {
		return nil, err
	}

	if len(res.LaunchTemplateVersions) != 1 {
		return nil, fmt.Errorf("wrong count for launch templates for template %s", *base.Name)
	}

	//template.Name = *res.LaunchTemplateVersions[0].LaunchTemplateName
	template.Name = *base.Name
	template.VersionNumber = res.LaunchTemplateVersions[0].VersionNumber

	var data *ec2types.ResponseLaunchTemplateData = res.LaunchTemplateVersions[0].LaunchTemplateData
	template.InstanceType = string(data.InstanceType)
	template.SSHKeyName = data.KeyName

	template.AMI = infrav2.AMIReference{
		ID: data.ImageId,
	}

	if data.IamInstanceProfile != nil {
		if data.IamInstanceProfile.Name != nil && !strings.HasPrefix(*data.IamInstanceProfile.Name, "eks-") {
			template.IamInstanceProfile = *data.IamInstanceProfile.Name
		} else if data.IamInstanceProfile.Arn != nil {
			template.IamInstanceProfile = *data.IamInstanceProfile.Arn
		}
	}

	if data.InstanceMarketOptions != nil && data.InstanceMarketOptions.SpotOptions != nil {
		template.SpotMarketOptions = &infrav2.SpotMarketOptions{
			MaxPrice: data.InstanceMarketOptions.SpotOptions.MaxPrice,
		}
	}

	if len(data.BlockDeviceMappings) > 0 {
		var (
			device     ec2types.LaunchTemplateBlockDeviceMapping = data.BlockDeviceMappings[0]
			throughput int64                                     = int64(*device.Ebs.Throughput)
		)
		template.RootVolume = &infrav2.Volume{
			DeviceName: *device.DeviceName,
			Encrypted:  device.Ebs.Encrypted,
			IOPS:       int64(*device.Ebs.Iops),
			Size:       int64(*device.Ebs.VolumeSize),
			Throughput: &throughput,
			Type:       infrav2.VolumeType(device.Ebs.VolumeType),
		}
	}

	// This is necessary to ensure duplicate security groups are not
	// added into the list as there is no sanitation on the AWS launch
	// template to prevent it.
	// AWS will simply store whatever you provide.
	var sgs []string = make([]string, 0)
	for _, id := range data.SecurityGroupIds {
		var added bool = false
		for _, v := range sgs {
			if id == v {
				added = true
			}
		}
		if !added {
			sgs = append(sgs, id)
		}
	}

	template.AdditionalSecurityGroups = make([]infrav2.AWSResourceReference, 0, len(sgs))
	for i := range sgs {
		template.AdditionalSecurityGroups = append(template.AdditionalSecurityGroups, infrav2.AWSResourceReference{
			ID: &sgs[i],
		})
	}

	return &template, nil
}
