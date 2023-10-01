package main

import (
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/giantswarm/xfnlib/pkg/composite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Policy Policies for referencing.
type Policy struct {
	Resolution string `json:"resolution,omitempty"`
	Resolve    string `json:"resolve,omitempty"`
}

// ProviderConfigRef specifies how the provider that will be used to create, observe, update, and delete this managed resource should be configured.
type ProviderConfigRef struct {
	Name   string `json:"name"`
	Policy Policy `json:"policy,omitempty"`
}

// ConnectionSecretRef specifies the namespace and name of a Secret to which any connection details for this managed resource should be written.
type ConnectionSecretRef struct {
	Namespace string `json:"namespace"`
}

// EksImportXRObject is the information we are going to pull from the XR
type EksImportXRObject struct {
	Metadata metav1.ObjectMeta `json:"metadata"`
	Spec     XRSpec            `json:"spec"`
}

type XRSpec struct {
	KubernetesAdditionalLabels  map[string]string `json:"kubernetesAdditionalLabels"`
	Labels                      map[string]string `json:"labels"`
	AwsProviderConfigRef        string            `json:"cloudProviderConfigRef"`
	ClusterName                 string            `json:"clusterName"`
	DeletionPolicy              string            `json:"deletionPolicy"`
	KubernetesProviderConfigRef string            `json:"clusterProviderConfigRef"`
	Region                      string            `json:"regionOrLocation"`
	ResourceGroupName           string            `json:"resourceGroupName,omitempty"`
	ClaimRef                    struct {
		Namespace string `json:"namespace"`
	} `json:"claimRef"`

	CompositionSelector struct {
		MatchLabels struct {
			Provider string `json:"provider"`
		} `json:"matchLabels"`
	} `json:"compositionSelector"`
}

type XRStatus struct {
	AWSRoleArn string `json:"roleArn"`
}

// Function returns whatever response you ask it to.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer
	log       logging.Logger
	composed  *composite.Composition
	composite EksImportXRObject
}
