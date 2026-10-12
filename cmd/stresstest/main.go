package main

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"time"

	chapi "chantico/api/v1alpha1"
	k8s "chantico/internal/k8s"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const mibPath = "./dev/mibs/TNO-PDU-MIB.txt"

// parseMIB extracts identifiers that precede "OBJECT-TYPE" lines in a MIB file.
// e.g. "tnoPduEnergyValue OBJECT-TYPE" -> "tnoPduEnergyValue"
func parseMIB(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	re := regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s+OBJECT-TYPE`)
	var names []string
	for scanner.Scan() {
		line := scanner.Text()
		if matches := re.FindStringSubmatch(line); matches != nil {
			names = append(names, matches[1])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

// generateMeasurementDevice creates a MeasurementDevice with a random subset of walks.
// It sets TypeMeta and ObjectMeta so the resource can be created via the Kubernetes API.
func generateMeasurementDevice(allWalks []string, idx int) chapi.MeasurementDevice {
	n := len(allWalks)
	subCount := 1
	if n > 1 {
		subCount = rand.Intn(n) + 1 // [1..n]
	}

	perm := rand.Perm(n)
	walks := make([]string, 0, subCount)
	for i := 0; i < subCount; i++ {
		walks = append(walks, allWalks[perm[i]])
	}

	name := fmt.Sprintf("stresstest-md-%d-%d", time.Now().Unix(), idx)

	md := chapi.MeasurementDevice{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MeasurementDevice",
			APIVersion: "chantico/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "chantico",
		},
		Spec: chapi.MeasurementDeviceSpec{
			Walks: walks,
			AuthFrom: &k8s.SecretConfigMapSelector{
				Value: `community: public
version: 2`,
			},
		},
	}
	return md
}

func main() {
	rand.Seed(time.Now().UnixNano())

	allWalks, err := parseMIB(mibPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse MIB %s: %v\n", mibPath, err)
		os.Exit(1)
	}
	if len(allWalks) == 0 {
		fmt.Fprintln(os.Stderr, "no walks found in MIB; nothing to generate")
		os.Exit(1)
	}

	// Build scheme and add our API types
	scheme := runtime.NewScheme()
	if err := chapi.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add API to scheme: %v\n", err)
		os.Exit(1)
	}

	// Get kubeconfig (in-cluster or KUBECONFIG)
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to get kubeconfig: %v\n", err)
		os.Exit(1)
	}

	cli, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create kubernetes client: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Generate and create 5 MeasurementDevice resources
	for i := 0; i < 10; i++ {
		md := generateMeasurementDevice(allWalks, i)
		fmt.Printf("Creating MeasurementDevice %s in namespace %s with %d walks\n", md.Name, md.Namespace, len(md.Spec.Walks))
		if err := cli.Create(ctx, &md); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create %s: %v\n", md.Name, err)
		}
		// small delay between creates to avoid hammering the API server
		time.Sleep(100 * time.Millisecond)
	}
}
