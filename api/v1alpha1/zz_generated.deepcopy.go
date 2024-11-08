//go:build !ignore_autogenerated

/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Annotation) DeepCopyInto(out *Annotation) {
	*out = *in
	if in.ValidValues != nil {
		in, out := &in.ValidValues, &out.ValidValues
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.FinalStateValues != nil {
		in, out := &in.FinalStateValues, &out.FinalStateValues
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Annotation.
func (in *Annotation) DeepCopy() *Annotation {
	if in == nil {
		return nil
	}
	out := new(Annotation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MaintenanceCondition) DeepCopyInto(out *MaintenanceCondition) {
	*out = *in
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MaintenanceCondition.
func (in *MaintenanceCondition) DeepCopy() *MaintenanceCondition {
	if in == nil {
		return nil
	}
	out := new(MaintenanceCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MessageChannels) DeepCopyInto(out *MessageChannels) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MessageChannels.
func (in *MessageChannels) DeepCopy() *MessageChannels {
	if in == nil {
		return nil
	}
	out := new(MessageChannels)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NotifyMaintenance) DeepCopyInto(out *NotifyMaintenance) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NotifyMaintenance.
func (in *NotifyMaintenance) DeepCopy() *NotifyMaintenance {
	if in == nil {
		return nil
	}
	out := new(NotifyMaintenance)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NotifyMaintenance) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NotifyMaintenanceList) DeepCopyInto(out *NotifyMaintenanceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]NotifyMaintenance, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NotifyMaintenanceList.
func (in *NotifyMaintenanceList) DeepCopy() *NotifyMaintenanceList {
	if in == nil {
		return nil
	}
	out := new(NotifyMaintenanceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *NotifyMaintenanceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NotifyMaintenanceSpec) DeepCopyInto(out *NotifyMaintenanceSpec) {
	*out = *in
	out.AdditionalMessageChannels = in.AdditionalMessageChannels
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NotifyMaintenanceSpec.
func (in *NotifyMaintenanceSpec) DeepCopy() *NotifyMaintenanceSpec {
	if in == nil {
		return nil
	}
	out := new(NotifyMaintenanceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NotifyMaintenanceStatus) DeepCopyInto(out *NotifyMaintenanceStatus) {
	*out = *in
	if in.SLAExpires != nil {
		in, out := &in.SLAExpires, &out.SLAExpires
		*out = (*in).DeepCopy()
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]MaintenanceCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.TransitionTimestamps != nil {
		in, out := &in.TransitionTimestamps, &out.TransitionTimestamps
		*out = make([]StatusTransitionTimestamp, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NotifyMaintenanceStatus.
func (in *NotifyMaintenanceStatus) DeepCopy() *NotifyMaintenanceStatus {
	if in == nil {
		return nil
	}
	out := new(NotifyMaintenanceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StatusTransitionTimestamp) DeepCopyInto(out *StatusTransitionTimestamp) {
	*out = *in
	in.TransitionTimestamp.DeepCopyInto(&out.TransitionTimestamp)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StatusTransitionTimestamp.
func (in *StatusTransitionTimestamp) DeepCopy() *StatusTransitionTimestamp {
	if in == nil {
		return nil
	}
	out := new(StatusTransitionTimestamp)
	in.DeepCopyInto(out)
	return out
}
