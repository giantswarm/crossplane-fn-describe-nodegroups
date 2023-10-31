package main

import (
	"context"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/giantswarm/crossplane-fn-describe-nodegroups/input/v1beta1"
	"github.com/giantswarm/xfnlib/pkg/composite"
)

const composedName = "crossplane-fn-describe-nodegroups"

// RunFunction Execute the desired reconcilliation state, creating any required resources
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (rsp *fnv1beta1.RunFunctionResponse, err error) {
	f.log.Info("preparing function", composedName, req.GetMeta().GetTag())
	rsp = response.To(req, response.DefaultTTL)

	input := v1beta1.Input{}
	if f.composed, err = composite.New(req, &input, &f.composite); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "error setting up function "+composedName))
		return rsp, nil
	}

	if _, ok := f.composed.ObservedComposed[input.Spec.ClusterRef]; !ok {
		response.Normal(rsp, "Waiting for resource")
		return rsp, nil
	}

	var (
		clusterName       *string = &f.composite.Spec.ClusterName
		namespace         *string = &f.composite.Spec.ClaimRef.Namespace
		region            *string = &f.composite.Spec.Region
		provider          *string = &f.composite.Spec.CompositionSelector.MatchLabels.Provider
		providerConfigRef *string = &f.composite.Spec.CloudProviderConfigRef

		labels      map[string]string = f.composite.Metadata.Labels
		annotations map[string]string = map[string]string{
			"cluster.x-k8s.io/managed-by": "crossplane",
		}
	)

	// Merge in the additional labels for kubernetes resources
	for k, v := range f.composite.Spec.KubernetesAdditionalLabels {
		labels[k] = v
	}

	f.log.Info(*provider)
	switch strings.ToLower(*provider) {
	case "aws":
		f.log.Info("discovered aws provider", composedName, req.GetMeta().GetTag())
		if err = f.CreateAWSNodegroupSpec(clusterName, namespace, region, providerConfigRef, labels, annotations); err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composite resources from %T", req))
			return rsp, nil
		}
	case "azure":
		f.log.Info("Azure provider is not yet implemented")
	case "gcp":
		f.log.Info("GCP provider is not yet implemented")
	}

	if err = f.composed.ToResponse(rsp); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot convert composition to response %T", rsp))
		return
	}

	response.Normal(rsp, "Successful run")
	return rsp, nil
}
