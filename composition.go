package main

import (
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	expinfrav2 "sigs.k8s.io/cluster-api-provider-aws/v2/exp/api/v1beta2"
)

type Composition struct {
	// ObservedComposite is an object that reflects the composite resource that is created from the claim
	ObservedComposite EksImportXRObject

	// DesiredComposite is the raw composite resource we want creating
	DesiredComposite *resource.Composite

	// ObservedComposed is a set of resources that are composed by the composite and exist in the cluster
	ObservedComposed map[resource.Name]resource.ObservedComposed

	// DesiredComposed is the set of resources we require to be created
	DesiredComposed map[resource.Name]*resource.DesiredComposed
}

func NewComposition(req *fnv1beta1.RunFunctionRequest) (c *Composition, err error) {
	c = &Composition{}

	// This is the "empty" version of the composite resource. It is pretty much just apiVersion and kind
	if c.DesiredComposite, err = request.GetDesiredCompositeResource(req); err != nil {
		err = errors.Wrapf(err, "cannot get desired composed resources from %T", req)
		return
	}

	// Observed composite resource is what the claim provides to the composite
	var oxr *resource.Composite
	if oxr, err = request.GetObservedCompositeResource(req); err != nil {
		err = errors.Wrap(err, "cannot get observed composite resource")
		return
	}

	if err = toObject(oxr.Resource.Object, &c.ObservedComposite); err != nil {
		err = errors.Wrapf(err, "Failed to convert XR object to struct %T", c.ObservedComposite)
		return
	}

	c.DesiredComposite.Resource.SetAPIVersion(oxr.Resource.GetAPIVersion())
	c.DesiredComposite.Resource.SetKind(oxr.Resource.GetKind())

	if c.DesiredComposed, err = request.GetDesiredComposedResources(req); err != nil {
		err = errors.Wrapf(err, "cannot get desired composite resources from %T", req)
		return
	}

	if c.ObservedComposed, err = request.GetObservedComposedResources(req); err != nil {
		err = errors.Wrapf(err, "cannot get observed composed resources from %T", req)
	}

	return
}

func (c *Composition) ToResponse(rsp *fnv1beta1.RunFunctionResponse) (err error) {
	if err = response.SetDesiredCompositeResource(rsp, c.DesiredComposite); err != nil {
		err = errors.Wrapf(err, "cannot set desired composite resources in %T", rsp)
		return
	}

	if err = response.SetDesiredComposedResources(rsp, c.DesiredComposed); err != nil {
		err = errors.Wrapf(err, "cannot set desired composed resources in %T", rsp)
	}
	return
}

func (c *Composition) AddDesired(object *expinfrav2.AWSManagedMachinePool) (err error) {
	var objectSpec unstructured.Unstructured
	if objectSpec, err = c.toKubernetesObject(object); err != nil {
		return
	}

	var name resource.Name = resource.Name(object.ObjectMeta.Name)
	c.DesiredComposed[name] = &resource.DesiredComposed{
		Resource: &composed.Unstructured{
			Unstructured: objectSpec,
		},
		Ready: resource.ReadyTrue,
	}
	return
}

func (c *Composition) toKubernetesObject(mp *expinfrav2.AWSManagedMachinePool) (objectSpec unstructured.Unstructured, err error) {
	objectSpec = unstructured.Unstructured{}
	var unstructuredData map[string]interface{}
	if err = toObject(mp, &unstructuredData); err != nil {
		return
	}

	var labels map[string]interface{} = make(map[string]interface{})
	for k, v := range mp.ObjectMeta.Labels {
		labels[k] = v
	}

	objectSpec.Object = map[string]interface{}{
		"apiVersion": "kubernetes.crossplane.io/v1alpha1",
		"kind":       "Object",
		"metadata": map[string]interface{}{
			"name":   mp.ObjectMeta.Name,
			"labels": labels,
		},
		"spec": map[string]interface{}{
			"forProvider": map[string]interface{}{
				"manifest": unstructuredData,
			},
			"writeConnectionSecretToRef": map[string]interface{}{
				"name":      mp.ObjectMeta.Name,
				"namespace": mp.ObjectMeta.Namespace,
			},
			"providerConfigRef": map[string]interface{}{
				"name": c.ObservedComposite.Spec.KubernetesProviderConfigRef,
			},
		},
	}
	return
}
