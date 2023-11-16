package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	asg "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	asgtypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	"github.com/giantswarm/crossplane-fn-describe-nodegroups/pkg/input/v1beta1"

	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
)

var (
	xrExample = `{"apiVersion": "example.org/v1","kind": "XR", "spec": {
	"clusterName": "example","clusterProviderConfigRef": "thingy",
	"regionOrLocation": "placey", "deletionPolicy": "Delete",
	"objectDeletionPolicy": "Delete", "claimRef":{"namespace":"default"},
	"labels": {"test": "label","anothertest": "label"
	},"kubernetesAdditionalLabels": {"foo": "bar"},
	"compositionSelector": {"matchLabels": {"provider": "aws"}}}}`

	xrTest = `{"apiVersion": "example.org/v1","kind": "XR", "spec": {
	"clusterName": "test","clusterProviderConfigRef": "thingy",
	"regionOrLocation": "placey", "deletionPolicy": "Delete",
	"objectDeletionPolicy": "Delete", "claimRef":{"namespace":"default"},
	"labels": {"test": "label","anothertest": "label"
	},"kubernetesAdditionalLabels": {"foo": "bar"},
	"compositionSelector": {"matchLabels": {"provider": "aws"}}}}`

	clusterExample = `{"apiVersion": "eks.aws.upbound.io/v1beta1","kind":"Cluster",
	"metadata": {"annotations": {"crossplane.io/external-name": "example",
	"crossplane.io/composition-resource-name": "eks-cluster"}, "labels": {
	"crossplane.io/claim-name": "example"}}, "managementPolicies": ["Observe"],
	"spec": {"forProvider": {"region": "eu-central-1"},"providerConfigRef": {
	"name": "example"},"writeConnectionSecretToRef": {"namespace": "example"}},
	"status": {"atProvider": {"vpcConfig": [{"vpcId": "vpc-12345678",
	"subnetIds": ["subnet-123456"]}]}}}`

	clusterTest = `{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"Cluster",
	"metadata":{"annotations":{"crossplane.io/external-name":"test",
	"crossplane.io/composition-resource-name":"eks-cluster"},
	"labels":{"crossplane.io/claim-name":"test"}},"managementPolicies":["Observe"],
	"spec":{"forProvider":{"region":"eu-central-1"},"providerConfigRef":{
	"name":"example"},"writeConnectionSecretToRef":{"namespace":"example"}},
	"status":{"atProvider":{"vpcConfig":[{"vpcId":"vpc-12345678",
	"subnetIds":["subnet-123456"]}]}}}`

	nodepoolExample = `{"apiVersion":"kubernetes.crossplane.io/v1alpha1",
	"kind":"Object","metadata":{"labels":{"foo":"bar",
	"giantswarm.io/cluster":"example","giantswarm.io/machine-pool":"ng-12345",
	"cluster.x-k8s.io/cluster-name":"example"},
	"name":"example-awsmanagedmachinepool-ng-12345"},"spec":{
	"deletionPolicy":"Delete","forProvider":{"manifest":{
	"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta2",
	"kind":"AWSManagedMachinePool","metadata":{"labels":{"foo":"bar",
	"cluster.x-k8s.io/cluster-name":"example","giantswarm.io/cluster":"example",
	"giantswarm.io/machine-pool":"ng-12345"},
	"name":"example-awsmanagedmachinepool-ng-12345","namespace":"default",
	"creationTimestamp":null},"spec":{"amiType":"AL2_x86_64","availabilityZones":[
	"eu-central-1a","eu-central-1c","eu-central-1b"],"awsLaunchTemplate":{
	"additionalSecurityGroups":[{"id":"sg-11111111111111111"},{
	"id":"sg-22222222222222222"}],"ami":{"id":"ami-0ab553a58389ae35a"},
	"instanceType":"m5.large","name":"eksctl-example-nodegroup-ng-1",
	"rootVolume":{"deviceName":"/dev/xvda","iops":3000,"size":80,
	"throughput":125,"type":"gp3"},"spotMarketOptions":{"maxPrice":"expensive"},
	"sshKeyName":"test-key","versionNumber":1},"capacityType":"onDemand",
	"eksNodegroupName":"ng-12345","providerIDList":["aws:///eu-central-1c/i-1111111111111111",
	"aws:///eu-central-1a/i-2222222222222222","aws:///eu-central-1b/i-3333333333333333"],
	"roleName":"eksctl-example-nodegroup-NodeInstanceRole-123456789123","scaling":{"maxSize":3,
	"minSize":1},"subnetIDs":["subnet-1111111111111111","subnet-2222222222222222",
	"subnet-3333333333333333"],"updateConfig":{"maxUnavailable":1}},
	"status":{"launchTemplateID":"lt-123456","launchTemplateVersion":"2","ready":true,
	"replicas":3}}},"providerConfigRef":{"name":"thingy"},"writeConnectionSecretToRef":{
	"name":"example-awsmanagedmachinepool-ng-12345","namespace":"default"}}}`

	nodepoolTest = `{"apiVersion":"kubernetes.crossplane.io/v1alpha1","kind":"Object",
	"metadata":{"labels":{"foo":"bar","giantswarm.io/cluster":"test",
	"giantswarm.io/machine-pool":"ng-23456","cluster.x-k8s.io/cluster-name":"test"},
	"name":"test-awsmanagedmachinepool-ng-23456"},"spec":{"deletionPolicy":"Delete",
	"forProvider":{"manifest":{"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta2",
	"kind":"AWSManagedMachinePool","metadata":{"labels":{"foo":"bar",
	"cluster.x-k8s.io/cluster-name":"test","giantswarm.io/cluster":"test",
	"giantswarm.io/machine-pool":"ng-23456"},"name":"test-awsmanagedmachinepool-ng-23456",
	"namespace":"default","creationTimestamp":null},"spec":{"amiType":"AL2_x86_64",
	"availabilityZones":["eu-central-1a","eu-central-1c","eu-central-1b"],
	"awsLaunchTemplate":{"additionalSecurityGroups":[{"id":"sg-11111111111111111"},
	{"id":"sg-22222222222222222"}],"ami":{"id":"ami-0ab553a58389ae35a"},
	"instanceType":"m5.large","iamInstanceProfile": "arn::123456789:/role/something",
	"name":"eksctl-test-nodegroup-ng-1","rootVolume":{
	"deviceName":"/dev/xvda","iops":3000,"size":80,"throughput":125,"type":"gp3"},
	"spotMarketOptions":{"maxPrice":"expensive"},"sshKeyName":"test-key",
	"versionNumber":1},"capacityType":"onDemand","eksNodegroupName":"ng-23456",
	"providerIDList":["aws:///eu-central-1c/i-1111111111111111",
	"aws:///eu-central-1a/i-2222222222222222","aws:///eu-central-1b/i-3333333333333333"],
	"roleName":"eksctl-test-nodegroup-NodeInstanceRole-123456789123","scaling":{
	"maxSize":3,"minSize":1},"subnetIDs":["subnet-1111111111111111",
	"subnet-2222222222222222","subnet-3333333333333333"],"updateConfig":{
	"maxUnavailable":1}},"status":{"launchTemplateID":"lt-234567",
	"launchTemplateVersion":"2","ready":true,"replicas":3}}},"providerConfigRef":{
	"name":"thingy"},"writeConnectionSecretToRef":{
	"name":"test-awsmanagedmachinepool-ng-23456","namespace":"default"}}}`

	machinepoolExample = `{"apiVersion":"kubernetes.crossplane.io/v1alpha1",
	"kind":"Object","metadata":{"labels":{"foo":"bar",
	"giantswarm.io/cluster":"example","giantswarm.io/machine-pool":"ng-12345",
	"cluster.x-k8s.io/cluster-name":"example"},
	"name":"example-machinepool-ng-12345"},"spec":{"deletionPolicy":"Delete",
	"forProvider":{"manifest":{"apiVersion":"cluster.x-k8s.io/v1beta1",
	"kind":"MachinePool","metadata":{"labels":{"foo":"bar",
	"giantswarm.io/cluster":"example","giantswarm.io/machine-pool":"ng-12345",
	"cluster.x-k8s.io/cluster-name":"example"},"name":"example-machinepool-ng-12345",
	"namespace":"default","creationTimestamp":null},"spec":{"clusterName":"example",
	"replicas":3,"selector":{},"template":{"metadata":{},"spec":{"bootstrap":{
	"dataSecretName":""},"clusterName":"example","infrastructureRef":{
	"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta2",
	"kind":"AWSManagedMachinePool","name":"example-awsmanagedmachinepool-ng-12345",
	"namespace":"default"}}}},"status":{"availableReplicas":0,"readyReplicas":0,
	"replicas":0,"unavailableReplicas":0,"updatedReplicas":0}}},
	"providerConfigRef":{"name":"thingy"},"writeConnectionSecretToRef":{
	"name":"example-machinepool-ng-12345","namespace":"default"}}}`

	machinepoolTest = `{"apiVersion":"kubernetes.crossplane.io/v1alpha1","kind":"Object",
	"metadata":{"labels":{"foo":"bar","giantswarm.io/cluster":"test",
	"giantswarm.io/machine-pool":"ng-23456","cluster.x-k8s.io/cluster-name":"test"},
	"name":"test-machinepool-ng-23456"},"spec":{"deletionPolicy":"Delete",
	"forProvider":{"manifest":{"apiVersion":"cluster.x-k8s.io/v1beta1",
	"kind":"MachinePool","metadata":{"labels":{"foo":"bar",
	"giantswarm.io/cluster":"test","giantswarm.io/machine-pool":"ng-23456",
	"cluster.x-k8s.io/cluster-name":"test"},"name":"test-machinepool-ng-23456",
	"namespace":"default","creationTimestamp":null},"spec":{"clusterName":"test",
	"replicas":3,"selector":{},"template":{"metadata":{},"spec":{
	"bootstrap":{"dataSecretName":""},"clusterName":"test",
	"infrastructureRef":{"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta2",
	"kind":"AWSManagedMachinePool","name":"test-awsmanagedmachinepool-ng-23456",
	"namespace":"default"}}}},"status":{"availableReplicas":0,"readyReplicas":0,
	"replicas":0,"unavailableReplicas":0,"updatedReplicas":0}}},
	"providerConfigRef":{"name":"thingy"},"writeConnectionSecretToRef":{
	"name":"test-machinepool-ng-23456","namespace":"default"}}}`
)

type NodegroupErrorMock struct {
	eks.Client
}

func (n *NodegroupErrorMock) DescribeNodegroup(ctx context.Context,
	params *eks.DescribeNodegroupInput,
	optFns ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error) {
	return nil, fmt.Errorf("just a failure")
}

func (e *NodegroupErrorMock) ListNodegroups(ctx context.Context,
	params *eks.ListNodegroupsInput,
	optFns ...func(*eks.Options)) (*eks.ListNodegroupsOutput, error) {
	return nil, fmt.Errorf("just a failure")
}

type NodegroupMock struct {
	eks.Client
}

func (n *NodegroupMock) DescribeNodegroup(ctx context.Context,
	params *eks.DescribeNodegroupInput,
	opttFns ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error) {
	fmt.Printf("%+s %+s\n", *params.ClusterName, *params.NodegroupName)
	switch *params.NodegroupName {
	case "ng-12345":
		return &eks.DescribeNodegroupOutput{
			Nodegroup: &types.Nodegroup{
				AmiType:       "AL2_x86_64",
				Version:       aws.String("1.25"),
				CapacityType:  types.CapacityTypesOnDemand,
				ClusterName:   aws.String("test"),
				CreatedAt:     aws.Time(time.Now()),
				InstanceTypes: nil,
				LaunchTemplate: &types.LaunchTemplateSpecification{
					Id:      aws.String("lt-123456"),
					Name:    aws.String("eksctl-example-nodegroup-ng-1"),
					Version: aws.String("2"),
				},
				NodeRole: aws.String("role/eksctl-example-nodegroup-NodeInstanceRole-123456789123"),
				ScalingConfig: &types.NodegroupScalingConfig{
					DesiredSize: aws.Int32(1),
					MaxSize:     aws.Int32(3),
					MinSize:     aws.Int32(1),
				},
				NodegroupArn: aws.String("arn::123456:some-role"),
				Subnets: []string{
					"subnet-1111111111111111",
					"subnet-2222222222222222",
					"subnet-3333333333333333",
				},
				NodegroupName: aws.String("ng-12345"),
				UpdateConfig: &types.NodegroupUpdateConfig{
					MaxUnavailable: aws.Int32(1),
				},
			},
		}, nil
	case "ng-23456":
		return &eks.DescribeNodegroupOutput{
			Nodegroup: &types.Nodegroup{
				AmiType:       "AL2_x86_64",
				Version:       aws.String("1.25"),
				CapacityType:  types.CapacityTypesOnDemand,
				ClusterName:   aws.String("test"),
				CreatedAt:     aws.Time(time.Now()),
				InstanceTypes: nil,
				LaunchTemplate: &types.LaunchTemplateSpecification{
					Id:      aws.String("lt-234567"),
					Name:    aws.String("eksctl-test-nodegroup-ng-1"),
					Version: aws.String("2"),
				},
				NodeRole: aws.String("role/eksctl-test-nodegroup-NodeInstanceRole-123456789123"),
				ScalingConfig: &types.NodegroupScalingConfig{
					DesiredSize: aws.Int32(1),
					MaxSize:     aws.Int32(3),
					MinSize:     aws.Int32(1),
				},
				NodegroupArn: aws.String("arn::123456:some-role"),
				Subnets: []string{
					"subnet-1111111111111111",
					"subnet-2222222222222222",
					"subnet-3333333333333333",
				},
				NodegroupName: aws.String("ng-23456"),
				UpdateConfig: &types.NodegroupUpdateConfig{
					MaxUnavailable: aws.Int32(1),
				},
				Resources: &types.NodegroupResources{
					AutoScalingGroups: []types.AutoScalingGroup{
						{
							Name: aws.String("asg-23456"),
						},
					},
				},
			},
		}, nil
	}
	return nil, nil
}

func (e *NodegroupMock) ListNodegroups(ctx context.Context,
	params *eks.ListNodegroupsInput,
	optFns ...func(*eks.Options)) (*eks.ListNodegroupsOutput, error) {
	switch *params.ClusterName {
	case "example":
		return &eks.ListNodegroupsOutput{
			Nodegroups: []string{
				"ng-12345",
			},
		}, nil
	case "test":
		return &eks.ListNodegroupsOutput{
			Nodegroups: []string{
				"ng-23456",
			},
		}, nil
	}
	return nil, nil
}

type EmptyEc2Mock struct{}

func (e *EmptyEc2Mock) DescribeLaunchTemplateVersions(ctx context.Context,
	params *ec2.DescribeLaunchTemplateVersionsInput,
	optFns ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplateVersionsOutput, error) {
	return nil, nil
}

type ValidEc2Mock struct{}

func (e *ValidEc2Mock) DescribeLaunchTemplateVersions(ctx context.Context,
	params *ec2.DescribeLaunchTemplateVersionsInput,
	optFns ...func(*ec2.Options)) (*ec2.DescribeLaunchTemplateVersionsOutput, error) {
	switch *params.LaunchTemplateId {
	case "lt-123456":
		return &ec2.DescribeLaunchTemplateVersionsOutput{
			LaunchTemplateVersions: []ec2types.LaunchTemplateVersion{
				{
					LaunchTemplateData: &ec2types.ResponseLaunchTemplateData{
						InstanceMarketOptions: &ec2types.LaunchTemplateInstanceMarketOptions{
							SpotOptions: &ec2types.LaunchTemplateSpotMarketOptions{
								MaxPrice: aws.String("expensive"),
							},
						},
						BlockDeviceMappings: []ec2types.LaunchTemplateBlockDeviceMapping{
							{
								DeviceName: aws.String("/dev/xvda"),
								Ebs: &ec2types.LaunchTemplateEbsBlockDevice{
									Iops:       aws.Int32(3000),
									VolumeSize: aws.Int32(80),
									Throughput: aws.Int32(125),
									VolumeType: ec2types.VolumeTypeGp3,
								},
							},
						},
						IamInstanceProfile: &ec2types.LaunchTemplateIamInstanceProfileSpecification{
							Name: aws.String("eks-123456789123456789"),
						},
						SecurityGroupIds: []string{
							"sg-11111111111111111",
							"sg-22222222222222222",
						},
						InstanceType: ec2types.InstanceTypeM5Large,
						KeyName:      aws.String("test-key"),
						ImageId:      aws.String("ami-0ab553a58389ae35a"),
					},
					VersionNumber: aws.Int64(1),
				},
			},
		}, nil
	case "lt-234567":
		return &ec2.DescribeLaunchTemplateVersionsOutput{
			LaunchTemplateVersions: []ec2types.LaunchTemplateVersion{
				{
					LaunchTemplateData: &ec2types.ResponseLaunchTemplateData{
						InstanceMarketOptions: &ec2types.LaunchTemplateInstanceMarketOptions{
							SpotOptions: &ec2types.LaunchTemplateSpotMarketOptions{
								MaxPrice: aws.String("expensive"),
							},
						},
						BlockDeviceMappings: []ec2types.LaunchTemplateBlockDeviceMapping{
							{
								DeviceName: aws.String("/dev/xvda"),
								Ebs: &ec2types.LaunchTemplateEbsBlockDevice{
									Iops:       aws.Int32(3000),
									VolumeSize: aws.Int32(80),
									Throughput: aws.Int32(125),
									VolumeType: ec2types.VolumeTypeGp3,
								},
							},
						},
						IamInstanceProfile: &ec2types.LaunchTemplateIamInstanceProfileSpecification{
							Arn: aws.String("arn::123456789:/role/something"),
						},
						ImageId: aws.String("ami-0ab553a58389ae35a"),
						SecurityGroupIds: []string{
							"sg-11111111111111111",
							"sg-22222222222222222",
						},
						InstanceType: ec2types.InstanceTypeM5Large,
						KeyName:      aws.String("test-key"),
					},
					VersionNumber: aws.Int64(1),
				},
			},
		}, nil
	}
	return nil, nil
}

type EmptyAsgMock struct{}

func (e *EmptyAsgMock) DescribeAutoScalingGroups(ctx context.Context,
	params *asg.DescribeAutoScalingGroupsInput,
	optFns ...func(*asg.Options)) (*asg.DescribeAutoScalingGroupsOutput, error) {
	return nil, nil
}

type ValidAsgMock struct{}

func (e *ValidAsgMock) DescribeAutoScalingGroups(ctx context.Context,
	params *asg.DescribeAutoScalingGroupsInput,
	optFns ...func(*asg.Options)) (*asg.DescribeAutoScalingGroupsOutput, error) {
	switch params.AutoScalingGroupNames[0] {
	case "asg-23456":
		return &asg.DescribeAutoScalingGroupsOutput{
			AutoScalingGroups: []asgtypes.AutoScalingGroup{
				{
					AutoScalingGroupName: aws.String("example"),
					AvailabilityZones: []string{
						"eu-central-1a",
						"eu-central-1c",
						"eu-central-1b",
					},
					Instances: []asgtypes.Instance{
						{
							InstanceId:       aws.String("i-1111111111111111"),
							AvailabilityZone: aws.String("eu-central-1c"),
						},
						{
							InstanceId:       aws.String("i-2222222222222222"),
							AvailabilityZone: aws.String("eu-central-1a"),
						},
						{
							InstanceId:       aws.String("i-3333333333333333"),
							AvailabilityZone: aws.String("eu-central-1b"),
						},
					},
					MixedInstancesPolicy: &asgtypes.MixedInstancesPolicy{
						LaunchTemplate: &asgtypes.LaunchTemplate{
							LaunchTemplateSpecification: &asgtypes.LaunchTemplateSpecification{
								LaunchTemplateId:   aws.String("lt-234567"),
								LaunchTemplateName: aws.String("test-12345"),
								Version:            aws.String("1"),
							},
						},
					},
				},
			},
		}, nil
	default:
		return &asg.DescribeAutoScalingGroupsOutput{
			AutoScalingGroups: []asgtypes.AutoScalingGroup{
				{
					AutoScalingGroupName: aws.String("example"),
					AvailabilityZones: []string{
						"eu-central-1a",
						"eu-central-1c",
						"eu-central-1b",
					},
					Instances: []asgtypes.Instance{
						{
							InstanceId:       aws.String("i-1111111111111111"),
							AvailabilityZone: aws.String("eu-central-1c"),
						},
						{
							InstanceId:       aws.String("i-2222222222222222"),
							AvailabilityZone: aws.String("eu-central-1a"),
						},
						{
							InstanceId:       aws.String("i-3333333333333333"),
							AvailabilityZone: aws.String("eu-central-1b"),
						},
					},
				},
			},
		}, nil
	}
}

func TestRunFunction(t *testing.T) {

	type args struct {
		ctx context.Context
		req *fnv1beta1.RunFunctionRequest
	}
	type want struct {
		rsp *fnv1beta1.RunFunctionResponse
		err error
	}

	type mocks struct {
		ec2 func(cfg aws.Config) AwsEc2Api
		eks func(cfg aws.Config) AwsEksApi
		asg func(cfg aws.Config) AwsAsgApi
		aws func(region, provider *string) (aws.Config, error)
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
		mocks  mocks
	}{
		"input is undefined": {
			reason: "When cluster ref is undefined, we get a fatal response",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Input{}),
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Results: []*fnv1beta1.Result{
						{
							Severity: fnv1beta1.Severity_SEVERITY_FATAL,
							Message:  "object does not contain spec field",
						},
					},
				},
			},
			mocks: mocks{},
		},
		"spec is empty": {
			reason: "the function returns normal if spec is not yet populated",
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Meta: &fnv1beta1.RequestMeta{Tag: "hello"},
					Input: resource.MustStructJSON(`{
						"apiVersion": "dummy.fn.crossplane.io",
						"kind": "Input",
						"spec": {}
					}`),
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Tag: "hello", Ttl: durationpb.New(response.DefaultTTL)},
				},
			},
			mocks: mocks{},
		},
		"function returns fatal if nodegroups cannot be loaded": {
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Input{
						Spec: &v1beta1.Spec{
							ClusterRef: "eks-cluster",
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(xrExample),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"eks-cluster": {
								Resource: resource.MustStructJSON(clusterExample),
							},
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(xrExample),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"eks-cluster": {
								Resource: resource.MustStructJSON(clusterExample),
							},
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Results: []*fnv1beta1.Result{
						{
							Severity: fnv1beta1.Severity_SEVERITY_FATAL,
							Message:  "cannot create composed resources from *v1beta1.RunFunctionRequest: failed to load nodegroups for cluster \"example\": just a failure",
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(xrExample),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"eks-cluster": {
								Resource: resource.MustStructJSON(clusterExample),
							},
						},
					},
				},
			},
			mocks: mocks{
				aws: func(region, provider *string) (aws.Config, error) {
					return aws.Config{}, nil
				},
				eks: func(_ aws.Config) AwsEksApi {
					return &NodegroupErrorMock{}
				},
				ec2: func(_ aws.Config) AwsEc2Api {
					return &EmptyEc2Mock{}
				},
				asg: func(_ aws.Config) AwsAsgApi {
					return &EmptyAsgMock{}
				},
			},
		},
		"function returns success when nodepool is created example cluster": {
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Input{
						Spec: &v1beta1.Spec{
							ClusterRef: "eks-cluster",
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(xrExample),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"eks-cluster": {
								Resource: resource.MustStructJSON(clusterExample),
							},
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(xrExample),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"eks-cluster": {
								Resource: resource.MustStructJSON(clusterExample),
							},
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(xrExample),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"eks-cluster": {
								Resource: resource.MustStructJSON(clusterExample),
							},
							"example-awsmanagedmachinepool-ng-12345": {
								Ready:    fnv1beta1.Ready_READY_TRUE,
								Resource: resource.MustStructJSON(nodepoolExample),
							},
							"example-machinepool-ng-12345": {
								Ready:    fnv1beta1.Ready_READY_TRUE,
								Resource: resource.MustStructJSON(machinepoolExample),
							},
						},
					},
				},
			},
			mocks: mocks{
				aws: func(region, provider *string) (aws.Config, error) {
					return aws.Config{}, nil
				},
				eks: func(_ aws.Config) AwsEksApi {
					return &NodegroupMock{}
				},
				ec2: func(_ aws.Config) AwsEc2Api {
					return &ValidEc2Mock{}
				},
				asg: func(_ aws.Config) AwsAsgApi {
					return &ValidAsgMock{}
				},
			},
		},
		"function returns success when nodepool is created test cluster": {
			args: args{
				req: &fnv1beta1.RunFunctionRequest{
					Input: resource.MustStructObject(&v1beta1.Input{
						Spec: &v1beta1.Spec{
							ClusterRef: "eks-cluster",
						},
					}),
					Observed: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(xrTest),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"eks-cluster": {
								Resource: resource.MustStructJSON(clusterTest),
							},
						},
					},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(xrTest),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"eks-cluster": {
								Resource: resource.MustStructJSON(clusterTest),
							},
						},
					},
				},
			},
			want: want{
				rsp: &fnv1beta1.RunFunctionResponse{
					Meta: &fnv1beta1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1beta1.State{
						Composite: &fnv1beta1.Resource{
							Resource: resource.MustStructJSON(xrTest),
						},
						Resources: map[string]*fnv1beta1.Resource{
							"eks-cluster": {
								Resource: resource.MustStructJSON(clusterTest),
							},
							"test-awsmanagedmachinepool-ng-23456": {
								Ready:    fnv1beta1.Ready_READY_TRUE,
								Resource: resource.MustStructJSON(nodepoolTest),
							},
							"test-machinepool-ng-23456": {
								Ready:    fnv1beta1.Ready_READY_TRUE,
								Resource: resource.MustStructJSON(machinepoolTest),
							},
						},
					},
				},
			},
			mocks: mocks{
				aws: func(region, provider *string) (aws.Config, error) {
					return aws.Config{}, nil
				},
				eks: func(_ aws.Config) AwsEksApi {
					return &NodegroupMock{}
				},
				ec2: func(_ aws.Config) AwsEc2Api {
					return &ValidEc2Mock{}
				},
				asg: func(_ aws.Config) AwsAsgApi {
					return &ValidAsgMock{}
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// set up any required mocks
			awsConfig = tc.mocks.aws
			getAsgClient = tc.mocks.asg
			getEc2Client = tc.mocks.ec2
			getEksClient = tc.mocks.eks

			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)
			if diff := cmp.Diff(tc.want.rsp, rsp, protocmp.Transform()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want rsp, +got rsp:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}
