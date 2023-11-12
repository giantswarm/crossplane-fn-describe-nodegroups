// Package v1beta1 contains the definition of the XR requirements for using this function
// +groupName=definition
// +versionName=v1beta1
package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane;composition;functions;subnets

// XrObjectDefinition contains information about the XR
//
// This type is a meta-type for defining the XRD spec as it excludes
// fields normally defined as part of a standard XRD definition
type XrObjectDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec The specification of the XR
	Spec XrClaimSpec `json:"spec"`
}

type CompositeObject struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec The specification of the XR
	Spec XrSpec `json:"spec"`
}

type XrClaimSpec struct {
	// Labels is a set of additional labels to be applied to all objects
	// +optional
	// +mapType=granular
	Labels map[string]string `json:"labels"`

	// Defines the name of the providerconfig for the cloud provider
	// +kubebuilder:validation:Required
	CloudProviderConfigRef string `json:"cloudProviderConfigRef"`

	// Defines the name of the providerconfig used by `crossplane-contrib/provider-kubernetes`
	// +kubebuilder:validation:Required
	ClusterProviderConfigRef string `json:"clusterProviderConfigRef"`

	// Defines the name of the cluster to map from
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName"`

	// Defines the region or location for cloud resources
	// +kubebuilder:validation:Required
	Region string `json:"regionOrLocation"`

	// The deletion policy for kubernetes objects
	// +kubebuilder:validation:Required
	ObjectDeletionPolicy string `json:"objectDeletionPolicy,omitempty"`

	// Additional labels to add to kubernetes resources
	// +optional
	// +mapType=granular
	KubernetesAdditionalLabels map[string]string `json:"kubernetesAdditionalLabels"`

	// AZURE ONLY The name of the resource group that the cluster is located in
	// This has no effect if set for Google cloud or AWS
	// +optional
	ResourceGroupName string `json:"resourceGroupName,omitempty"`
}

// XRSpec is the definition of the XR as an object
type XrSpec struct {
	XrClaimSpec `json:",inline"`
	// Defines the deletion policy for the XR
	// +optional
	DeletionPolicy string `json:"deletionPolicy"`

	// Defines a reference to the claim used by this XR
	// +optional
	ClaimRef ClaimRef `json:"claimRef"`

	// Defines the selector for the composition
	// +optional
	CompositionSelector CompositionSelector `json:"compositionSelector"`
}

// ClaimRef stores information about the claim
type ClaimRef struct {
	// The namespace the claim is stored in
	Namespace string `json:"namespace"`
}

// The selector for the composition
type CompositionSelector struct {
	MatchLabels MatchLabels `json:"matchLabels"`
}

// Labels to match on the composition for selection
type MatchLabels struct {
	// The provider label used to select the composition
	Provider string `json:"provider"`
}
