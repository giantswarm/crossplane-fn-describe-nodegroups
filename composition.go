package main

import (
	"encoding/json"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/giantswarm/function-describe-nodegroups/input/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type MissingMetadata struct{}

func (e *MissingMetadata) Error() string {
	return "object does not contain metadata"
}

type InvalidMetadata struct{}

func (e *InvalidMetadata) Error() string {
	return "invalid or empty metadata object"
}

type MissingSpec struct{}

func (e *MissingSpec) Error() string {
	return "object does not contain spec field"
}

type InvalidSpec struct{}

func (e *InvalidSpec) Error() string {
	return "invalid or empty object spec"
}

// Composition contains the main request objects required for interacting with composition function resources.
type Composition struct {
	// ObservedComposite is an object that reflects the composite resource that is created from the claim
	ObservedComposite EksImportXRObject

	// DesiredComposite is the raw composite resource we want creating
	DesiredComposite *resource.Composite

	// ObservedComposed is a set of resources that are composed by the composite and exist in the cluster
	ObservedComposed map[resource.Name]resource.ObservedComposed

	// DesiredComposed is the set of resources we require to be created
	DesiredComposed map[resource.Name]*resource.DesiredComposed

	// Input is the information brought in from the function binding
	Input v1beta1.Input
}

// RequestToComposition takes a RunFunctionRequest object and converts it to a Composition
//
// This method should be called at the top of your RunFunction.
//
// Example:
//
//	func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (rsp *fnv1beta1.RunFunctionResponse, err error) {
//		f.log.Info("Running Function", composedName, req.GetMeta().GetTag())
//		rsp = response.To(req, response.DefaultTTL)
//
//		if f.composed, err = RequestToComposition(req); err != nil {
//			response.Fatal(rsp, errors.Wrap(err, "error setting up function "+composedName))
//			return rsp, nil
//		}
//		...
//		// Function body
//		...
//		if err = f.composed.ToResponse(rsp); err != nil {
//			response.Fatal(rsp, errors.Wrapf(err, "cannot convert composition to response %T", rsp))
//			return
//		}
//
//		response.Normal(rsp, "Successful run")
//		return rsp, nil
//	}
func RequestToComposition(req *fnv1beta1.RunFunctionRequest) (c *Composition, err error) {
	c = &Composition{}

	if c.DesiredComposite, err = request.GetDesiredCompositeResource(req); err != nil {
		err = errors.Wrapf(err, "cannot get desired composed resources from %T", req)
		return
	}

	var oxr *resource.Composite
	if oxr, err = request.GetObservedCompositeResource(req); err != nil {
		err = errors.Wrap(err, "cannot get observed composite resource")
		return
	}

	if err = c.To(oxr.Resource.Object, &c.ObservedComposite); err != nil {
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
		return
	}

	//c.Input = &v1beta1.Input{}
	if err = request.GetInput(req, &c.Input); err != nil {
		return
	}

	if c.Input.Spec == nil {
		return nil, &WaitingForSpec{}
	}

	return
}

// ToResponse converts the composition back into the response object
//
// This method should be called at the end of your RunFunction immediately before returning a normal response.
// Wrap this in an error handler and set `response.Fatal` on error
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

// AddDesired takes an unstructured object and adds it to the desired composed resources
func (c *Composition) AddDesired(name string, object *unstructured.Unstructured) (err error) {
	c.DesiredComposed[resource.Name(name)] = &resource.DesiredComposed{
		Resource: &composed.Unstructured{
			Unstructured: *object,
		},
		Ready: resource.ReadyTrue,
	}
	return
}

// ToUnstructured is a helper function that creates an unstructured object from any object
// that contains metadata, spec and optionally status.
func (c *Composition) ToUnstructured(apiVersion, kind, object any) (objectSpec *unstructured.Unstructured, err error) {
	objectSpec = &unstructured.Unstructured{}
	type objS struct {
		Metadata map[string]interface{}
		Spec     map[string]interface{}
		Status   map[string]interface{}
	}
	var o objS
	if err = c.To(object, &o); err != nil {
		return
	}

	if len(o.Metadata) == 0 {
		err = &InvalidMetadata{}
		return
	}

	if len(o.Spec) == 0 {
		err = &InvalidSpec{}
		return
	}

	objectSpec.Object = map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   o.Metadata,
		"spec":       o.Spec,
	}
	if len(o.Status) > 0 {
		objectSpec.Object["status"] = o.Status
	}
	return
}

// ToUnstructuredKubernetesObject is a helper function that wraps a given CR resource in
// a `crossplane-contrib/provider-kubernetes.Object` structure and returns this as an unstructured.Unstructured object
func (c *Composition) ToUnstructuredKubernetesObject(mp any) (objectSpec *unstructured.Unstructured, err error) {
	objectSpec = &unstructured.Unstructured{}
	var unstructuredData map[string]interface{}
	if err = c.To(mp, &unstructuredData); err != nil {
		return
	}

	if _, ok := unstructuredData["metadata"]; !ok {
		err = errors.Wrap(&MissingMetadata{}, "unable to create kubernetes object. object missing metadata")
		return
	}

	var meta metav1.ObjectMeta
	if err = c.To(unstructuredData["metadata"], &meta); err != nil {
		err = errors.Wrap(err, fmt.Sprintf("unable to create kubernetes object : %+v", unstructuredData["metadata"]))
		return
	}

	var labels map[string]interface{} = make(map[string]interface{})
	for k, v := range meta.Labels {
		labels[k] = v
	}

	objectSpec.Object = map[string]interface{}{
		"apiVersion": "kubernetes.crossplane.io/v1alpha1",
		"kind":       "Object",
		"metadata": map[string]interface{}{
			"name":   meta.Name,
			"labels": labels,
		},
		"spec": map[string]interface{}{
			"forProvider": map[string]interface{}{
				"manifest": unstructuredData,
			},
			"writeConnectionSecretToRef": map[string]interface{}{
				"name":      meta.Name,
				"namespace": meta.Namespace,
			},
			"providerConfigRef": map[string]interface{}{
				"name": c.ObservedComposite.Spec.KubernetesProviderConfigRef,
			},
		},
	}
	return
}

// To is a helper function that converts any object to any object by sending it round-robin through `json.Marshal`
func (c *Composition) To(resource any, jsonObject any) (err error) {
	var b []byte
	if b, err = json.Marshal(resource); err != nil {
		return
	}

	err = json.Unmarshal(b, jsonObject)
	return
}
