// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validator

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"

	"github.com/gardener/gardener-extension-provider-vsphere/pkg/apis/vsphere"
	"github.com/gardener/gardener-extension-provider-vsphere/pkg/apis/vsphere/helper"
	vspherevalidation "github.com/gardener/gardener-extension-provider-vsphere/pkg/apis/vsphere/validation"
)

type validationContext struct {
	shoot       *core.Shoot
	infraConfig *vsphere.InfrastructureConfig
	cpConfig    *vsphere.ControlPlaneConfig
}

var (
	specPath           = field.NewPath("spec")
	providerConfigPath = specPath.Child("providerConfig")
	nwPath             = specPath.Child("networking")
	providerPath       = specPath.Child("provider")
	infraConfigPath    = providerPath.Child("infrastructureConfig")
	cpConfigPath       = providerPath.Child("controlPlaneConfig")
	workersPath        = providerPath.Child("workers")
)

func (v *Shoot) validateShootCreation(ctx context.Context, shoot *core.Shoot) error {
	valContext, err := newValidationContext(v.decoder, shoot)
	if err != nil {
		return err
	}

	if err := v.validateInfraAgainstCloudProfile(ctx, shoot, valContext.infraConfig, infraConfigPath); err != nil {
		return err
	}
	if err := v.validateCpAgainstCloudProfile(ctx, shoot, valContext.cpConfig, cpConfigPath); err != nil {
		return err
	}

	return v.validateShoot(ctx, valContext)
}

func (v *Shoot) validateShootUpdate(ctx context.Context, oldShoot, shoot *core.Shoot) error {
	oldValContext, err := newValidationContext(v.decoder, oldShoot)
	if err != nil {
		return err
	}

	valContext, err := newValidationContext(v.decoder, shoot)
	if err != nil {
		return err
	}

	if errList := vspherevalidation.ValidateInfrastructureConfigUpdate(oldValContext.infraConfig, valContext.infraConfig, infraConfigPath); len(errList) != 0 {
		return errList.ToAggregate()
	}

	if errList := vspherevalidation.ValidateControlPlaneConfigUpdate(oldValContext.cpConfig, valContext.cpConfig, cpConfigPath); len(errList) != 0 {
		return errList.ToAggregate()
	}

	if errList := vspherevalidation.ValidateWorkersUpdate(oldShoot.Spec.Provider.Workers, shoot.Spec.Provider.Workers, workersPath); len(errList) > 0 {
		return errList.ToAggregate()
	}

	return v.validateShoot(ctx, valContext)
}

func (v *Shoot) validateShoot(ctx context.Context, context *validationContext) error {
	if errList := vspherevalidation.ValidateNetworking(context.shoot.Spec.Networking, nwPath); len(errList) != 0 {
		return errList.ToAggregate()
	}

	if errList := vspherevalidation.ValidateInfrastructureConfig(context.infraConfig, context.shoot.Spec.Networking.Nodes, infraConfigPath); len(errList) != 0 {
		return errList.ToAggregate()
	}

	if errList := vspherevalidation.ValidateControlPlaneConfig(context.cpConfig, cpConfigPath); len(errList) != 0 {
		return errList.ToAggregate()
	}

	if errList := vspherevalidation.ValidateWorkers(context.shoot.Spec.Provider.Workers, workersPath); len(errList) != 0 {
		return errList.ToAggregate()
	}

	return nil
}

func (v *Shoot) validateInfraAgainstCloudProfile(ctx context.Context, shoot *core.Shoot, infraConfig *vsphere.InfrastructureConfig, fldPath *field.Path) error {
	cloudProfile := &gardencorev1beta1.CloudProfile{}
	if err := v.client.Get(ctx, kutil.Key(shoot.Spec.CloudProfileName), cloudProfile); err != nil {
		return err
	}

	if cloudProfile.Spec.ProviderConfig == nil {
		return fmt.Errorf("providerConfig is not given for cloud profile %q", cloudProfile.Name)
	}
	cloudProfileConfig, err := helper.DecodeCloudProfileConfig(cloudProfile.Spec.ProviderConfig, providerConfigPath)
	if err != nil {
		return fmt.Errorf("an error occurred while reading the cloud profile %q: %v", cloudProfile.Name, err)
	}

	if errList := vspherevalidation.ValidateInfrastructureConfigAgainstCloudProfile(infraConfig, shoot.Spec.Region, cloudProfileConfig, fldPath); len(errList) != 0 {
		return errList.ToAggregate()
	}

	return nil
}

func (v *Shoot) validateCpAgainstCloudProfile(ctx context.Context, shoot *core.Shoot, cpConfig *vsphere.ControlPlaneConfig, fldPath *field.Path) error {
	cloudProfile := &gardencorev1beta1.CloudProfile{}
	if err := v.client.Get(ctx, kutil.Key(shoot.Spec.CloudProfileName), cloudProfile); err != nil {
		return err
	}

	if cloudProfile.Spec.ProviderConfig == nil {
		return fmt.Errorf("providerConfig is not given for cloud profile %q", cloudProfile.Name)
	}
	cloudProfileConfig, err := helper.DecodeCloudProfileConfig(cloudProfile.Spec.ProviderConfig, providerConfigPath)
	if err != nil {
		return fmt.Errorf("an error occurred while reading the cloud profile %q: %v", cloudProfile.Name, err)
	}

	if errList := vspherevalidation.ValidateControlPlaneConfigAgainstCloudProfile(cpConfig, shoot.Spec.Region, cloudProfile, cloudProfileConfig, fldPath); len(errList) != 0 {
		return errList.ToAggregate()
	}

	return nil
}

func newValidationContext(decoder runtime.Decoder, shoot *core.Shoot) (*validationContext, error) {
	if shoot.Spec.Provider.InfrastructureConfig == nil {
		return nil, field.Required(infraConfigPath, "infrastructureConfig must be set for OpenStack shoots")
	}
	infraConfig, err := helper.DecodeInfrastructureConfig(shoot.Spec.Provider.InfrastructureConfig, infraConfigPath)
	if err != nil {
		return nil, err
	}

	if shoot.Spec.Provider.ControlPlaneConfig == nil {
		return nil, field.Required(cpConfigPath, "controlPlaneConfig must be set for OpenStack shoots")
	}
	cpConfig, err := helper.DecodeControlPlaneConfig(shoot.Spec.Provider.ControlPlaneConfig, cpConfigPath)
	if err != nil {
		return nil, err
	}

	return &validationContext{
		shoot:       shoot,
		infraConfig: infraConfig,
		cpConfig:    cpConfig,
	}, nil
}
