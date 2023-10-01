package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	asg "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	xfnaws "github.com/giantswarm/xfnlib/pkg/auth/aws"
	"github.com/giantswarm/xfnlib/pkg/composite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	infrav2 "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	expinfrav2 "sigs.k8s.io/cluster-api-provider-aws/v2/exp/api/v1beta2"
)

// EKSNodegroupAPI describes the AWS functions required by this composition function
// in order to track nodegroup objects for the desired cluster
type EKSNodegroupsAPI interface {
	ListNodegroups(ctx context.Context,
		params *eks.ListNodegroupsInput,
		optFns ...func(*eks.Options)) (*eks.ListNodegroupsOutput, error)

	DescribeNodegroup(ctx context.Context,
		params *eks.DescribeNodegroupInput,
		optFns ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error)
}

// Get the nodegroups attached to the provided cluster
func GetNodegroups(c context.Context, api EKSNodegroupsAPI, input *eks.ListNodegroupsInput) (*eks.ListNodegroupsOutput, error) {
	return api.ListNodegroups(c, input)

}

// Describe a single nodegroup
func DescribeNodegroup(c context.Context, api EKSNodegroupsAPI, input *eks.DescribeNodegroupInput) (*eks.DescribeNodegroupOutput, error) {
	return api.DescribeNodegroup(c, input)
}

// EC2API Describes the functions required to access data on the AWS EC2 api
type EC2API interface {
	DescribeLaunchTemplateVersions(ctx context.Context,
		params *ec2.DescribeLaunchTemplateVersionsInput,
		optFns ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplateVersionsOutput, error)
}

// Get the EC2 Launch template versions for a given launch template
func GetLaunchTemplate(c context.Context, api EC2API, input *ec2.DescribeLaunchTemplateVersionsInput) (*ec2.DescribeLaunchTemplateVersionsOutput, error) {
	return api.DescribeLaunchTemplateVersions(c, input)
}

// AutoscalingAPI presents functions required for reading autoscaling groups from AWS
type AutoscalingAPI interface {
	DescribeAutoScalingGroups(ctx context.Context,
		params *asg.DescribeAutoScalingGroupsInput,
		optFns ...func(*asg.Options)) (*asg.DescribeAutoScalingGroupsOutput, error)
}

// Describe the autoscaling group(s)
func GetAutoScalingGroups(c context.Context, api AutoscalingAPI, input *asg.DescribeAutoScalingGroupsInput) (*asg.DescribeAutoScalingGroupsOutput, error) {
	return api.DescribeAutoScalingGroups(c, input)
}

func (f *Function) CreateAWSNodegroupSpec(cluster, namespace, region, providerConfig *string, labels, annotations map[string]string) (err error) {
	var (
		res *eks.ListNodegroupsOutput
		cfg aws.Config
	)

	// Set up the assume role clients
	if cfg, err = xfnaws.Config(region, providerConfig); err != nil {
		err = errors.Wrap(err, "failed to load aws config for assume role")
		return
	}

	eksclient := eks.NewFromConfig(cfg)
	ec2client := ec2.NewFromConfig(cfg)
	asgclient := asg.NewFromConfig(cfg)
	// end setting up clients

	clusterInput := &eks.ListNodegroupsInput{
		ClusterName: cluster,
	}

	if res, err = GetNodegroups(context.TODO(), eksclient, clusterInput); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("failed to load nodegroups for cluster %q", *cluster))
		return
	}

	for _, nodegroup := range res.Nodegroups {
		nodegroupInput := &eks.DescribeNodegroupInput{
			ClusterName:   cluster,
			NodegroupName: &nodegroup,
		}
		var group *eks.DescribeNodegroupOutput
		if group, err = DescribeNodegroup(context.TODO(), eksclient, nodegroupInput); err != nil {
			f.log.Debug(fmt.Sprintf("cannot describe nodegroup %s for cluster %s", nodegroup, *cluster), "error was", err)
			continue
		}

		var ng *expinfrav2.AWSManagedMachinePoolSpec
		if ng, err = f.nodegroupToCapiObject(group.Nodegroup, ec2client, asgclient); err != nil {
			f.log.Debug(fmt.Sprintf("cannot create nodegroup object for nodegroup %q in cluster %q", nodegroup, *cluster), "error was", err)
			continue
		}

		var nodegroupName string = fmt.Sprintf("%s-%s", *cluster, nodegroup)
		var awsmmp *expinfrav2.AWSManagedMachinePool = &expinfrav2.AWSManagedMachinePool{
			TypeMeta: metav1.TypeMeta{
				Kind:       "AWSManagedMachinePool",
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        nodegroupName,
				Namespace:   *namespace,
				Labels:      labels,
				Annotations: annotations,
			},
			Spec: *ng,
			Status: expinfrav2.AWSManagedMachinePoolStatus{
				Ready:                 true,
				Replicas:              int32(len(ng.ProviderIDList)),
				LaunchTemplateID:      group.Nodegroup.LaunchTemplate.Id,
				LaunchTemplateVersion: group.Nodegroup.LaunchTemplate.Version,
			},
		}

		var object *unstructured.Unstructured
		if object, err = composite.ToUnstructuredKubernetesObject(awsmmp, f.composite.Spec.KubernetesProviderConfigRef); err != nil {
			f.log.Debug(fmt.Sprintf("failed to convert nodegroup %q to kubernetes object for cluster %q.", nodegroup, *cluster), "error was", err)
			continue
		}

		if err = f.composed.AddDesired(nodegroupName, object); err != nil {
			f.log.Info(composedName, "add machinepool", errors.Wrap(err, "cannot add composed object "+nodegroupName))
			continue
		}
	}
	return nil
}

// Pull all the information together to create a AWSManagedMachinePool object
func (f *Function) nodegroupToCapiObject(group *types.Nodegroup, ec2client *ec2.Client, asgclient *asg.Client) (pool *expinfrav2.AWSManagedMachinePoolSpec, err error) {
	pool = &expinfrav2.AWSManagedMachinePoolSpec{}
	pool.AdditionalTags = make(map[string]string)
	for k, v := range group.Tags {
		if !strings.HasPrefix(k, "aws:") {
			pool.AdditionalTags[k] = v
		}
	}
	pool.AMIType = (*expinfrav2.ManagedMachineAMIType)(&group.AmiType)

	var (
		name          string
		autoscaling   *asgtypes.AutoScalingGroup
		autoscalinglt *expinfrav2.AWSLaunchTemplate
	)
	if group.Resources != nil {
		name = *group.Resources.AutoScalingGroups[0].Name
	}

	if autoscaling, autoscalinglt, err = getAutoscaling(name, asgclient, ec2client); err != nil {
		if autoscaling == nil {
			return nil, err
		}
	}

	pool.AMIVersion = group.Version

	pool.AvailabilityZones = autoscaling.AvailabilityZones

	if group.LaunchTemplate == nil {
		pool.InstanceType = &group.InstanceTypes[0]
	} else {
		if pool.AWSLaunchTemplate, err = getLaunchTemplate(group.LaunchTemplate, ec2client); err != nil {
			return pool, err
		}
		if pool.AWSLaunchTemplate.InstanceType == "" {
			pool.AWSLaunchTemplate.InstanceType = group.InstanceTypes[0]
		}

		f.log.Debug("Autoscaling", "LaunchTemplate", pool.AWSLaunchTemplate)
		f.log.Debug("Autoscaling", "autoscalinglt", autoscalinglt)
		if autoscalinglt != nil {
			if pool.AWSLaunchTemplate.AMI.ID == nil {
				pool.AWSLaunchTemplate.AMI.ID = autoscalinglt.AMI.ID
			}

			if pool.AWSLaunchTemplate.IamInstanceProfile == "" {
				pool.AWSLaunchTemplate.IamInstanceProfile = autoscalinglt.IamInstanceProfile
			}
		}
	}

	var capacityTypes map[types.CapacityTypes]expinfrav2.ManagedMachinePoolCapacityType = map[types.CapacityTypes]expinfrav2.ManagedMachinePoolCapacityType{
		types.CapacityTypesOnDemand: expinfrav2.ManagedMachinePoolCapacityTypeOnDemand,
		types.CapacityTypesSpot:     expinfrav2.ManagedMachinePoolCapacityTypeOnDemand,
	}
	ct := capacityTypes[group.CapacityType]

	pool.CapacityType = &ct
	pool.DiskSize = group.DiskSize
	pool.EKSNodegroupName = *group.NodegroupName
	pool.Labels = group.Labels

	for _, instance := range autoscaling.Instances {
		var pid string = fmt.Sprintf("aws:///%s/%s", *instance.AvailabilityZone, *instance.InstanceId)
		pool.ProviderIDList = append(pool.ProviderIDList, pid)
	}

	if group.RemoteAccess != nil {
		pool.RemoteAccess = &expinfrav2.ManagedRemoteAccess{
			SSHKeyName:           group.RemoteAccess.Ec2SshKey,
			SourceSecurityGroups: group.RemoteAccess.SourceSecurityGroups,
		}
	}

	// RoleAdditionalPolicies???

	pool.RoleName = strings.Split(*group.NodeRole, "/")[1]

	if group.ScalingConfig != nil {
		pool.Scaling = &expinfrav2.ManagedMachinePoolScaling{
			MinSize: group.ScalingConfig.MinSize,
			MaxSize: group.ScalingConfig.MaxSize,
		}
	}

	pool.SubnetIDs = group.Subnets
	for _, taint := range group.Taints {
		t := expinfrav2.Taint{
			Effect: expinfrav2.TaintEffect(taint.Effect),
			Key:    *taint.Key,
			Value:  *taint.Value,
		}
		pool.Taints = append(pool.Taints, t)
	}

	pool.UpdateConfig = &expinfrav2.UpdateConfig{}
	{
		if group.UpdateConfig.MaxUnavailable != nil {
			var max int = int(*group.UpdateConfig.MaxUnavailable)
			pool.UpdateConfig.MaxUnavailable = &max
		}

		if group.UpdateConfig.MaxUnavailablePercentage != nil {
			var max int = int(*group.UpdateConfig.MaxUnavailablePercentage)
			pool.UpdateConfig.MaxUnavailablePercentage = &max
		}
	}

	return
}

func getAutoscaling(name string, client *asg.Client, ec2client *ec2.Client) (*asgtypes.AutoScalingGroup, *expinfrav2.AWSLaunchTemplate, error) {
	var (
		res *asg.DescribeAutoScalingGroupsOutput
		err error
	)
	input := asg.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{
			name,
		},
	}

	if res, err = GetAutoScalingGroups(context.TODO(), client, &input); err != nil {
		return nil, nil, err
	}

	var (
		// I think we only need the first here
		autoscaling       asgtypes.AutoScalingGroup = res.AutoScalingGroups[0]
		asglt             *asgtypes.LaunchTemplateSpecification
		lt                types.LaunchTemplateSpecification
		asgLaunchTemplate *expinfrav2.AWSLaunchTemplate
	)

	if autoscaling.MixedInstancesPolicy != nil && autoscaling.MixedInstancesPolicy.LaunchTemplate != nil {
		asglt = autoscaling.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification
		var latest string = "$Default"

		if asglt.Version == nil {
			asglt.Version = &latest
		}
		lt = types.LaunchTemplateSpecification{
			Id:      asglt.LaunchTemplateId,
			Name:    asglt.LaunchTemplateName,
			Version: asglt.Version,
		}
		asgLaunchTemplate, err = getLaunchTemplate(&lt, ec2client)
	}

	return &autoscaling, asgLaunchTemplate, err
}

func getLaunchTemplate(base *types.LaunchTemplateSpecification, client *ec2.Client) (*expinfrav2.AWSLaunchTemplate, error) {
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

	// OK... Which launch template should be used here?
	// Both the nodegroup and the autoscaling group have a launch template
	// and both seem to be different
	if res, err = GetLaunchTemplate(context.TODO(), client, &input); err != nil {
		return nil, err
	}

	if len(res.LaunchTemplateVersions) != 1 {
		return nil, fmt.Errorf("wrong count for launch templates for template %s", *base.Name)
	}

	template.Name = *res.LaunchTemplateVersions[0].LaunchTemplateName
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

	for _, id := range data.SecurityGroupIds {
		id := id
		template.AdditionalSecurityGroups = append(template.AdditionalSecurityGroups, infrav2.AWSResourceReference{
			ID: &id,
		})
	}

	return &template, nil
}
