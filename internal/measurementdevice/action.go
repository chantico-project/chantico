package measurementdevice

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	chantico "chantico/api/v1alpha1"
	chanticok8s "chantico/internal/k8s"
	pm "chantico/internal/postmortem"
	vol "chantico/internal/volumes"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	ph "chantico/internal/patch"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// In that context Pure means does not modify the kubernetes cluster resources
type ActionFunctionType int

const (
	ActionFunctionIO = iota
	ActionFunctionPure
)

type ActionResult struct {
	*ctrl.Result
	ph.PatchType
}

type ActionFunction struct {
	Type ActionFunctionType
	Pure func(
		*chantico.MeasurementDevice,
	) *ActionResult
	IO func(
		context.Context,
		client.Client,
		*chantico.MeasurementDevice,
	) *ActionResult
}

var ActionMap = map[State][]ActionFunction{
	StateInit: {
		{Type: ActionFunctionPure, Pure: InitializeFinalizer},
	},
	StateEntryPoint: {
		{Type: ActionFunctionPure, Pure: CreateSNMPGenerator},
		{Type: ActionFunctionPure, Pure: CreateSNMPDeploymentConfig},
		{Type: ActionFunctionIO, IO: ScheduleSNMPGeneratorJob},
	},
	StatePendingSNMPConfigUpdate: {
		{Type: ActionFunctionPure, Pure: RequeueWithDelay},
	},
	StateSucceededSNMPConfigUpdate: {
		{Type: ActionFunctionPure, Pure: CreateSNMPDeploymentConfig},
		{Type: ActionFunctionIO, IO: ReloadSNMPService},
	},
	StateDelete: {
		{Type: ActionFunctionPure, Pure: DeleteSNMPConfig},
		{Type: ActionFunctionPure, Pure: CreateSNMPDeploymentConfig},
		{Type: ActionFunctionIO, IO: ReloadSNMPService},
		{Type: ActionFunctionPure, Pure: UpdateFinalizer},
	},
	StatePendingSNMPReload: {},
	StateFailed:            {},
	StateEndPoint:          {},
}

func ExecuteActions(
	ctx context.Context,
	kubernetesClient client.Client,
	measurementDevice *chantico.MeasurementDevice,
	patch *ph.PatchHelper,
) *ActionResult {
	var result *ActionResult = nil
	stateActions := ActionMap[State(measurementDevice.Status.State)] // IVO: maybe this "state conversion" can already be done when Unmarshalling the JSON. We can then enforce it.
	for i, actionFunction := range stateActions {
		log.Printf("Start step %d, status: %s\n", i, measurementDevice.Status.State)
		switch actionFunction.Type {
		case ActionFunctionPure:
			result = actionFunction.Pure(measurementDevice)
		case ActionFunctionIO:
			result = actionFunction.IO(ctx, kubernetesClient, measurementDevice)
		}

		if result != nil {
			patch.Patch(result.PatchType)
			if result.Result != nil || measurementDevice.Status.State == StateFailed {
				break
			}
		}
	}
	return result
}

func InitializeFinalizer(
	measurementDevice *chantico.MeasurementDevice,
) *ActionResult {
	if slices.Contains(measurementDevice.ObjectMeta.Finalizers, chantico.SNMPUpdateFinalizer) {
		return nil
	}
	measurementDevice.ObjectMeta.Finalizers = append(measurementDevice.ObjectMeta.Finalizers, chantico.SNMPUpdateFinalizer)
	log.Printf("ADDED FINALIZER: %#v", measurementDevice.ObjectMeta.Finalizers)
	return &ActionResult{PatchType: ph.PatchResource}
}

func UpdateFinalizer(
	measurementDevice *chantico.MeasurementDevice,
) *ActionResult {
	if measurementDevice.ObjectMeta.DeletionTimestamp.IsZero() {
		return nil
	}
	accumulator := []string{}
	for _, f := range measurementDevice.ObjectMeta.Finalizers {
		if f != chantico.SNMPUpdateFinalizer {
			accumulator = append(accumulator, f)
		}
	}
	measurementDevice.ObjectMeta.Finalizers = accumulator
	return &ActionResult{PatchType: ph.PatchResource}
}

func UpdateModification(
	measurementDevice *chantico.MeasurementDevice,
) *ActionResult {
	measurementDevice.Status.UpdateTime = metav1.Time{Time: time.Now()}.Format(time.RFC3339)
	measurementDevice.Status.UpdateGeneration = measurementDevice.ObjectMeta.Generation
	return &ActionResult{PatchType: ph.PatchResourceStatus}
}

func RequeueWithDelay(
	measurementDevice *chantico.MeasurementDevice,
) *ActionResult {
	// TODO: Figure out requeuing strategy, might need a redesign
	return &ActionResult{Result: &ctrl.Result{RequeueAfter: chantico.RequeueDelay}}
}

func CreateSNMPGenerator(
	measurementDevice *chantico.MeasurementDevice,
) *ActionResult {
	log.Printf("CHECK FINALIZER: %#v", measurementDevice.ObjectMeta.Finalizers)
	generatorYaml, err := GenerateSNMPGeneratorConfig(*measurementDevice)
	if err != nil {
		pm.NewPostMortem(err, measurementDevice)
	}
	generatorPath := fmt.Sprintf(
		"%s/%s/generator_%s.yml",
		os.Getenv(vol.ChanticoVolumeLocationEnv),
		snmpConfigDir,
		string(measurementDevice.GetUID()),
	)
	err = os.WriteFile(generatorPath, []byte(generatorYaml), 0666)
	if err != nil {
		measurementDevice.Status.State = StateFailed
		measurementDevice.Status.ErrorMessage = fmt.Sprintf("Could not write to %s", generatorPath)
		log.Printf("%s\n", measurementDevice.Status.ErrorMessage)
	}
	configPath := filepath.Join(
		os.Getenv(vol.ChanticoVolumeLocationEnv),
		snmpConfigDir,
		fmt.Sprintf("config_%s.yml", string(measurementDevice.GetUID())),
	)
	err = os.WriteFile(configPath, []byte{}, 0666)
	if err != nil {
		measurementDevice.Status.State = StateFailed
		measurementDevice.Status.ErrorMessage = fmt.Sprintf("Could not write to %s", configPath)
		log.Printf("%s\n", measurementDevice.Status.ErrorMessage)
	}
	return &ActionResult{PatchType: ph.PatchResourceStatus}
}

func DeleteSNMPConfig(
	measurementDevice *chantico.MeasurementDevice,
) *ActionResult {
	volumePath := os.Getenv(vol.ChanticoVolumeLocationEnv)
	_ = os.Remove(filepath.Join(volumePath, snmpConfigDir, fmt.Sprintf("config_%s.yml", measurementDevice.ObjectMeta.GetUID())))
	_ = os.Remove(filepath.Join(volumePath, snmpConfigDir, fmt.Sprintf("generator_%s.yml", measurementDevice.ObjectMeta.GetUID())))
	return nil
}

func CreateSNMPDeploymentConfig(
	measurementDevice *chantico.MeasurementDevice,
) *ActionResult {
	// Find files match the config_*.yml format
	configFilesGlobPattern := filepath.Join(
		os.Getenv(vol.ChanticoVolumeLocationEnv),
		snmpConfigDir,
		"config_*.yml",
	)
	configFilePaths, err := filepath.Glob(configFilesGlobPattern)
	if err != nil {
		return nil
	}

	// Create the file contents structure
	fileContents := [][]byte{}
	for _, configFilePath := range configFilePaths {
		fileContent, err := os.ReadFile(configFilePath)
		if err != nil {
			fmt.Printf("Could not load file %s: %s", configFilePath, err)
		}
		fileContents = append(fileContents, fileContent)
	}

	// Merge the data
	mergedSNMPConfig, err := MergeSNMPConfigs(fileContents)
	if err != nil {
		fmt.Printf("Could not create the SNMP deployment config: %s", err)
		return nil
	}
	configSNMPPath := filepath.Join(
		os.Getenv(vol.ChanticoVolumeLocationEnv),
		snmpYmlDir,
		"snmp.yml",
	)
	err = os.WriteFile(
		configSNMPPath,
		[]byte(mergedSNMPConfig),
		0666,
	)
	if err != nil {
		fmt.Printf("Could not write to %s: %s", configSNMPPath, err)
		return nil
	}
	return nil
}

var snmpReloadMutex sync.Mutex = sync.Mutex{}

func ReloadSNMPService(
	ctx context.Context,
	kubernetesClient client.Client,
	measurementDevice *chantico.MeasurementDevice,
) *ActionResult {
	snmpDeployment := &appsv1.Deployment{}
	_ = kubernetesClient.Get(ctx, client.ObjectKey{Name: "chantico-snmp", Namespace: "chantico"}, snmpDeployment)

	if !snmpReloadMutex.TryLock() {
		return &ActionResult{Result: &ctrl.Result{RequeueAfter: chantico.RequeueDelay}}
	}

	if measurementDevice.Status.State != StateDelete {
		measurementDevice.Status.State = StatePendingSNMPReload
	}
	go func() {
		log.Printf("Enter SNMP reload logic")
		var err error
		defer snmpReloadMutex.Unlock()
		restartCtx, cancel := context.WithTimeout(context.Background(), chantico.ReloadTimeout)
		defer cancel()

		// Add the annotation to the deployment
		if snmpDeployment.Spec.Template.Annotations == nil {
			snmpDeployment.Spec.Template.Annotations = make(map[string]string)
		}

		snmpDeployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
		if err = kubernetesClient.Update(restartCtx, snmpDeployment); err != nil {
			log.Printf("Failed to update")
			measurementDevice.Status.State = StateFailed
			measurementDevice.Status.ErrorMessage = err.Error()
		}

		log.Printf("Update sent")
		// Poll to check if the deployment is ready
		ticker := time.NewTicker(chantico.ReloadInterval)
		defer ticker.Stop()
		for {
			select {
			case <-restartCtx.Done():
				log.Printf("Failed")
				if measurementDevice.Status.State != StateDelete {
					measurementDevice.Status.State = StateFailed
					measurementDevice.Status.ErrorMessage = "chantico-snmp reload timed out"
				}
				return
			case <-ticker.C:
				log.Printf("Polling")
				if err := kubernetesClient.Get(restartCtx, client.ObjectKey{Name: "chantico-snmp", Namespace: "chantico"}, snmpDeployment); err != nil {
					continue
				}
				if chanticok8s.CheckDeploymentAvailability(*snmpDeployment) {
					if measurementDevice.Status.State != StateDelete {
						measurementDevice.Status.State = StateEndPoint
					}
					time.Sleep(chanticok8s.K8sGracePeriod)
					err = kubernetesClient.Status().Update(ctx, measurementDevice)
					if err != nil {
						log.Printf("Could not update status", err)
					}
					return
				}
			}
		}
	}()
	return &ActionResult{PatchType: ph.PatchResourceStatus}
}

func ScheduleSNMPGeneratorJob(
	ctx context.Context,
	kubernetesClient client.Client,
	scheme *runtime.Scheme,
	measurementDevice *chantico.MeasurementDevice,
) *ActionResult {
	measurementDevice.Status.JobName = fmt.Sprintf("update-snmp-%s-%d", measurementDevice.Name, int(time.Now().Unix()))
	measurementDevice.Status.State = StatePendingSNMPConfigUpdate
	log.Printf("New Status: %s\n", measurementDevice.Status.State)

	updateJob := MakeJob(*measurementDevice)
	err = controllerutil.SetControllerReference(measurementDevice, updateJob, r.)

	err := kubernetesClient.Create(ctx, updateJob)
	if err != nil {
		measurementDevice.Status.State = StateFailed
		measurementDevice.Status.ErrorMessage = err.Error()
		return &ActionResult{PatchType: ph.PatchResourceStatus}
	}
	return &ActionResult{PatchType: ph.PatchResourceStatus}
}
