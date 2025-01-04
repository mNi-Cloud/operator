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

	"github.com/mNi-Cloud/operator/internal/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1apply "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"
	networkingv1apply "k8s.io/client-go/applyconfigurations/networking/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1alpha1 "github.com/mNi-Cloud/operator/api/v1alpha1"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config config.OperatorConfig
}

// +kubebuilder:rbac:groups=operator.mnicloud.jp,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.mnicloud.jp,resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.mnicloud.jp,resources=services/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Service object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var svc operatorv1alpha1.Service
	if err := r.Get(ctx, req.NamespacedName, &svc); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to get Service", "name", req.NamespacedName)
		return ctrl.Result{}, err
	}

	if !svc.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if err := r.reconcileDeployment(ctx, svc); err != nil {
		result, err2 := r.updateStatus(ctx, svc)
		logger.Error(err2, "unable to update status")
		return result, err
	}

	if err := r.reconcileService(ctx, svc); err != nil {
		result, err2 := r.updateStatus(ctx, svc)
		logger.Error(err2, "unable to update status")
		return result, err
	}

	if svc.Spec.External == operatorv1alpha1.ExternalIngress {
		if err := r.reconcileIngress(ctx, svc); err != nil {
			result, err2 := r.updateStatus(ctx, svc)
			logger.Error(err2, "unable to update status")
			return result, err
		}
	}

	return r.updateStatus(ctx, svc)
}

func (r *ServiceReconciler) reconcileDeployment(ctx context.Context, svc operatorv1alpha1.Service) error {
	logger := log.FromContext(ctx)

	var imagePullSecrets []*corev1apply.LocalObjectReferenceApplyConfiguration
	for _, secret := range svc.Spec.ImagePullSecrets {
		imagePullSecrets = append(imagePullSecrets, corev1apply.LocalObjectReference().WithName(secret.Name))
	}

	owner, err := controllerReference(svc, r.Scheme)
	if err != nil {
		return err
	}

	dep := appsv1apply.Deployment(svc.Name, r.Config.Namespace).
		WithLabels(map[string]string{
			"app.kubernetes.io/name":       "mni-service",
			"app.kubernetes.io/instance":   svc.Name,
			"app.kubernetes.io/created-by": "mni-operator",
		}).
		WithOwnerReferences(owner).
		WithSpec(appsv1apply.DeploymentSpec().
			WithReplicas(1).
			WithSelector(metav1apply.LabelSelector().WithMatchLabels(map[string]string{
				"app.kubernetes.io/name":       "mni-service",
				"app.kubernetes.io/instance":   svc.Name,
				"app.kubernetes.io/created-by": "mni-operator",
			})).
			WithTemplate(corev1apply.PodTemplateSpec().
				WithLabels(map[string]string{
					"app.kubernetes.io/name":       "mni-service",
					"app.kubernetes.io/instance":   svc.Name,
					"app.kubernetes.io/created-by": "mni-operator",
				}).
				WithSpec(corev1apply.PodSpec().
					WithContainers(corev1apply.Container().
						WithName(svc.Name).
						WithImage(svc.Spec.Image).
						WithImagePullPolicy(corev1.PullAlways).
						WithArgs(svc.Spec.Args...).
						WithEnv(
							corev1apply.EnvVar().
								WithName("SERVICE_USERNAME").
								WithValueFrom(corev1apply.EnvVarSource().WithSecretKeyRef(
									corev1apply.SecretKeySelector().WithName(svc.Spec.ServiceSecretRef).WithKey("username"))),
							corev1apply.EnvVar().
								WithName("SERVICE_PASSWORD").
								WithValueFrom(corev1apply.EnvVarSource().WithSecretKeyRef(
									corev1apply.SecretKeySelector().WithName(svc.Spec.ServiceSecretRef).WithKey("password"))),
						).
						WithPorts(corev1apply.ContainerPort().
							WithProtocol(corev1.ProtocolTCP).
							WithContainerPort(8080),
						),
					).
					WithImagePullSecrets(imagePullSecrets...).
					WithServiceAccountName("mni-cloud"),
				),
			),
		)

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep)
	if err != nil {
		return err
	}
	patch := &unstructured.Unstructured{
		Object: obj,
	}

	var current appsv1.Deployment
	err = r.Get(ctx, client.ObjectKey{Namespace: r.Config.Namespace, Name: svc.Name}, &current)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	currApplyConfig, err := appsv1apply.ExtractDeployment(&current, "mni-operator")
	if err != nil {
		return err
	}

	if equality.Semantic.DeepEqual(dep, currApplyConfig) {
		return nil
	}

	err = r.Patch(ctx, patch, client.Apply, &client.PatchOptions{
		FieldManager: "mni-operator",
		Force:        pointer.Bool(true),
	})

	if err != nil {
		logger.Error(err, "unable to create or update Deployment")
		return err
	}
	logger.Info("reconcile Deployment successfully", "name", svc.Name)
	return nil
}

func (r *ServiceReconciler) reconcileService(ctx context.Context, svc operatorv1alpha1.Service) error {
	logger := log.FromContext(ctx)

	owner, err := controllerReference(svc, r.Scheme)
	if err != nil {
		return err
	}

	svcType := corev1.ServiceTypeClusterIP
	if svc.Spec.External == operatorv1alpha1.ExternalLoadBalancer {
		svcType = corev1.ServiceTypeLoadBalancer
	}

	service := corev1apply.Service(svc.Name, r.Config.Namespace).
		WithLabels(map[string]string{
			"app.kubernetes.io/name":       "mni-service",
			"app.kubernetes.io/instance":   svc.Name,
			"app.kubernetes.io/created-by": "mni-operator",
		}).
		WithOwnerReferences(owner).
		WithSpec(corev1apply.ServiceSpec().
			WithSelector(map[string]string{
				"app.kubernetes.io/name":       "mni-service",
				"app.kubernetes.io/instance":   svc.Name,
				"app.kubernetes.io/created-by": "mni-operator",
			}).
			WithType(svcType).
			WithPorts(corev1apply.ServicePort().
				WithProtocol(corev1.ProtocolTCP).
				WithPort(80).
				WithTargetPort(intstr.FromInt32(8080)),
			),
		)

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(service)
	if err != nil {
		return err
	}
	patch := &unstructured.Unstructured{
		Object: obj,
	}

	var current corev1.Service
	err = r.Get(ctx, client.ObjectKey{Namespace: svc.Namespace, Name: svc.Name}, &current)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	currApplyConfig, err := corev1apply.ExtractService(&current, "mni-operator")
	if err != nil {
		return err
	}

	if equality.Semantic.DeepEqual(service, currApplyConfig) {
		return nil
	}

	err = r.Patch(ctx, patch, client.Apply, &client.PatchOptions{
		FieldManager: "mni-operator",
		Force:        pointer.Bool(true),
	})
	if err != nil {
		logger.Error(err, "unable to create or update Service")
		return err
	}

	logger.Info("reconcile Service successfully", "name", svc.Name)
	return nil
}

func (r *ServiceReconciler) reconcileIngress(ctx context.Context, svc operatorv1alpha1.Service) error {
	logger := log.FromContext(ctx)

	owner, err := controllerReference(svc, r.Scheme)
	if err != nil {
		return err
	}

	ingress := networkingv1apply.Ingress(svc.Name, r.Config.Namespace).
		WithLabels(map[string]string{
			"app.kubernetes.io/name":       "mni-service",
			"app.kubernetes.io/instance":   svc.Name,
			"app.kubernetes.io/created-by": "mni-operator",
		}).
		WithAnnotations(map[string]string{
			"nginx.ingress.kubernetes.io/rewrite-target": "/$1",
			"nginx.ingress.kubernetes.io/use-regex":      "true",
		}).
		WithOwnerReferences(owner).
		WithSpec(
			networkingv1apply.IngressSpec().
				WithRules(networkingv1apply.IngressRule().
					WithHost("api." + r.Config.CustomDomain).
					WithHTTP(networkingv1apply.HTTPIngressRuleValue().
						WithPaths(networkingv1apply.HTTPIngressPath().
							WithPath("/" + svc.Name + "(.*)").
							WithPathType(networkingv1.PathTypeImplementationSpecific).
							WithBackend(networkingv1apply.IngressBackend().
								WithService(networkingv1apply.IngressServiceBackend().
									WithName(svc.Name).
									WithPort(networkingv1apply.ServiceBackendPort().WithNumber(80)),
								),
							),
						),
					),
				).
				WithTLS(networkingv1apply.IngressTLS().
					WithHosts("api." + r.Config.CustomDomain).
					WithSecretName("api-tls"),
				),
		)

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ingress)
	if err != nil {
		return err
	}
	patch := &unstructured.Unstructured{
		Object: obj,
	}

	var current networkingv1.Ingress
	err = r.Get(ctx, client.ObjectKey{Namespace: svc.Namespace, Name: svc.Name}, &current)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	currApplyConfig, err := networkingv1apply.ExtractIngress(&current, "mni-operator")
	if err != nil {
		return err
	}

	if equality.Semantic.DeepEqual(ingress, currApplyConfig) {
		return nil
	}

	err = r.Patch(ctx, patch, client.Apply, &client.PatchOptions{
		FieldManager: "mni-operator",
		Force:        pointer.Bool(true),
	})
	if err != nil {
		logger.Error(err, "unable to create or update Ingress")
		return err
	}

	logger.Info("reconcile Ingress successfully", "name", svc.Name)
	return nil
}

func (r *ServiceReconciler) updateStatus(ctx context.Context, svc operatorv1alpha1.Service) (ctrl.Result, error) {
	meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
		Type:   operatorv1alpha1.TypeServiceAvailable,
		Status: metav1.ConditionTrue,
		Reason: "OK",
	})
	meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
		Type:   operatorv1alpha1.TypeServiceDegraded,
		Status: metav1.ConditionFalse,
		Reason: "OK",
	})

	var service corev1.Service
	if err := r.Get(ctx, client.ObjectKey{Namespace: r.Config.Namespace, Name: svc.Name}, &service); err != nil {
		if errors.IsNotFound(err) {
			meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
				Type:    operatorv1alpha1.TypeServiceDegraded,
				Status:  metav1.ConditionTrue,
				Reason:  "Reconciling",
				Message: "Service not found",
			})
			meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
				Type:   operatorv1alpha1.TypeServiceAvailable,
				Status: metav1.ConditionFalse,
				Reason: "Reconciling",
			})
		} else {
			return ctrl.Result{}, err
		}
	}

	var dep appsv1.Deployment
	if err := r.Get(ctx, client.ObjectKey{Namespace: r.Config.Namespace, Name: svc.Name}, &dep); err != nil {
		if errors.IsNotFound(err) {
			meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
				Type:    operatorv1alpha1.TypeServiceDegraded,
				Status:  metav1.ConditionTrue,
				Reason:  "Reconciling",
				Message: "Deployment not found",
			})
			meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
				Type:   operatorv1alpha1.TypeServiceAvailable,
				Status: metav1.ConditionFalse,
				Reason: "Reconciling",
			})
		} else {
			return ctrl.Result{}, err
		}
	}

	result := ctrl.Result{}
	if dep.Status.AvailableReplicas == 0 {
		meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
			Type:    operatorv1alpha1.TypeServiceAvailable,
			Status:  metav1.ConditionFalse,
			Reason:  "Unavailable",
			Message: "AvailableReplicas is 0",
		})
		result = ctrl.Result{Requeue: true}
	}

	if service.Spec.ClusterIP == "" {
		meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
			Type:    operatorv1alpha1.TypeServiceAvailable,
			Status:  metav1.ConditionFalse,
			Reason:  "Unavailable",
			Message: "ClusterIP is not assigned",
		})
		result = ctrl.Result{Requeue: true}
	} else {
		svc.Status.Endpoints.Internal = "http://" + svc.Name + "." + r.Config.Namespace + ".svc"
	}

	if svc.Spec.External == operatorv1alpha1.ExternalLoadBalancer {
		if service.Status.LoadBalancer.Ingress == nil || len(service.Status.LoadBalancer.Ingress) <= 0 {
			meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
				Type:    operatorv1alpha1.TypeServiceAvailable,
				Status:  metav1.ConditionFalse,
				Reason:  "Unavailable",
				Message: "LoadBalancerIP is not assigned",
			})
		} else {
			svc.Status.Endpoints.External = "http://" + service.Status.LoadBalancer.Ingress[0].IP
		}
	} else if svc.Spec.External == operatorv1alpha1.ExternalIngress {
		var ingress networkingv1.Ingress
		if err := r.Get(ctx, client.ObjectKey{Namespace: r.Config.Namespace, Name: svc.Name}, &ingress); err != nil {
			if errors.IsNotFound(err) {
				meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
					Type:    operatorv1alpha1.TypeServiceDegraded,
					Status:  metav1.ConditionTrue,
					Reason:  "Reconciling",
					Message: "Ingress not found",
				})
				meta.SetStatusCondition(&svc.Status.Conditions, metav1.Condition{
					Type:   operatorv1alpha1.TypeServiceAvailable,
					Status: metav1.ConditionFalse,
					Reason: "Reconciling",
				})
			} else {
				return ctrl.Result{}, err
			}
		} else {
			svc.Status.Endpoints.External = "https://api." + r.Config.CustomDomain + "/" + svc.Name
		}
	}

	err := r.Status().Update(ctx, &svc)
	return result, err
}

func controllerReference(svc operatorv1alpha1.Service, scheme *runtime.Scheme) (*metav1apply.OwnerReferenceApplyConfiguration, error) {
	gvk, err := apiutil.GVKForObject(&svc, scheme)
	if err != nil {
		return nil, err
	}
	ref := metav1apply.OwnerReference().
		WithAPIVersion(gvk.GroupVersion().String()).
		WithKind(gvk.Kind).
		WithName(svc.Name).
		WithUID(svc.GetUID()).
		WithBlockOwnerDeletion(true).
		WithController(true)
	return ref, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.Service{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Named("service").
		Complete(r)
}
