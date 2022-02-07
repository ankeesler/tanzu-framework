package main

import (
	"bytes"
	"context"
	"fmt"
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

// TODO: any other t-f controller conventions that we aren't following that we should follow?
// TODO: use tanzu logging solution (from controller-runtime?)
// TODO: provide a way to pause the controller
// TODO: pass pinniped-info ConfigMap namespace and name via command line flags
// TODO: why does the post-deploy job run again after we edit the pinniped-info cm?
// TODO: why do the pinniped-supervisor pods seem to restart every time we update the pinniped-info cm?

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

var reconcileCallCount = 0

func (c *pinnipedInfoController) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	// TODO: let's be aware of how often our controller is running since we are watching secrets (of which there are many)
	log.Printf("Reconcile() call count: %d", reconcileCallCount)
	reconcileCallCount++

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
	log.Printf("supervisorAddress: %q, supervisorCABundle: %d chars", supervisorAddress, len(supervisorCABundle))

	// Get all addon Secret's
	addonSecrets := &corev1.SecretList{}
	addonSecretLabel := client.MatchingLabels{"tkg.tanzu.vmware.com/addon-name": "pinniped"} // TODO: get rid of raw strings...
	if err := c.client.List(ctx, addonSecrets, addonSecretLabel); err != nil {
		panic(err) // TODO: handle me
	}

	// Loop through addon secrets and update pinniped.supervisor_svc_endpoint and
	// supervisor_ca_bundle_data
	for _, addonSecret := range addonSecrets.Items {
		// TODO: do we actually need to DeepCopy()?
		if err := c.updateSecret(ctx, addonSecret.DeepCopy(), supervisorAddress, supervisorCABundle); err != nil {
			panic(err) // TODO: handle me
		}
	}

	// TODO: ...and see if workload cluster gets configured correctly :)

	// TODO: what if addon secret doesn't exist?

	// TODO: don't send a request if the addon secret is already up to date

	return reconcile.Result{}, nil
}

func (c *pinnipedInfoController) updateSecret(
	ctx context.Context,
	addonSecret *corev1.Secret,
	supervisorAddress, supervisorCABundle string,
) error {
	// Get old data values.
	oldValuesYAML, ok := addonSecret.Data["values.yaml"] // TODO: get rid of raw strings...
	if !ok {
		panic("could not find data values") // TODO: handle me
	}

	// Unmarshal old data values into map.
	values := make(map[string]interface{})
	if err := yaml.Unmarshal(oldValuesYAML, &values); err != nil {
		panic("could not unmarshal values.yaml") // TODO: handle me
	}

	// Set supervisor info in data values.
	pinnipedValue, ok := values["pinniped"]
	if !ok {
		panic("could not find .pinniped value path") // TODO: handle me
	}
	pinnipedMapValue, ok := pinnipedValue.(map[interface{}]interface{})
	if !ok {
		panic(fmt.Sprintf(".pinniped value is unexpected type (got %T: %+v)", pinnipedValue, pinnipedValue)) // TODO: handle me
	}
	pinnipedMapValue["supervisor_svc_endpoint"] = supervisorAddress
	pinnipedMapValue["supervisor_ca_bundle_data"] = supervisorCABundle

	// Marshal new data values.
	newValuesYAML, err := yaml.Marshal(values)
	if err != nil {
		panic(err) // TODO: handle me
	}

	// Prepend new values with YTT data values directive.
	// TODO: this is ugly, let's not do this.
	yttDataValuesDirective := `#@data/values
#@overlay/match-child-defaults missing_ok=True
---`
	newValuesYAML = []byte(fmt.Sprintf("%s\n%s\n", yttDataValuesDirective, newValuesYAML))

	log.Printf(
		"addonSecret: %s/%s, oldValuesYAML: %q, newValuesYAML: %q",
		addonSecret.Namespace,
		addonSecret.Name,
		string(oldValuesYAML),
		string(newValuesYAML),
	)

	// If we don't have any updates to make to the data values, then don't call the API.
	if bytes.Equal(oldValuesYAML, newValuesYAML) {
		return nil
	}

	// Update the Secret.
	// TODO: do we care that this is going to wipe out all YAML comments in data values?
	addonSecret.Data["values.yaml"] = newValuesYAML // TODO: get rid of raw strings...
	// TODO: is Update() ok or do we want Patch()?
	if err := c.client.Update(ctx, addonSecret); err != nil {
		panic(err) // TODO: handle me
	}

	return nil
}
