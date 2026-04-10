package ipmidevice

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	chantico "chantico/api/v1alpha1"
	chanticok8s "chantico/internal/k8s"
	sm "chantico/internal/statemachine"

	"go.yaml.in/yaml/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	ph "chantico/internal/patch"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var StateMachine = sm.Machine[*chantico.IPMIDevice]{
	Actions: map[string][]sm.ActionFunction[*chantico.IPMIDevice]{
		StateInit: {
			{Type: sm.ActionFunctionPure, Pure: sm.InitializeFinalizer[*chantico.IPMIDevice]},
			{Type: sm.ActionFunctionIO, IO: MoveCredentialsToSecret},
		},
		StateEntryPoint: {
			{Type: sm.ActionFunctionIO, IO: CreateIPMIDeploymentConfig},
		},
		StateSucceededIPMIConfigUpdate: {
			{Type: sm.ActionFunctionIO, IO: CreateIPMIDeploymentConfig}, // XXX: Remove this once we have new controller logic
			{Type: sm.ActionFunctionIO, IO: ReloadIPMIService},
		},
		StateDelete: {
			{Type: sm.ActionFunctionPure, Pure: DeleteIPMIConfig},
			{Type: sm.ActionFunctionIO, IO: ReloadIPMIService},
			{Type: sm.ActionFunctionPure, Pure: sm.RemoveFinalizer[*chantico.IPMIDevice]},
		},
		StatePendingIPMIReload: {},
		StateFailed:            {},
		StateEndPoint:          {},
	},
	FailState: StateFailed,
}

func UpdateModification(
	ipmiDevice *chantico.IPMIDevice,
) *sm.ActionResult {
	ipmiDevice.Status.UpdateTime = metav1.Time{Time: time.Now()}.Format(time.RFC3339)
	ipmiDevice.Status.UpdateGeneration = ipmiDevice.ObjectMeta.Generation
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}

func MoveCredentialsToSecret(
	ctx context.Context,
	kubernetesClient client.Client,
	ipmiDevice *chantico.IPMIDevice,
) *sm.ActionResult {
	var secretName = ipmiDevice.Name + "-credentials"
	if ipmiDevice.Spec.SecretRef != "" {
		secretName = ipmiDevice.Spec.SecretRef
	}
	var secret corev1.Secret
	if err := kubernetesClient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: "chantico"}, &secret); err == nil {
		// TODO: Validation of secret contents (should be basic-auth with username and password keys)
		log.Printf("Secret %s already exists, skipping creation", secretName)
		return nil
	}

	// Create Secret
	secret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "chantico",
		},
		Type: corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			"username": []byte(ipmiDevice.Spec.Auth.User),
			"password": []byte(ipmiDevice.Spec.Auth.Password),
		},
	}

	// TODO: Set controller reference so that the secret gets deleted when the IPMI device config is deleted
	// This requires the reconciler scheme which needs to be passed to this function, once we have new logic
	/*
		if err := ctrl.SetControllerReference(ipmiDevice, &secret, r.Scheme); err != nil {
			// TODO: Error reporting to user (with new return value logic)
			log.Printf("Secret %s could not be linked to IPMI device config %s", secretName, ipmiDevice.Name)
			ipmiDevice.Status.State = StateFailed
			return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
		}
	*/

	if err := kubernetesClient.Create(ctx, &secret); err != nil {
		// TODO: Error reporting to user (with new return value logic)
		log.Printf("Secret %s could not be created for IPMI device config %s", secretName, ipmiDevice.Name)
		ipmiDevice.Status.State = StateFailed
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}

	// Remove credentials from the IPMI device config spec and set the secret reference
	ipmiDevice.Spec.SecretRef = secretName
	ipmiDevice.Spec.Auth.User = ""
	ipmiDevice.Spec.Auth.Password = ""
	return &sm.ActionResult{PatchType: ph.PatchResource}
}

func DeleteIPMIConfig(
	ipmiDevice *chantico.IPMIDevice,
) *sm.ActionResult {
	configFilePath := getConfigPath()

	// Create the file contents structure
	fileContents, err := os.ReadFile(configFilePath)
	if err != nil {
		fmt.Printf("Could not load file %s: %s", configFilePath, err)
		return nil
	}

	newIPMIConfig, err := DeleteIPMIConfigModule(fileContents, ipmiDevice.Name)
	if err != nil {
		fmt.Printf("Could not delete IPMI config module: %s", err)
		return nil
	}

	err = os.WriteFile(
		configFilePath,
		[]byte(newIPMIConfig),
		0666,
	)
	if err != nil {
		fmt.Printf("Could not write to %s: %s", configFilePath, err)
		return nil
	}
	return nil
}

func GenerateIPMIConfig(ipmiDevice *chantico.IPMIDevice, secret *corev1.Secret) (string, error) {
	modules := map[string]chantico.IPMIConfig{}
	auth := *ipmiDevice.Spec.Auth.DeepCopy()
	auth.User = string(secret.Data["username"])
	auth.Password = string(secret.Data["password"])
	modules[ipmiDevice.Name] = auth
	ipmiDeviceConfig := ipmiConfig{Modules: modules}

	out, err := yaml.Marshal(ipmiDeviceConfig)
	return string(out), err
}

/*
Combines module into config.yml
XXX: Based on SNMP controller action logic, but this has pre-logging printfs and
no proper state change upon errors
*/
func CreateIPMIDeploymentConfig(
	ctx context.Context,
	kubernetesClient client.Client,
	ipmiDevice *chantico.IPMIDevice,
) *sm.ActionResult {
	configFilePath := getConfigPath()

	// Create the file contents structure
	fileContents, err := os.ReadFile(configFilePath)
	if err != nil {
		fmt.Printf("Could not load file %s: %s", configFilePath, err)
		return nil
	}

	var secret corev1.Secret
	if err := kubernetesClient.Get(ctx, client.ObjectKey{Name: ipmiDevice.Spec.SecretRef, Namespace: "chantico"}, &secret); err != nil {
		log.Printf("Secret %s not found", ipmiDevice.Spec.SecretRef)
		return nil
	}
	newIPMIConfig, err := GenerateIPMIConfig(ipmiDevice, &secret)
	if err != nil {
		fmt.Printf("Could not generate IPMI config: %s", err)
		return nil
	}

	// Merge the data
	mergedIPMIConfig, err := MergeIPMIConfigs([][]byte{fileContents, []byte(newIPMIConfig)})
	if err != nil {
		fmt.Printf("Could not create the IPMI deployment config: %s", err)
		return nil
	}
	err = os.WriteFile(
		configFilePath,
		[]byte(mergedIPMIConfig),
		0666,
	)
	if err != nil {
		fmt.Printf("Could not write to %s: %s", configFilePath, err)
		return nil
	}
	ipmiDevice.Status.State = StateSucceededIPMIConfigUpdate
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}

var ipmiReloadMutex sync.Mutex = sync.Mutex{}

func ReloadIPMIService(
	ctx context.Context,
	kubernetesClient client.Client,
	ipmiDevice *chantico.IPMIDevice,
) *sm.ActionResult {
	ipmiDeployment := &appsv1.Deployment{}
	_ = kubernetesClient.Get(ctx, client.ObjectKey{Name: "chantico-ipmi", Namespace: "chantico"}, ipmiDeployment)

	if !ipmiReloadMutex.TryLock() {
		return &sm.ActionResult{Result: &ctrl.Result{RequeueAfter: chantico.RequeueDelay}}
	}

	if ipmiDevice.Status.State != StateDelete {
		ipmiDevice.Status.State = StatePendingIPMIReload
	}
	go func() {
		log.Printf("Enter IPMI reload logic")
		var err error
		defer ipmiReloadMutex.Unlock()
		restartCtx, cancel := context.WithTimeout(context.Background(), chantico.ReloadTimeout)
		defer cancel()

		// Add the annotation to the deployment
		if ipmiDeployment.Spec.Template.Annotations == nil {
			ipmiDeployment.Spec.Template.Annotations = make(map[string]string)
		}

		ipmiDeployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
		if err = kubernetesClient.Update(restartCtx, ipmiDeployment); err != nil {
			log.Printf("Failed to update")
			ipmiDevice.Status.State = StateFailed
			ipmiDevice.Status.ErrorMessage = err.Error()
		}

		log.Printf("Update sent")
		// Poll to check if the deployment is ready
		ticker := time.NewTicker(chantico.ReloadInterval)
		defer ticker.Stop()
		for {
			select {
			case <-restartCtx.Done():
				log.Printf("Failed")
				if ipmiDevice.Status.State != StateDelete {
					ipmiDevice.Status.State = StateFailed
					ipmiDevice.Status.ErrorMessage = "chantico-ipmi reload timed out"
				}
				return
			case <-ticker.C:
				log.Printf("Polling")
				if err := kubernetesClient.Get(restartCtx, client.ObjectKey{Name: "chantico-ipmi", Namespace: "chantico"}, ipmiDeployment); err != nil {
					continue
				}
				if chanticok8s.CheckDeploymentAvailability(*ipmiDeployment) {
					if ipmiDevice.Status.State != StateDelete {
						ipmiDevice.Status.State = StateEndPoint
					}
					time.Sleep(chanticok8s.K8sGracePeriod)
					err = kubernetesClient.Status().Update(ctx, ipmiDevice)
					if err != nil {
						log.Printf("Could not update status")
					}
					return
				}
			}
		}
	}()
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}
