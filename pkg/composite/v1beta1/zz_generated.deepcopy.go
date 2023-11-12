//go:build !ignore_autogenerated

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import ()

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClaimRef) DeepCopyInto(out *ClaimRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClaimRef.
func (in *ClaimRef) DeepCopy() *ClaimRef {
	if in == nil {
		return nil
	}
	out := new(ClaimRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CompositeObject) DeepCopyInto(out *CompositeObject) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CompositeObject.
func (in *CompositeObject) DeepCopy() *CompositeObject {
	if in == nil {
		return nil
	}
	out := new(CompositeObject)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CompositionSelector) DeepCopyInto(out *CompositionSelector) {
	*out = *in
	out.MatchLabels = in.MatchLabels
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CompositionSelector.
func (in *CompositionSelector) DeepCopy() *CompositionSelector {
	if in == nil {
		return nil
	}
	out := new(CompositionSelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MatchLabels) DeepCopyInto(out *MatchLabels) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MatchLabels.
func (in *MatchLabels) DeepCopy() *MatchLabels {
	if in == nil {
		return nil
	}
	out := new(MatchLabels)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *XrClaimSpec) DeepCopyInto(out *XrClaimSpec) {
	*out = *in
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.KubernetesAdditionalLabels != nil {
		in, out := &in.KubernetesAdditionalLabels, &out.KubernetesAdditionalLabels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new XrClaimSpec.
func (in *XrClaimSpec) DeepCopy() *XrClaimSpec {
	if in == nil {
		return nil
	}
	out := new(XrClaimSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *XrObjectDefinition) DeepCopyInto(out *XrObjectDefinition) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new XrObjectDefinition.
func (in *XrObjectDefinition) DeepCopy() *XrObjectDefinition {
	if in == nil {
		return nil
	}
	out := new(XrObjectDefinition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *XrSpec) DeepCopyInto(out *XrSpec) {
	*out = *in
	in.XrClaimSpec.DeepCopyInto(&out.XrClaimSpec)
	out.ClaimRef = in.ClaimRef
	out.CompositionSelector = in.CompositionSelector
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new XrSpec.
func (in *XrSpec) DeepCopy() *XrSpec {
	if in == nil {
		return nil
	}
	out := new(XrSpec)
	in.DeepCopyInto(out)
	return out
}