package main

import (
	"context"
	"log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TODO: use tanzu logging solution (from controller-runtime?)

func main() {
	log.Print("starting")

	manager, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		// TODO: do we want to set any of these options (e.g., webhook port, leader election)?
	})
	if err != nil {
		panic(err) // TODO: handle me
	}

	ctrl.
		NewControllerManagedBy(manager).
		For(
			&corev1.ConfigMap{},
			withNamespacedName(types.NamespacedName{Namespace: "kube-public", Name: "pinniped-info"}),
		).
		Complete(&pinnipedInfoController{})

	if err := manager.Start(ctrl.SetupSignalHandler()); err != nil {
		panic(err) // TODO: handle me
	}
}

func withNamespacedName(namespacedName types.NamespacedName) builder.Predicates {
	isNamespacedName := func(o client.Object) bool {
		return o.GetNamespace() == namespacedName.Namespace && o.GetName() == namespacedName.Name
	}
	return builder.WithPredicates(
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return isNamespacedName(e.Object) },
			UpdateFunc: func(e event.UpdateEvent) bool {
				return isNamespacedName(e.ObjectOld) || isNamespacedName(e.ObjectNew)
			},
			DeleteFunc:  func(e event.DeleteEvent) bool { return isNamespacedName(e.Object) },
			GenericFunc: func(e event.GenericEvent) bool { return isNamespacedName(e.Object) },
		},
	)
}

type pinnipedInfoController struct {
}

func (c *pinnipedInfoController) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	log.Print("something happened to pinniped-info cm")

	// TODO: loop through addon secrets and update pinniped.supervisor_svc_endpoint and supervisor_ca_bundle_data
	// ...and see if workload cluster gets configured correctly :)

	return reconcile.Result{}, nil
}
