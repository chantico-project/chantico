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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	ph "chantico/internal/patch"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var StateMachine = sm.Machine[*chantico.IPMIDevice]{
	Actions: map[string][]sm.ActionFunction[*chantico.IPMIDevice]{
		StateInit: {
			{Type: sm.ActionFunctionPure, Pure: sm.InitializeFinalizer[*chantico.IPMIDevice]},
		},
		StateEntryPoint: {
			{Type: sm.ActionFunctionPure, Pure: CreateIPMIDeploymentConfig},
		},
		StateSucceededIPMIConfigUpdate: {
			{Type: sm.ActionFunctionPure, Pure: CreateIPMIDeploymentConfig},
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

func GenerateIPMIConfig(ipmiDevice chantico.IPMIDevice) (string, error) {
	modules := map[string]chantico.IPMIConfig{}
	modules[ipmiDevice.Name] = ipmiDevice.Spec.Auth
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
	ipmiDevice *chantico.IPMIDevice,
) *sm.ActionResult {
	configFilePath := getConfigPath()

	// Create the file contents structure
	fileContents, err := os.ReadFile(configFilePath)
	if err != nil {
		fmt.Printf("Could not load file %s: %s", configFilePath, err)
		return nil
	}

	newIPMIConfig, err := GenerateIPMIConfig(*ipmiDevice)
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
