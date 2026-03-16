# Kubernetes controller concepts

Conditions
additionalColums (phase/status)
observedGeneration
OwnerReferences
resourceVersion
finalizers



// +kubebuilder:subresource:status // creates a separate endpoint for status and spec
// +kubebuilder:printcolumn:name="Status",type=boolean,JSONPath=`.status.conditions[?(@.type=="Ready")].status` // shows additional column when using kubectl get



controller-runtime has a cache, r.Get(..., &myExample) // where r is the reconciler
gives a reference to the cache. Great is you just want to read. If you want to edit, you need to create a deep-copy.

can we perform multiple updates per reconciliation loop, or is it better to perform one update per reconciliation loop?
Status().Patch() does change the resourceversion,
it also triggers a reconciliation.

https://book.kubebuilder.io/reference/markers.html


return early, as soon as there is a change in the condition
r.reconcileFinalizers() // return if it returns something
// r.reconcileJob()
// r.reconcile...() // how to write tests?

controllers in eigen mappen? andere plekken? herbruikbare functie misschien een laag erboven? core.go wellicht in hetzelfde mapje


Controllers are designed to be level-based, not event-based.

You never know what triggered the Reconcile function. But you should always check the current situation and then apply changes (level-based).
If you have For().Owns(), you do not know if the Owned object caused the trigger or the For object. It should always be the same. You still get the main object (so your custom resource). Owns does this by looking at the owner references.


What is the desired state of the MeasurementDevice?
- have a finalizer
- keep sync with walks, mibs and the snmp exporter