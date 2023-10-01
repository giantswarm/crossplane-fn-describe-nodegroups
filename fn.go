package main

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	fnv1beta1 "github.com/crossplane/function-sdk-go/proto/v1beta1"
	"github.com/crossplane/function-sdk-go/response"
)

const composedName = "function-describe-nodegroups"

// RunFunction Execute the desired reconcilliation state, creating any required resources
func (f *Function) RunFunction(_ context.Context, req *fnv1beta1.RunFunctionRequest) (rsp *fnv1beta1.RunFunctionResponse, err error) {
	f.log.Info("Running Function", composedName, req.GetMeta().GetTag())
	rsp = response.To(req, response.DefaultTTL)

	if f.composed, err = NewComposition(req); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "error setting up function "+composedName))
		return rsp, nil
	}

	var arn string
	if arn, err = f.getAssumeRoleArn(); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "error retrieving provider assume role arn "+composedName))
		return rsp, nil
	}

	var (
		clusterName string = f.composed.ObservedComposite.Spec.ClusterName
		namespace   string = f.composed.ObservedComposite.Spec.ClaimRef.Namespace
		region      string = f.composed.ObservedComposite.Spec.Region

		labels      map[string]string = f.composed.ObservedComposite.Metadata.Labels
		annotations map[string]string = map[string]string{
			"cluster.x-k8s.io/managed-by": "crossplane",
		}
	)

	// Merge in the additional labels for kubernetes resources
	for k, v := range f.composed.ObservedComposite.Spec.KubernetesAdditionalLabels {
		labels[k] = v
	}

	if err = f.CreateNodegroupSpec(clusterName, namespace, region, arn, labels, annotations); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composite resources from %T", req))
		return rsp, nil
	}

	if err = f.composed.ToResponse(rsp); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set resources to %T", rsp))
		return
	}

	response.Normal(rsp, "Successful run")
	return rsp, nil
}
