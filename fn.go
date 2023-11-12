package main

import (
	"context"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/giantswarm/crossplane-fn-describe-nodegroups/pkg/input/v1beta1"
	"github.com/giantswarm/xfnlib/pkg/composite"
)

const composedName = "crossplane-fn-describe-nodegroups"

// RunFunction Execute the desired reconcilliation state, creating any required resources
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (rsp *fnv1beta1.RunFunctionResponse, err error) {
	f.log.Info("preparing function", composedName, req.GetMeta().GetTag())

	rsp = response.To(req, response.DefaultTTL)

	var (
		ac    XrConfig = XrConfig{}
		input v1beta1.Input
	)
	if ac.composed, err = composite.New(req, &input, &ac.composite); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "error setting up function "+composedName))
		return rsp, nil
	}

	if input.Spec == nil {
		response.Fatal(rsp, &composite.MissingSpec{})
		return rsp, nil
	}

	if _, ok := ac.composed.ObservedComposed[input.Spec.ClusterRef]; !ok {
		response.Normal(rsp, "waiting for resource")
		return rsp, nil
	}

	ac.cluster = &ac.composite.Spec.ClusterName
	ac.namespace = &ac.composite.Spec.ClaimRef.Namespace
	ac.region = &ac.composite.Spec.Region
	ac.providerConfigRef = &ac.composite.Spec.CloudProviderConfigRef

	ac.labels = ac.composite.Metadata.Labels
	if ac.labels == nil {
		ac.labels = make(map[string]string)
	}
	for k, v := range ac.composite.Spec.KubernetesAdditionalLabels {
		ac.labels[k] = v
	}
	ac.labels["cluster.x-k8s.io/cluster-name"] = *ac.cluster
	ac.labels["giantswarm.io/cluster"] = *ac.cluster

	var provider string = ac.composite.Spec.CompositionSelector.MatchLabels.Provider
	{
		switch strings.ToLower(provider) {
		case "aws":
			f.log.Info("discovered aws provider", composedName, req.GetMeta().GetTag())
			if err = f.CreateAWSNodegroupSpec(&ac); err != nil {
				response.Fatal(rsp, errors.Wrapf(err, "cannot create composed resources from %T", req))
				return rsp, nil
			}
		case "azure":
			f.log.Info("Azure provider is not yet implemented")
		case "gcp":
			f.log.Info("GCP provider is not yet implemented")
		}
	}

	if err = ac.composed.ToResponse(rsp); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot convert composition to response %T", rsp))
		return
	}

	response.Normal(rsp, "Successful run")
	return rsp, nil
}
