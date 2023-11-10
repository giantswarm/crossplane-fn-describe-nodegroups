package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	asg "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	xfnaws "github.com/giantswarm/xfnlib/pkg/auth/aws"
	"github.com/giantswarm/xfnlib/pkg/composite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	expinfrav2 "sigs.k8s.io/cluster-api-provider-aws/v2/exp/api/v1beta2"
	capiinfra "sigs.k8s.io/cluster-api/api/v1beta1"
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

// CreateAWSNodegroupSpec will attempt to determine how the nodegroup is defined
// and map that back into objects for cluster-api and cluster-api-provider-aws
//
// This function will output both a MachinePool and an AWSManagedMachinepool object
func (f *Function) CreateAWSNodegroupSpec(ac *awsconfig) (err error) {
	var (
		res *eks.ListNodegroupsOutput
		cfg aws.Config
	)

	if cfg, err = xfnaws.Config(ac.region, ac.providerConfigRef); err != nil {
		err = errors.Wrap(err, "failed to load aws config for assume role")
		return
	}

	eksclient := eks.NewFromConfig(cfg)
	ec2client := ec2.NewFromConfig(cfg)
	asgclient := asg.NewFromConfig(cfg)

	clusterInput := &eks.ListNodegroupsInput{
		ClusterName: ac.cluster,
	}

	if res, err = GetNodegroups(context.TODO(), eksclient, clusterInput); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("failed to load nodegroups for cluster %q", *ac.cluster))
		return
	}

	for _, nodegroup := range res.Nodegroups {
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
func (f *Function) nodegroupToCapiObject(group *types.Nodegroup, ec2client *ec2.Client, asgclient *asg.Client) (pool *expinfrav2.AWSManagedMachinePoolSpec, err error) {
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
