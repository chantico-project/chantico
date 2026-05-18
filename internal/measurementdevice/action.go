package measurementdevice

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	chantico "chantico/api/v1alpha1"
	config "chantico/internal/configuration"
	chanticok8s "chantico/internal/k8s"
	pm "chantico/internal/postmortem"
	sm "chantico/internal/statemachine"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	ph "chantico/internal/patch"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var StateMachine = sm.Machine[*chantico.MeasurementDevice]{
	Actions: map[string][]sm.ActionFunction[*chantico.MeasurementDevice]{
		StateInit: {
			{Type: sm.ActionFunctionPure, Pure: sm.InitializeFinalizer[*chantico.MeasurementDevice]},
		},
		StateEntryPoint: {
			{Type: sm.ActionFunctionPure, Pure: CreateSNMPGenerator},
			{Type: sm.ActionFunctionPure, Pure: CreateSNMPDeploymentConfig},
			{Type: sm.ActionFunctionIO, IO: ScheduleSNMPGeneratorJob},
		},
		StatePendingSNMPConfigUpdate: {
			{Type: sm.ActionFunctionPure, Pure: RequeueWithDelay},
		},
		StateSucceededSNMPConfigUpdate: {
			{Type: sm.ActionFunctionPure, Pure: CreateSNMPDeploymentConfig},
			{Type: sm.ActionFunctionIO, IO: ReloadSNMPService},
		},
		StateDelete: {
			{Type: sm.ActionFunctionPure, Pure: DeleteSNMPConfig},
			{Type: sm.ActionFunctionPure, Pure: CreateSNMPDeploymentConfig},
			{Type: sm.ActionFunctionIO, IO: ReloadSNMPService},
			{Type: sm.ActionFunctionPure, Pure: sm.RemoveFinalizer[*chantico.MeasurementDevice]},
		},
		StatePendingSNMPReload: {},
		StateFailed:            {},
		StateEndPoint:          {},
	},
	FailState: StateFailed,
}

func UpdateModification(
	measurementDevice *chantico.MeasurementDevice,
) *sm.ActionResult {
	measurementDevice.Status.UpdateTime = metav1.Time{Time: time.Now()}.Format(time.RFC3339)
	measurementDevice.Status.UpdateGeneration = measurementDevice.ObjectMeta.Generation
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}

func RequeueWithDelay(
	ctx context.Context,
	measurementDevice *chantico.MeasurementDevice,
) *sm.ActionResult {
	// TODO: Figure out requeuing strategy, might need a redesign
	return &sm.ActionResult{Result: &ctrl.Result{RequeueAfter: chantico.RequeueDelay}}
}

/*
Creates generator_[id].yml file, and empty config_[id].yml file.
*/
func CreateSNMPGenerator(
	ctx context.Context,
	measurementDevice *chantico.MeasurementDevice,
) *sm.ActionResult {
	l := log.FromContext(ctx)
	l.Info("Check finalizer", "finalizers", strings.Join(measurementDevice.ObjectMeta.Finalizers, ", "))
	generatorYaml, err := GenerateSNMPGeneratorConfig(*measurementDevice)
	if err != nil {
		pm.NewPostMortem(err, measurementDevice)
	}
	generatorPath := fmt.Sprintf(
		"%s/%s/generator_%s.yml",
		config.ValidatedEnv.VolumeLocation,
		snmpConfigDir,
		string(measurementDevice.GetUID()),
	)
	err = os.WriteFile(generatorPath, []byte(generatorYaml), 0666)
	if err != nil {
		measurementDevice.Status.State = StateFailed
		measurementDevice.Status.ErrorMessage = fmt.Sprintf("Could not write to %s", generatorPath)
		l.Error(nil, measurementDevice.Status.ErrorMessage)
	}
	configPath := filepath.Join(
		config.ValidatedEnv.VolumeLocation,
		snmpConfigDir,
		fmt.Sprintf("config_%s.yml", string(measurementDevice.GetUID())),
	)
	err = os.WriteFile(configPath, []byte{}, 0666)
	if err != nil {
		measurementDevice.Status.State = StateFailed
		measurementDevice.Status.ErrorMessage = fmt.Sprintf("Could not write to %s", configPath)
		l.Error(nil, measurementDevice.Status.ErrorMessage)
	}
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}

func DeleteSNMPConfig(
	ctx context.Context,
	measurementDevice *chantico.MeasurementDevice,
) *sm.ActionResult {
	volumePath := config.ValidatedEnv.VolumeLocation
	_ = os.Remove(filepath.Join(volumePath, snmpConfigDir, fmt.Sprintf("config_%s.yml", measurementDevice.ObjectMeta.GetUID())))
	_ = os.Remove(filepath.Join(volumePath, snmpConfigDir, fmt.Sprintf("generator_%s.yml", measurementDevice.ObjectMeta.GetUID())))
	return nil
}

/*
Combines config_*.yml into snmp.yml
*/
func CreateSNMPDeploymentConfig(
	ctx context.Context,
	measurementDevice *chantico.MeasurementDevice,
) *sm.ActionResult {
	l := log.FromContext(ctx)
	// Find files match the config_*.yml format
	configFilesGlobPattern := filepath.Join(
		config.ValidatedEnv.VolumeLocation,
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
			l.Info("Could not load file", "file", configFilePath, "error", err)
		}
		fileContents = append(fileContents, fileContent)
	}

	// Merge the data
	mergedSNMPConfig, err := MergeSNMPConfigs(fileContents)
	if err != nil {
		l.Error(err, "Could not create the SNMP deployment config")
		return nil
	}
	configSNMPPath := filepath.Join(
		config.ValidatedEnv.VolumeLocation,
		snmpYmlDir,
		"snmp.yml",
	)
	err = os.WriteFile(
		configSNMPPath,
		[]byte(mergedSNMPConfig),
		0666,
	)
	if err != nil {
		l.Error(err, "Could not write to SNMP deployment config", "file", configSNMPPath)
		return nil
	}
	return nil
}

var snmpReloadMutex sync.Mutex = sync.Mutex{}

func ReloadSNMPService(
	ctx context.Context,
	kubernetesClient client.Client,
	measurementDevice *chantico.MeasurementDevice,
) *sm.ActionResult {
	l := log.FromContext(ctx)
	snmpDeployment := &appsv1.Deployment{}
	_ = kubernetesClient.Get(ctx, client.ObjectKey{Name: "chantico-snmp", Namespace: "chantico"}, snmpDeployment)

	if !snmpReloadMutex.TryLock() {
		return &sm.ActionResult{Result: &ctrl.Result{RequeueAfter: chantico.RequeueDelay}}
	}

	if measurementDevice.Status.State != StateDelete {
		measurementDevice.Status.State = StatePendingSNMPReload
	}
	go func() {
		l.Info("Enter SNMP reload logic")
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
			l.Error(err, "Failed to update SNMP deployment")
			measurementDevice.Status.State = StateFailed
			measurementDevice.Status.ErrorMessage = err.Error()
		}

		l.Info("Update sent")
		// Poll to check if the deployment is ready
		ticker := time.NewTicker(chantico.ReloadInterval)
		defer ticker.Stop()
		for {
			select {
			case <-restartCtx.Done():
				l.Error(nil, "Failed")
				if measurementDevice.Status.State != StateDelete {
					measurementDevice.Status.State = StateFailed
					measurementDevice.Status.ErrorMessage = "chantico-snmp reload timed out"
				}
				return
			case <-ticker.C:
				l.Info("Polling")
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
						l.Error(err, "Could not update status")
					}
					return
				}
			}
		}
	}()
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}

/*
Creates Job. The Job uses the MIBs and generator_[id].yml file to give actual content to the empty config_[id].yml.
*/
func ScheduleSNMPGeneratorJob(
	ctx context.Context,
	kubernetesClient client.Client,
	measurementDevice *chantico.MeasurementDevice,
) *sm.ActionResult {
	measurementDevice.Status.JobName = fmt.Sprintf("update-snmp-%s-%d", measurementDevice.Name, int(time.Now().Unix()))
	measurementDevice.Status.State = StatePendingSNMPConfigUpdate
	log.FromContext(ctx).Info("New Status", "status", measurementDevice.Status.State)

	updateJob := MakeJob(*measurementDevice)
	err := kubernetesClient.Create(ctx, updateJob)
	if err != nil {
		measurementDevice.Status.State = StateFailed
		measurementDevice.Status.ErrorMessage = err.Error()
		return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
	}
	return &sm.ActionResult{PatchType: ph.PatchResourceStatus}
}
