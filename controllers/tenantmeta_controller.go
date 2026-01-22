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
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/netrisai/netriswebapi/http"
	"github.com/netrisai/netriswebapi/v1/types/tenant"
	api "github.com/netrisai/netriswebapi/v2"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8sv1alpha1 "github.com/netrisai/netris-operator/api/v1alpha1"
	"github.com/netrisai/netris-operator/netrisstorage"
)

// TenantMetaReconciler reconciles a TenantMeta object
type TenantMetaReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Cred     *api.Clientset
	NStorage *netrisstorage.Storage
}

// +kubebuilder:rbac:groups=k8s.netris.ai,resources=tenantmeta,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=k8s.netris.ai,resources=tenantmeta/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8s.netris.ai,resources=tenantmeta/finalizers,verbs=update

// Reconcile is the main reconciler for the appropriate resource type
func (r *TenantMetaReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	debugLogger := r.Log.WithValues("name", req.NamespacedName).V(int(zapcore.WarnLevel))

	tenantMeta := &k8sv1alpha1.TenantMeta{}
	tenantCR := &k8sv1alpha1.Tenant{}
	tenantMetaCtx, tenantMetaCancel := context.WithTimeout(cntxt, contextTimeout)
	defer tenantMetaCancel()
	if err := r.Get(tenantMetaCtx, req.NamespacedName, tenantMeta); err != nil {
		if errors.IsNotFound(err) {
			debugLogger.Info(err.Error())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger := r.Log.WithValues("name", fmt.Sprintf("%s/%s", req.NamespacedName.Namespace, tenantMeta.Spec.TenantName))
	debugLogger = logger.V(int(zapcore.WarnLevel))

	if tenantMeta.DeletionTimestamp != nil {
		if tenantMeta.Spec.ID > 0 && !tenantMeta.Spec.Reclaim {
			reply, err := r.Cred.Tenant().Delete(tenantMeta.Spec.ID)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("{deleteTenant} %s", err)
			}
			resp, err := http.ParseAPIResponse(reply.Data)
			if err != nil {
				return ctrl.Result{}, err
			}
			if !resp.IsSuccess && reply.StatusCode != 404 {
				return ctrl.Result{}, fmt.Errorf(resp.Message)
			}
		}

		tenantMeta.SetFinalizers(nil)
		tenantCtx, tenantCancel := context.WithTimeout(cntxt, contextTimeout)
		defer tenantCancel()
		err := r.Update(tenantCtx, tenantMeta.DeepCopyObject(), &client.UpdateOptions{})
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{RequeueAfter: requeueInterval}, fmt.Errorf("{DeleteTenantMetaCR Finalizer} %s", err)
		}

		return ctrl.Result{}, nil
	}

	if tenantMeta.GetFinalizers() == nil {
		tenantMeta.SetFinalizers([]string{"resource.k8s.netris.ai/delete"})
		tenantCtx, tenantCancel := context.WithTimeout(cntxt, contextTimeout)
		defer tenantCancel()
		err := r.Patch(tenantCtx, tenantMeta.DeepCopyObject(), client.Merge, &client.PatchOptions{})
		if err != nil {
			logger.Error(fmt.Errorf("{Patch TenantMeta Finalizer} %s", err), "")
			return ctrl.Result{RequeueAfter: requeueInterval}, nil
		}
	}

	u := uniReconciler{
		Client:      r.Client,
		Logger:      logger,
		DebugLogger: debugLogger,
		Cred:        r.Cred,
		NStorage:    r.NStorage,
	}

	provisionState := "OK"

	tenantNN := req.NamespacedName
	tenantNN.Name = tenantMeta.Spec.TenantName
	tenantNNCtx, tenantNNCancel := context.WithTimeout(cntxt, contextTimeout)
	defer tenantNNCancel()
	if err := r.Get(tenantNNCtx, tenantNN, tenantCR); err != nil {
		if errors.IsNotFound(err) {
			debugLogger.Info(err.Error())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if tenantMeta.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if tenantMeta.Spec.ID == 0 {
		debugLogger.Info("ID Not found in meta", "type", "Tenant", "name", tenantMeta.Spec.TenantName)
		if tenantMeta.Spec.Imported {
			logger.Info("Importing tenant")
			debugLogger.Info("Imported yaml mode. Finding Tenant by name", "type", "Tenant", "name", tenantMeta.Spec.TenantName)
			if tenantItem, ok := r.NStorage.TenantsStorage.FindByName(tenantMeta.Spec.Name); ok {
				debugLogger.Info("Imported yaml mode. Tenant found", "type", "Tenant", "name", tenantMeta.Spec.TenantName)
				tenantMeta.Spec.ID = tenantItem.ID

				tenantMetaPatchCtx, tenantMetaPatchCancel := context.WithTimeout(cntxt, contextTimeout)
				defer tenantMetaPatchCancel()
				err := r.Patch(tenantMetaPatchCtx, tenantMeta.DeepCopyObject(), client.Merge, &client.PatchOptions{})
				if err != nil {
					logger.Error(fmt.Errorf("{patch tenantmeta.Spec.ID} %s", err), "")
					return u.patchTenantStatus(tenantCR, "Failure", err.Error())
				}
				debugLogger.Info("Imported yaml mode. ID patched", "type", "Tenant", "name", tenantMeta.Spec.TenantName)
				logger.Info("Tenant imported")
				return ctrl.Result{RequeueAfter: requeueInterval}, nil
			}
			logger.Info("Tenant not found for import")
			debugLogger.Info("Imported yaml mode. Tenant not found", "type", "Tenant", "name", tenantMeta.Spec.TenantName)
		}

		logger.Info("Creating Tenant")
		if _, err, errMsg := r.createTenant(tenantMeta); err != nil {
			logger.Error(fmt.Errorf("{createTenant} %s", err), "")
			return u.patchTenantStatus(tenantCR, "Failure", errMsg.Error())
		}
		logger.Info("Tenant Created")
	} else {
		if apiTenant, ok := r.NStorage.TenantsStorage.FindByID(tenantMeta.Spec.ID); ok {

			debugLogger.Info("Comparing TenantMeta with Netris Tenant", "type", "Tenant", "name", tenantMeta.Spec.TenantName)
			if ok := compareTenantMetaAPITenant(tenantMeta, apiTenant, u); ok {
				debugLogger.Info("Nothing Changed")
			} else {
				debugLogger.Info("Go to update Tenant in Netris")
				logger.Info("Updating Tenant")
				tenantUpdate, err := TenantMetaToNetrisUpdate(tenantMeta)
				if err != nil {
					logger.Error(fmt.Errorf("{TenantMetaToNetrisUpdate} %s", err), "")
					return u.patchTenantStatus(tenantCR, "Failure", err.Error())
				}

				js, _ := json.Marshal(tenantUpdate)
				debugLogger.Info("tenantUpdate", "payload", string(js))

				_, err, errMsg := updateTenant(tenantMeta.Spec.ID, tenantUpdate, r.Cred)
				if err != nil {
					logger.Error(fmt.Errorf("{updateTenant} %s", err), "")
					return u.patchTenantStatus(tenantCR, "Failure", errMsg.Error())
				}
				logger.Info("Tenant Updated")
			}
		} else {
			debugLogger.Info("Tenant not found in Netris", "type", "Tenant", "name", tenantMeta.Spec.TenantName)
			debugLogger.Info("Going to create Tenant", "type", "Tenant", "name", tenantMeta.Spec.TenantName)
			logger.Info("Creating Tenant")
			if _, err, errMsg := r.createTenant(tenantMeta); err != nil {
				logger.Error(fmt.Errorf("{createTenant} %s", err), "")
				return u.patchTenantStatus(tenantCR, "Failure", errMsg.Error())
			}
			logger.Info("Tenant Created")
		}
	}
	return u.patchTenantStatus(tenantCR, provisionState, "Success")
}

func (r *TenantMetaReconciler) createTenant(tenantMeta *k8sv1alpha1.TenantMeta) (ctrl.Result, error, error) {
	debugLogger := r.Log.WithValues(
		"name", fmt.Sprintf("%s/%s", tenantMeta.Namespace, tenantMeta.Spec.TenantName),
		"tenantName", tenantMeta.Spec.TenantCRGeneration,
	).V(int(zapcore.WarnLevel))

	tenantAdd, err := TenantMetaToNetris(tenantMeta)
	if err != nil {
		return ctrl.Result{}, err, err
	}

	js, _ := json.Marshal(tenantAdd)
	debugLogger.Info("tenantToAdd", "payload", string(js))

	reply, err := r.Cred.Tenant().Add(tenantAdd)
	if err != nil {
		return ctrl.Result{}, err, err
	}
	resp, err := http.ParseAPIResponse(reply.Data)
	if err != nil {
		return ctrl.Result{}, err, err
	}
	if !resp.IsSuccess {
		return ctrl.Result{}, fmt.Errorf(resp.Message), fmt.Errorf(resp.Message)
	}

	idStruct := struct {
		ID int `json:"id"`
	}{}
	debugLogger.Info("response Data", "payload", resp.Data)
	err = http.Decode(resp.Data, &idStruct)
	if err != nil {
		return ctrl.Result{}, err, err
	}

	debugLogger.Info("Tenant Created", "id", idStruct.ID)

	tenantMeta.Spec.ID = idStruct.ID

	ctx, cancel := context.WithTimeout(cntxt, contextTimeout)
	defer cancel()
	err = r.Patch(ctx, tenantMeta.DeepCopyObject(), client.Merge, &client.PatchOptions{}) // requeue
	if err != nil {
		return ctrl.Result{}, err, err
	}

	debugLogger.Info("ID patched to meta", "id", idStruct.ID)
	return ctrl.Result{}, nil, nil
}

func updateTenant(id int, tenant *tenant.Tenant, cred *api.Clientset) (ctrl.Result, error, error) {
	reply, err := cred.Tenant().Update(tenant)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("{updateTenant} %s", err), err
	}
	resp, err := http.ParseAPIResponse(reply.Data)
	if err != nil {
		return ctrl.Result{}, err, err
	}
	if !resp.IsSuccess {
		return ctrl.Result{}, fmt.Errorf("{updateTenant} %s", fmt.Errorf(resp.Message)), fmt.Errorf(resp.Message)
	}

	return ctrl.Result{}, nil, nil
}

// SetupWithManager .
func (r *TenantMetaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8sv1alpha1.TenantMeta{}).
		Complete(r)
}
