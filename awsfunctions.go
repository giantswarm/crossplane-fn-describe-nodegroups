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

	"github.com/aws/aws-sdk-go-v2/aws"
	asg "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/giantswarm/xfnlib/pkg/composite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capiinfra "sigs.k8s.io/cluster-api/api/v1beta1"
)

// CreateAWSNodegroupSpec will attempt to determine how the nodegroup is defined
// and map that back into objects for cluster-api and cluster-api-provider-aws
//
// This function will output both a MachinePool and an AWSManagedMachinepool object
func (f *Function) CreateAWSNodegroupSpec(ac *XrConfig) (err error) {
	var (
		res *eks.ListNodegroupsOutput
		cfg aws.Config
	)

	if cfg, err = awsConfig(ac.region, ac.providerConfigRef); err != nil {
		err = errors.Wrap(err, "failed to load aws config for assume role")
		return
	}

	eksclient := getEksClient(cfg)
	ec2client := getEc2Client(cfg)
	asgclient := getAsgClient(cfg)

	clusterInput := &eks.ListNodegroupsInput{
		ClusterName: ac.cluster,
	}

	if res, err = GetNodegroups(context.TODO(), eksclient, clusterInput); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("failed to load nodegroups for cluster %q", *ac.cluster))
		return
	}

	for _, nodegroup := range res.Nodegroups {
		nodegroup := nodegroup
		nodegroupInput := &eks.DescribeNodegroupInput{
			ClusterName:   ac.cluster,
			NodegroupName: &nodegroup,
		}
		var group *eks.DescribeNodegroupOutput
		if group, err = DescribeNodegroup(context.TODO(), eksclient, nodegroupInput); err != nil {
			f.log.Debug("AWSAPI", "cannot describe nodegroup", nodegroup, "cluster", *ac.cluster, "error", err)
			continue
		}

		var ng *expinfrav2.AWSManagedMachinePoolSpec
		if ng, err = f.nodegroupToCapiObject(group.Nodegroup, ec2client, asgclient); err != nil {
			f.log.Debug("AWSAPI", "cannot create nodegroup", nodegroup, "cluster", *ac.cluster, "error", err)
			continue
		}

		ac.labels["giantswarm.io/machine-pool"] = nodegroup
		var nodegroupName string = fmt.Sprintf("%s-awsmanagedmachinepool-%s", *ac.cluster, nodegroup)
		f.log.Info("AWSAPI", "Creating nodegroup", nodegroupName)
		var awsmmp expinfrav2.AWSManagedMachinePool = expinfrav2.AWSManagedMachinePool{
			TypeMeta: metav1.TypeMeta{
				Kind:       "AWSManagedMachinePool",
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        nodegroupName,
				Namespace:   *ac.namespace,
				Labels:      ac.labels,
				Annotations: ac.annotations,
			},
			Spec: *ng,
			Status: expinfrav2.AWSManagedMachinePoolStatus{
				Ready:                 true,
				Replicas:              int32(len(ng.ProviderIDList)),
				LaunchTemplateID:      group.Nodegroup.LaunchTemplate.Id,
				LaunchTemplateVersion: group.Nodegroup.LaunchTemplate.Version,
			},
		}

		var dataSecretName string = ""
		var machinepoolName string = fmt.Sprintf("%s-machinepool-%s", *ac.cluster, nodegroup)
		f.log.Info("AWSAPI", "Creating machinepool", machinepoolName)
		var machinepool *capiinfra.MachineDeployment = &capiinfra.MachineDeployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "MachinePool",
				APIVersion: "cluster.x-k8s.io/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        machinepoolName,
				Namespace:   *ac.namespace,
				Labels:      ac.labels,
				Annotations: ac.annotations,
			},
			Spec: capiinfra.MachineDeploymentSpec{
				Replicas:    &awsmmp.Status.Replicas,
				ClusterName: *ac.cluster,
				Template: capiinfra.MachineTemplateSpec{
					Spec: capiinfra.MachineSpec{
						ClusterName: *ac.cluster,
						Bootstrap: capiinfra.Bootstrap{
							DataSecretName: &dataSecretName,
						},
						InfrastructureRef: v1.ObjectReference{
							Kind:       "AWSManagedMachinePool",
							APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
							Namespace:  *ac.namespace,
							Name:       nodegroupName,
						},
					},
				},
			},
		}

		var awsobject, mpobject *unstructured.Unstructured
		if awsobject, err = composite.ToUnstructuredKubernetesObject(awsmmp, ac.composite.Spec.ClusterProviderConfigRef, ac.composite.Spec.ObjectDeletionPolicy); err != nil {
			f.log.Debug("failed to convert nodegroup", nodegroupName, "cluster", *ac.cluster, "error", err, "object", awsmmp)
			continue
		}

		f.log.Info("Adding nodegroup to required resources", "nodegroup", nodegroupName)
		if err = ac.composed.AddDesired(nodegroupName, awsobject); err != nil {
			f.log.Debug("failed to add nodegroup", nodegroupName, "cluster", *ac.cluster, "error", err, "object", awsobject)
			continue
		}

		if mpobject, err = composite.ToUnstructuredKubernetesObject(machinepool, ac.composite.Spec.ClusterProviderConfigRef, ac.composite.Spec.ObjectDeletionPolicy); err != nil {
			f.log.Debug("failed to convert machinepool", machinepoolName, "cluster", *ac.cluster, "error", err, "object", machinepool)
			continue
		}

		f.log.Info("Adding machinepool to required resources", "machinepool", nodegroupName)
		if err = ac.composed.AddDesired(machinepoolName, mpobject); err != nil {
			f.log.Debug("failed to add machinepool", machinepoolName, "cluster", *ac.cluster, "error", err, "object", mpobject)
			continue
		}
	}
	return nil
}

// Pull all the information together to create a AWSManagedMachinePool object
func (f *Function) nodegroupToCapiObject(group *types.Nodegroup, ec2client AwsEc2Api, asgclient AwsAsgApi) (pool *expinfrav2.AWSManagedMachinePoolSpec, err error) {
	pool = &expinfrav2.AWSManagedMachinePoolSpec{}
	var (
		asgName           string
		asg               *asgtypes.AutoScalingGroup
		asgLaunchTemplate *expinfrav2.AWSLaunchTemplate
	)

	if group.Resources != nil {
		asgName = *group.Resources.AutoScalingGroups[0].Name
	}

	if asg, asgLaunchTemplate, err = getAutoscaling(asgName, asgclient, ec2client); err != nil {
		if asg == nil {
			return nil, err
		}
	}
	pool.AMIType = (*expinfrav2.ManagedMachineAMIType)(&group.AmiType)
	pool.AvailabilityZones = asg.AvailabilityZones

	if pool.AWSLaunchTemplate, err = getLaunchTemplate(group.LaunchTemplate, ec2client); err != nil {
		f.log.Debug("AWSAPI", "AWSLaunchTemplate error", err)
	}

	if pool.AWSLaunchTemplate == nil {
		pool.InstanceType = &group.InstanceTypes[0]
	} else {
		if pool.AWSLaunchTemplate.InstanceType == "" {
			pool.AWSLaunchTemplate.InstanceType = group.InstanceTypes[0]
		}

		f.log.Debug("Autoscaling", "AWSLaunchTemplate", pool.AWSLaunchTemplate)
		f.log.Debug("Autoscaling", "asgLaunchTemplate", asgLaunchTemplate)
		if asgLaunchTemplate != nil {
			if pool.AWSLaunchTemplate.AMI.ID == nil {
				pool.AWSLaunchTemplate.AMI.ID = asgLaunchTemplate.AMI.ID
			}

			if pool.AWSLaunchTemplate.IamInstanceProfile == "" {
				pool.AWSLaunchTemplate.IamInstanceProfile = asgLaunchTemplate.IamInstanceProfile
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

	for _, instance := range asg.Instances {
		var pid string = fmt.Sprintf("aws:///%s/%s", *instance.AvailabilityZone, *instance.InstanceId)
		pool.ProviderIDList = append(pool.ProviderIDList, pid)
	}

	if group.RemoteAccess != nil {
		pool.RemoteAccess = &expinfrav2.ManagedRemoteAccess{
			SSHKeyName:           group.RemoteAccess.Ec2SshKey,
			SourceSecurityGroups: group.RemoteAccess.SourceSecurityGroups,
		}
	}

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
		if group.UpdateConfig != nil {
			if group.UpdateConfig.MaxUnavailable != nil {
				var max int = int(*group.UpdateConfig.MaxUnavailable)
				pool.UpdateConfig.MaxUnavailable = &max
			}

			if group.UpdateConfig.MaxUnavailablePercentage != nil {
				var max int = int(*group.UpdateConfig.MaxUnavailablePercentage)
				pool.UpdateConfig.MaxUnavailablePercentage = &max
			}
		}
	}

	return
}

func getAutoscaling(name string, client AwsAsgApi, ec2client AwsEc2Api) (*asgtypes.AutoScalingGroup, *expinfrav2.AWSLaunchTemplate, error) {
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
		autoscaling       asgtypes.AutoScalingGroup = res.AutoScalingGroups[0]
		asglt             *asgtypes.LaunchTemplateSpecification
		lt                types.LaunchTemplateSpecification
		asgLaunchTemplate *expinfrav2.AWSLaunchTemplate
	)

	if autoscaling.MixedInstancesPolicy != nil && autoscaling.MixedInstancesPolicy.LaunchTemplate != nil {
		asglt = autoscaling.MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification
		var latest string = "$Latest"

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

func getLaunchTemplate(base *types.LaunchTemplateSpecification, client AwsEc2Api) (*expinfrav2.AWSLaunchTemplate, error) {
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
