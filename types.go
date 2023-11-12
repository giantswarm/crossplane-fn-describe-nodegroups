package main

import (
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	xfc "github.com/giantswarm/crossplane-fn-describe-nodegroups/pkg/composite/v1beta1"
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
	Spec     xfc.XrSpec        `json:"spec"`
}

type XrConfig struct {
	cluster, namespace, region, providerConfigRef *string
	labels, annotations                           map[string]string
	composed                                      *composite.Composition
	composite                                     EksImportXRObject
}

// Function returns whatever response you ask it to.
type Function struct {
	fnv1beta1.UnimplementedFunctionRunnerServiceServer
	log logging.Logger
}
