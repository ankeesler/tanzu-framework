package main

import (
	"context"
	"log"

	"gopkg.in/yaml.v2"
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
		// TODO: watch secrets so that we can ensure desired state is actual state
		Complete(&pinnipedInfoController{client: manager.GetClient()})

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
	client client.Client
}

func (c *pinnipedInfoController) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	log.Print("something happened to pinniped-info cm")

	// Get pinniped-info ConfigMap
	pinnipedInfoCM := corev1.ConfigMap{}
	if err := c.client.Get(ctx, req.NamespacedName, &pinnipedInfoCM); err != nil {
		panic(err) // TODO: handle me
	}

	// Get Pinniped Supervisor info
	supervisorAddress, ok := pinnipedInfoCM.Data["issuer"] // TODO: get rid of raw strings...
	if !ok {
		panic("couldn't find issuer") // TODO: handle me
	}
	supervisorCABundle, ok := pinnipedInfoCM.Data["issuer_ca_bundle_data"] // TODO: get rid of raw strings...
	if !ok {
		panic("couldn't find ca bundle") // TODO: handle me
	}
	log.Printf("supervisorAddress: %q, supervisorCABundle: %q", supervisorAddress, supervisorCABundle)

	// Get all addon Secret's
	addonSecrets := &corev1.SecretList{}
	addonSecretLabel := client.MatchingLabels{"tkg.tanzu.vmware.com/addon-name": "pinniped"} // TODO: get rid of raw strings...
	if err := c.client.List(ctx, addonSecrets, addonSecretLabel); err != nil {
		panic(err) // TODO: handle me
	}

	// Loop through addon secrets and update pinniped.supervisor_svc_endpoint and
	// supervisor_ca_bundle_data
	for _, addonSecret := range addonSecrets.Items {
		if err := c.updateSecret(ctx, &addonSecret, supervisorAddress, supervisorCABundle); err != nil {
			panic(err) // TODO: handle me
		}
	}

	// TODO: ...and see if workload cluster gets configured correctly :)

	// TODO: handle case where addon secret exists
	// TODO: handle case where addon secret does not exist

	// TODO: don't send a request if the addon secret is already up to date

	return reconcile.Result{}, nil
}

func (c *pinnipedInfoController) updateSecret(
	ctx context.Context,
	addonSecret *corev1.Secret,
	supervisorAddress, supervisorCABundle string,
) error {
	valuesYAML, ok := addonSecret.Data["values.yaml"] // TODO: get rid of raw strings...
	if !ok {
		panic("could not find data values") // TODO: handle me
	}

	log.Printf("addonSecret: %q, valuesYAML: %q", addonSecret.Name, string(valuesYAML))

	// TODO: best way to set these fields? Merge these values back into values.yaml? Use raw map?

	values := struct {
		Pinniped struct {
			SupervisorSvcEndpoint  string `yaml:"supervisor_svc_endpoint"`
			SupervisorCABundleData string `yaml:"supervisor_ca_bundle_data"`
		} `yaml:"pinniped"`
	}{}
	if err := yaml.Unmarshal(valuesYAML, &values); err != nil {
		panic("could not unmarshal values.yaml") // TODO: handle me
	}

	values.Pinniped.SupervisorSvcEndpoint = supervisorAddress
	values.Pinniped.SupervisorCABundleData = supervisorCABundle

	log.Printf("values: %#v", values)

	return nil
}
