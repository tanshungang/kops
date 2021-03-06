/*
Copyright 2016 The Kubernetes Authors.

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

package gcetasks

import (
	"fmt"
	compute "google.golang.org/api/compute/v0.beta"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/gce"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"
	"reflect"
)

//go:generate fitask -type=InstanceGroupManager
type InstanceGroupManager struct {
	Name *string

	Zone             *string
	BaseInstanceName *string
	InstanceTemplate *InstanceTemplate
	TargetSize       *int64

	TargetPools []*TargetPool
}

var _ fi.CompareWithID = &InstanceGroupManager{}

func (e *InstanceGroupManager) CompareWithID() *string {
	return e.Name
}

func (e *InstanceGroupManager) Find(c *fi.Context) (*InstanceGroupManager, error) {
	cloud := c.Cloud.(*gce.GCECloud)

	r, err := cloud.Compute.InstanceGroupManagers.Get(cloud.Project, *e.Zone, *e.Name).Do()
	if err != nil {
		if gce.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error listing InstanceGroupManagers: %v", err)
	}

	actual := &InstanceGroupManager{}
	actual.Name = &r.Name
	actual.Zone = fi.String(lastComponent(r.Zone))
	actual.BaseInstanceName = &r.BaseInstanceName
	actual.TargetSize = &r.TargetSize
	actual.InstanceTemplate = &InstanceTemplate{Name: fi.String(lastComponent(r.InstanceTemplate))}

	for _, targetPool := range r.TargetPools {
		actual.TargetPools = append(actual.TargetPools, &TargetPool{
			Name: fi.String(lastComponent(targetPool)),
		})
	}
	// TODO: Sort by name

	return actual, nil
}

func (e *InstanceGroupManager) Run(c *fi.Context) error {
	return fi.DefaultDeltaRunMethod(e, c)
}

func (_ *InstanceGroupManager) CheckChanges(a, e, changes *InstanceGroupManager) error {
	return nil
}

func (_ *InstanceGroupManager) RenderGCE(t *gce.GCEAPITarget, a, e, changes *InstanceGroupManager) error {
	project := t.Cloud.Project

	i := &compute.InstanceGroupManager{
		Name:             *e.Name,
		Zone:             *e.Zone,
		BaseInstanceName: *e.BaseInstanceName,
		TargetSize:       *e.TargetSize,
		InstanceTemplate: e.InstanceTemplate.URL(project),
	}

	for _, targetPool := range e.TargetPools {
		i.TargetPools = append(i.TargetPools, targetPool.URL(t.Cloud))
	}

	if a == nil {
		//for {
		op, err := t.Cloud.Compute.InstanceGroupManagers.Insert(t.Cloud.Project, *e.Zone, i).Do()
		if err != nil {
			return fmt.Errorf("error creating InstanceGroupManager: %v", err)
		}

		if err := t.Cloud.WaitForOp(op); err != nil {
			return fmt.Errorf("error creating InstanceGroupManager: %v", err)
		}
	} else {
		if changes.TargetPools != nil {
			request := &compute.InstanceGroupManagersSetTargetPoolsRequest{
				TargetPools: i.TargetPools,
			}
			op, err := t.Cloud.Compute.InstanceGroupManagers.SetTargetPools(t.Cloud.Project, *e.Zone, i.Name, request).Do()
			if err != nil {
				return fmt.Errorf("error updating TargetPools for InstanceGroupManager: %v", err)
			}

			if err := t.Cloud.WaitForOp(op); err != nil {
				return fmt.Errorf("error updating TargetPools for InstanceGroupManager: %v", err)
			}

			changes.TargetPools = nil
		}

		empty := &InstanceGroupManager{}
		if !reflect.DeepEqual(empty, changes) {
			return fmt.Errorf("Cannot apply changes to InstanceGroupManager: %v", changes)
		}
	}

	return nil
}

type terraformInstanceGroupManager struct {
	Name             *string              `json:"name"`
	Zone             *string              `json:"zone"`
	BaseInstanceName *string              `json:"base_instance_name"`
	InstanceTemplate *terraform.Literal   `json:"instance_template"`
	TargetSize       *int64               `json:"target_size"`
	TargetPools      []*terraform.Literal `json:"target_pools,omitempty"`
}

func (_ *InstanceGroupManager) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *InstanceGroupManager) error {
	tf := &terraformInstanceGroupManager{
		Name:             e.Name,
		Zone:             e.Zone,
		BaseInstanceName: e.BaseInstanceName,
		InstanceTemplate: e.InstanceTemplate.TerraformLink(),
		TargetSize:       e.TargetSize,
	}

	for _, targetPool := range e.TargetPools {
		tf.TargetPools = append(tf.TargetPools, targetPool.TerraformLink())
	}

	return t.RenderResource("google_compute_instance_group_manager", *e.Name, tf)
}
