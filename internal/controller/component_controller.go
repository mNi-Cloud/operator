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

package controller

import (
	"context"
	errors2 "errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
	"net/http"
	"slices"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/mNi-Cloud/operator/api/v1alpha1"
)

// ComponentReconciler reconciles a Component object
type ComponentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type ComponentSource struct {
	URL string
}

const (
	indexFile    = "https://raw.githubusercontent.com/mNi-Cloud/operator/refs/heads/main/manifests/components/index.yaml"
	manifestFile = "https://raw.githubusercontent.com/mNi-Cloud/operator/refs/heads/main/manifests/components/%s/%s.yaml"
)

// +kubebuilder:rbac:groups=operator.mnicloud.jp,resources=components,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.mnicloud.jp,resources=components/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.mnicloud.jp,resources=components/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Component object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ComponentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var component operatorv1alpha1.Component
	if err := r.Get(ctx, req.NamespacedName, &component); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to get Component", "name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	if !component.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	httpClient := new(http.Client)
	index, err := r.requestIndex(ctx, httpClient)
	if err != nil {
		logger.Error(err, "unable to get index file")
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&component.Status.Conditions, metav1.Condition{
		Type:   operatorv1alpha1.TypeComponentAvailable,
		Status: metav1.ConditionTrue,
		Reason: "OK",
	})
	meta.SetStatusCondition(&component.Status.Conditions, metav1.Condition{
		Type:   operatorv1alpha1.TypeComponentDegraded,
		Status: metav1.ConditionFalse,
		Reason: "OK",
	})

	var manifestUrl string
	if versions, found := index[component.Name]; found {
		if slices.Contains(versions, component.Spec.AppVersion) {
			manifestUrl = fmt.Sprintf(manifestFile, component.Name, component.Spec.AppVersion)
		} else {
			meta.SetStatusCondition(&component.Status.Conditions, metav1.Condition{
				Type:    operatorv1alpha1.TypeComponentDegraded,
				Status:  metav1.ConditionFalse,
				Reason:  "NotFound",
				Message: fmt.Sprintf("Version %s for component %s not found", component.Spec.AppVersion, component.Name),
			})
			meta.SetStatusCondition(&component.Status.Conditions, metav1.Condition{
				Type:   operatorv1alpha1.TypeComponentAvailable,
				Status: metav1.ConditionFalse,
				Reason: "NotFound",
			})
		}
	} else {
		meta.SetStatusCondition(&component.Status.Conditions, metav1.Condition{
			Type:    operatorv1alpha1.TypeComponentDegraded,
			Status:  metav1.ConditionFalse,
			Reason:  "NotFound",
			Message: fmt.Sprintf("Component %s not found", component.Name),
		})
		meta.SetStatusCondition(&component.Status.Conditions, metav1.Condition{
			Type:   operatorv1alpha1.TypeComponentAvailable,
			Status: metav1.ConditionFalse,
			Reason: "NotFound",
		})
	}

	result := ctrl.Result{}

	if manifestUrl != "" {
		reader, err := r.requestManifests(ctx, httpClient, manifestUrl)
		if err != nil {
			logger.Error(err, "unable to get manifest file")
			return ctrl.Result{}, err
		}
		defer reader.Close()
		decoder := yaml.NewDecoder(reader)
		failed := false
		for {
			obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
			err := decoder.Decode(obj.Object)
			if errors2.Is(err, io.EOF) {
				break
			}
			if err != nil {
				logger.Error(err, "unable to decode manifest file")
				return ctrl.Result{}, err
			}
			if obj.Object == nil {
				logger.Info("skipping empty document")
				continue
			}

			err = ctrl.SetControllerReference(&component, obj, r.Scheme)
			if err != nil {
				logger.Error(err, "unable to set controller reference", "name", component.Name, "manifest-kind", obj.GroupVersionKind().String(), "manifest", obj.GetName())
				failed = true
				continue
			}

			err = r.Patch(ctx, obj, client.Apply, &client.PatchOptions{
				FieldManager: "mni-operator",
				Force:        pointer.Bool(true),
			})
			if err != nil {
				logger.Error(err, "unable to apply manifest file", "name", component.Name, "manifest-kind", obj.GroupVersionKind().String(), "manifest", obj.GetName())
				failed = true
				continue
			}
			logger.Info("applied manifest file", "name", component.Name, "manifest-kind", obj.GroupVersionKind().String(), "manifest", obj.GetName())
		}

		if failed {
			meta.SetStatusCondition(&component.Status.Conditions, metav1.Condition{
				Type:    operatorv1alpha1.TypeComponentAvailable,
				Status:  metav1.ConditionFalse,
				Reason:  "ApplyFailed",
				Message: "Some manifests failed to apply",
			})
		}
	}

	err = r.Status().Update(ctx, &component)
	return result, err
}

func (r *ComponentReconciler) requestIndex(ctx context.Context, httpClient *http.Client) (map[string][]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", indexFile, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to request: %s", resp.Status)
	}

	byteArray, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res map[string][]string
	err = yaml.Unmarshal(byteArray, &res)
	return res, err
}

func (r *ComponentReconciler) requestManifests(ctx context.Context, httpClient *http.Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to request: %s", resp.Status)
	}

	return resp.Body, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComponentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.Component{}).
		Named("component").
		Complete(r)
}
