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
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8sv1alpha1 "github.com/netrisai/netris-operator/api/v1alpha1"
	"github.com/netrisai/netris-operator/netrisstorage"
	api "github.com/netrisai/netriswebapi/v2"
)

// TenantReconciler reconciles a Tenant object
type TenantReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Cred     *api.Clientset
	NStorage *netrisstorage.Storage
}

// +kubebuilder:rbac:groups=k8s.netris.ai,resources=tenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8s.netris.ai,resources=tenants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8s.netris.ai,resources=tenants/finalizers,verbs=update

// Reconcile is the main reconciler for the appropriate resource type
func (r *TenantReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("name", req.NamespacedName)
	debugLogger := logger.V(int(zapcore.WarnLevel))
	tenant := &k8sv1alpha1.Tenant{}

	u := uniReconciler{
		Client:      r.Client,
		Logger:      logger,
		DebugLogger: debugLogger,
		Cred:        r.Cred,
		NStorage:    r.NStorage,
	}

	tenantCtx, tenantCancel := context.WithTimeout(cntxt, contextTimeout)
	defer tenantCancel()
	if err := r.Get(tenantCtx, req.NamespacedName, tenant); err != nil {
		if errors.IsNotFound(err) {
			debugLogger.Info(err.Error())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	tenantMetaNamespaced := req.NamespacedName
	tenantMetaNamespaced.Name = string(tenant.GetUID())
	tenantMeta := &k8sv1alpha1.TenantMeta{}
	metaFound := true

	tenantMetaCtx, tenantMetaCancel := context.WithTimeout(cntxt, contextTimeout)
	defer tenantMetaCancel()
	if err := r.Get(tenantMetaCtx, tenantMetaNamespaced, tenantMeta); err != nil {
		if errors.IsNotFound(err) {
			debugLogger.Info(err.Error())
			metaFound = false
			tenantMeta = nil
		} else {
			return ctrl.Result{}, err
		}
	}

	if tenant.DeletionTimestamp != nil {
		logger.Info("Go to delete")
		result, err := r.deleteTenant(tenant, tenantMeta)
		if err != nil {
			logger.Error(fmt.Errorf("{deleteTenant} %s", err), "")
			return u.patchTenantStatus(tenant, "Failure", err.Error())
		}
		if result.IsZero() {
			logger.Info("Tenant deleted")
		}
		return ctrl.Result{}, nil
	}

	if tenantMustUpdateAnnotations(tenant) {
		debugLogger.Info("Setting default annotations")
		tenantUpdateDefaultAnnotations(tenant)
		tenantPatchCtx, tenantPatchCancel := context.WithTimeout(cntxt, contextTimeout)
		defer tenantPatchCancel()
		err := r.Patch(tenantPatchCtx, tenant.DeepCopyObject(), client.Merge, &client.PatchOptions{})
		if err != nil {
			logger.Error(fmt.Errorf("{Patch Tenant default annotations} %s", err), "")
			return ctrl.Result{RequeueAfter: requeueInterval}, nil
		}
		return ctrl.Result{}, nil
	}

	if metaFound {
		debugLogger.Info("Meta found", "type", "Tenant", "name", tenant.GetName())
		if tenantCompareFieldsForNewMeta(tenant, tenantMeta) {
			debugLogger.Info("Generating New Meta")
			tenantID := tenantMeta.Spec.ID
			newTenantMeta, err := r.TenantToTenantMeta(tenant)
			if err != nil {
				logger.Error(fmt.Errorf("{TenantToTenantMeta} %s", err), "")
				return u.patchTenantStatus(tenant, "Failure", err.Error())
			}
			tenantMeta.Spec = newTenantMeta.DeepCopy().Spec
			tenantMeta.Spec.ID = tenantID
			tenantMeta.Spec.TenantCRGeneration = tenant.GetGeneration()

			tenantMetaUpdateCtx, tenantMetaUpdateCancel := context.WithTimeout(cntxt, contextTimeout)
			defer tenantMetaUpdateCancel()
			err = r.Update(tenantMetaUpdateCtx, tenantMeta.DeepCopyObject(), &client.UpdateOptions{})
			if err != nil {
				logger.Error(fmt.Errorf("{tenantMeta Update} %s", err), "")
				return ctrl.Result{RequeueAfter: requeueInterval}, nil
			}
		}
	} else {
		debugLogger.Info("Meta not found", "type", "Tenant", "name", tenant.GetName())
		if tenant.GetFinalizers() == nil {
			tenant.SetFinalizers([]string{"resource.k8s.netris.ai/delete"})

			tenantPatchCtx, tenantPatchCancel := context.WithTimeout(cntxt, contextTimeout)
			defer tenantPatchCancel()
			err := r.Patch(tenantPatchCtx, tenant.DeepCopyObject(), client.Merge, &client.PatchOptions{})
			if err != nil {
				logger.Error(fmt.Errorf("{Patch Tenant Finalizer} %s", err), "")
				return ctrl.Result{RequeueAfter: requeueInterval}, nil
			}
			return ctrl.Result{}, nil
		}

		tenantMeta, err := r.TenantToTenantMeta(tenant)
		if err != nil {
			logger.Error(fmt.Errorf("{TenantToTenantMeta} %s", err), "")
			return u.patchTenantStatus(tenant, "Failure", err.Error())
		}

		tenantMeta.Spec.TenantCRGeneration = tenant.GetGeneration()
		tenantMeta.SetFinalizers([]string{"resource.k8s.netris.ai/delete"})

		tenantMetaCreateCtx, tenantMetaCreateCancel := context.WithTimeout(cntxt, contextTimeout)
		defer tenantMetaCreateCancel()
		if err := r.Create(tenantMetaCreateCtx, tenantMeta.DeepCopyObject(), &client.CreateOptions{}); err != nil {
			logger.Error(fmt.Errorf("{tenantMeta Create} %s", err), "")
			return ctrl.Result{RequeueAfter: requeueInterval}, nil
		}
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *TenantReconciler) deleteTenant(tenant *k8sv1alpha1.Tenant, tenantMeta *k8sv1alpha1.TenantMeta) (ctrl.Result, error) {
	return r.deleteCRs(tenant, tenantMeta)
}

func (r *TenantReconciler) deleteCRs(tenant *k8sv1alpha1.Tenant, tenantMeta *k8sv1alpha1.TenantMeta) (ctrl.Result, error) {
	if tenantMeta != nil {
		_, err := r.deleteTenantMetaCR(tenantMeta)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("{deleteCRs} %s", err)
		}
	} else {
		return r.deleteTenantCR(tenant)
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

func (r *TenantReconciler) deleteTenantCR(tenant *k8sv1alpha1.Tenant) (ctrl.Result, error) {
	tenant.ObjectMeta.SetFinalizers(nil)
	tenant.SetFinalizers(nil)
	ctx, cancel := context.WithTimeout(cntxt, contextTimeout)
	defer cancel()
	if err := r.Update(ctx, tenant.DeepCopyObject(), &client.UpdateOptions{}); err != nil {
		return ctrl.Result{}, fmt.Errorf("{deleteTenantCR} %s", err)
	}

	return ctrl.Result{}, nil
}

func (r *TenantReconciler) deleteTenantMetaCR(tenantMeta *k8sv1alpha1.TenantMeta) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(cntxt, contextTimeout)
	defer cancel()
	if err := r.Delete(ctx, tenantMeta.DeepCopyObject(), &client.DeleteOptions{}); err != nil {
		return ctrl.Result{}, fmt.Errorf("{deleteTenantMetaCR} %s", err)
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// SetupWithManager .
func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8sv1alpha1.Tenant{}).
		Complete(r)
}
