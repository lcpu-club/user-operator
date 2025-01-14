package controller

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	useroperatorv1alpha1 "github.com/lcpu-club/user-operator/api/v1alpha1"
)

// UserReconciler reconciles a User object
type UserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=user-operator.lcpu.dev,resources=users,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=user-operator.lcpu.dev,resources=users/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=user-operator.lcpu.dev,resources=users/finalizers,verbs=update

// TODO: More precise permission control
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the User object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// TODO(user): your logic here
	configs := &useroperatorv1alpha1.UserCreationConfigList{}
	err := r.Client.List(ctx, configs)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Sort by creation date & Combine all configs
	slices.SortFunc(configs.Items, func(a, b useroperatorv1alpha1.UserCreationConfig) int {
		return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
	})
	combinedConfig := &useroperatorv1alpha1.UserCreationConfigSpec{}
	for _, config := range configs.Items {
		if !config.Spec.Enabled {
			continue
		}

		// TODO: determine should be += or =
		combinedConfig.NamespacePrefix += config.Spec.NamespacePrefix
		combinedConfig.Resources = append(combinedConfig.Resources, config.Spec.Resources...)
	}

	user := &useroperatorv1alpha1.User{}
	err = r.Client.Get(ctx, req.NamespacedName, user)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if user.Name == "" || user.Spec.Username == "" {
		log.Info("User is not ready", "user", user)
		return ctrl.Result{}, nil
	}

	myFinalizerName := "user-operator.lcpu.dev/user-finalizer"
	if user.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(user, myFinalizerName) {
			controllerutil.AddFinalizer(user, myFinalizerName)
			if err := r.Update(ctx, user); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if controllerutil.ContainsFinalizer(user, myFinalizerName) {
			// Run finalization logic for user
			log.Info("Finalizing user", "user", user)

			// Delete the namespace for the user
			nsName := fmt.Sprintf("%s%s", combinedConfig.NamespacePrefix, user.Spec.Username)
			ns := &corev1.Namespace{}
			err = r.Client.Get(ctx, types.NamespacedName{Name: nsName}, ns)
			if err == nil {
				err = r.Client.Delete(ctx, ns)
				if client.IgnoreNotFound(err) != nil {
					return ctrl.Result{}, err
				}
			}

			// Remove finalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(user, myFinalizerName)
			if err := r.Update(ctx, user); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure namespace's presence
	nsName := fmt.Sprintf("%s%s", combinedConfig.NamespacePrefix, user.Spec.Username)
	ns := &corev1.Namespace{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: nsName}, ns)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
			},
		}
		err = r.Client.Create(ctx, ns)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Apply all resources
	for _, resource := range combinedConfig.Resources {
		tpl, err := template.New("_cr").Parse(resource)
		if err != nil {
			return ctrl.Result{}, err
		}

		buf := bytes.NewBuffer(nil)
		if err := tpl.Execute(buf, map[string]interface{}{
			"Namespace": nsName,
			"Username":  user.Spec.Username,
			"UID":       user.Spec.UID,
			"Groups":    user.Spec.Groups,
			"Extra":     user.Spec.Extra,
		}); err != nil {
			return ctrl.Result{}, err
		}

		obj := unstructured.Unstructured{}
		_, gvk, err := scheme.Codecs.UniversalDeserializer().Decode(buf.Bytes(), nil, &obj)
		if err != nil {
			return ctrl.Result{}, err
		}

		obj.SetNamespace(nsName)
		obj.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: useroperatorv1alpha1.GroupVersion.String(),
				Kind:       "User",
				Name:       user.Name,
				UID:        user.UID,
			},
		})

		fieldOwner := "lcpu-user-operator-user-controller"

		if err := r.Patch(ctx, &obj, client.Apply, client.ForceOwnership,
			client.FieldOwner(fieldOwner)); err != nil {
			log.Error(err, "unable to apply YAML")
			// TODO: Better processing, now ignores error and steps forward
			continue
		}

		log.Info("Successfully applied YAML", "GVK", gvk)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&useroperatorv1alpha1.User{}).
		Named("user").
		Watches(
			&useroperatorv1alpha1.UserCreationConfig{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				reqs := []reconcile.Request{}
				users := &useroperatorv1alpha1.UserList{}
				err := r.Client.List(ctx, users)
				if err != nil {
					log.FromContext(ctx).Error(err, "unable to list users")
					return reqs
				}

				for _, user := range users.Items {
					reqs = append(reqs, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      user.Name,
							Namespace: user.Namespace,
						},
					})
				}

				return reqs
			}),
		).
		Complete(r)
}
