/*
Copyright 2021. Netris, Inc.

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

package controllers

import (
	k8sv1alpha1 "github.com/netrisai/netris-operator/api/v1alpha1"
	"github.com/netrisai/netriswebapi/v1/types/tenant"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TenantToTenantMeta converts the Tenant resource to TenantMeta type and used for add the Tenant for Netris API.
func (r *TenantReconciler) TenantToTenantMeta(tenant *k8sv1alpha1.Tenant) (*k8sv1alpha1.TenantMeta, error) {
	var (
		imported = false
		reclaim  = false
	)

	if i, ok := tenant.GetAnnotations()["resource.k8s.netris.ai/import"]; ok && i == "true" {
		imported = true
	}
	if i, ok := tenant.GetAnnotations()["resource.k8s.netris.ai/reclaimPolicy"]; ok && i == "retain" {
		reclaim = true
	}

	tenantMeta := &k8sv1alpha1.TenantMeta{
		ObjectMeta: metav1.ObjectMeta{
			Name:      string(tenant.GetUID()),
			Namespace: tenant.GetNamespace(),
		},
		TypeMeta: metav1.TypeMeta{},
		Spec: k8sv1alpha1.TenantMetaSpec{
			Imported:    imported,
			Reclaim:     reclaim,
			TenantName:  tenant.Name,
			Name:        tenant.Name,
			Description: tenant.Spec.Description,
		},
	}

	return tenantMeta, nil
}

func tenantCompareFieldsForNewMeta(tenant *k8sv1alpha1.Tenant, tenantMeta *k8sv1alpha1.TenantMeta) bool {
	imported := false
	reclaim := false
	if i, ok := tenant.GetAnnotations()["resource.k8s.netris.ai/import"]; ok && i == "true" {
		imported = true
	}
	if i, ok := tenant.GetAnnotations()["resource.k8s.netris.ai/reclaimPolicy"]; ok && i == "retain" {
		reclaim = true
	}
	return tenant.GetGeneration() != tenantMeta.Spec.TenantCRGeneration || imported != tenantMeta.Spec.Imported || reclaim != tenantMeta.Spec.Reclaim
}

func tenantMustUpdateAnnotations(tenant *k8sv1alpha1.Tenant) bool {
	update := false
	if i, ok := tenant.GetAnnotations()["resource.k8s.netris.ai/import"]; !(ok && (i == "true" || i == "false")) {
		update = true
	}
	if i, ok := tenant.GetAnnotations()["resource.k8s.netris.ai/reclaimPolicy"]; !(ok && (i == "retain" || i == "delete")) {
		update = true
	}
	return update
}

func tenantUpdateDefaultAnnotations(tenant *k8sv1alpha1.Tenant) {
	imported := "false"
	reclaim := "delete"
	if i, ok := tenant.GetAnnotations()["resource.k8s.netris.ai/import"]; ok && i == "true" {
		imported = "true"
	}
	if i, ok := tenant.GetAnnotations()["resource.k8s.netris.ai/reclaimPolicy"]; ok && i == "retain" {
		reclaim = "retain"
	}
	annotations := tenant.GetAnnotations()
	annotations["resource.k8s.netris.ai/import"] = imported
	annotations["resource.k8s.netris.ai/reclaimPolicy"] = reclaim
	tenant.SetAnnotations(annotations)
}

// TenantMetaToNetris converts the k8s Tenant resource to Netris type and used for add the Tenant for Netris API.
func TenantMetaToNetris(tenantMeta *k8sv1alpha1.TenantMeta) (*tenant.Tenant, error) {
	tenantAdd := &tenant.Tenant{
		Name:        tenantMeta.Spec.Name,
		Description: tenantMeta.Spec.Description,
	}

	return tenantAdd, nil
}

// TenantMetaToNetrisUpdate converts the k8s Tenant resource to Netris type and used for update the Tenant for Netris API.
func TenantMetaToNetrisUpdate(tenantMeta *k8sv1alpha1.TenantMeta) (*tenant.Tenant, error) {
	tenantAdd := &tenant.Tenant{
		ID:          tenantMeta.Spec.ID,
		Name:        tenantMeta.Spec.Name,
		Description: tenantMeta.Spec.Description,
	}

	return tenantAdd, nil
}

func compareTenantMetaAPITenant(tenantMeta *k8sv1alpha1.TenantMeta, apiTenant *tenant.Tenant, u uniReconciler) bool {
	if apiTenant.Name != tenantMeta.Spec.Name {
		u.DebugLogger.Info("Name changed", "netrisValue", apiTenant.Name, "k8sValue", tenantMeta.Spec.Name)
		return false
	}
	if apiTenant.Description != tenantMeta.Spec.Description {
		u.DebugLogger.Info("Description changed", "netrisValue", apiTenant.Description, "k8sValue", tenantMeta.Spec.Description)
		return false
	}

	return true
}
