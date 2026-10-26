package main

import (
	"bufio"
	k8s "chantico/internal/k8s"
	"fmt"
	"log"
	"math/rand"
	"os"
	"regexp"
	"time"

	chapi "chantico/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
	var allWalks []string
	for scanner.Scan() {
		line := scanner.Text()
		if matches := re.FindStringSubmatch(line); matches != nil {
			allWalks = append(allWalks, matches[1])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return allWalks, nil
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
	for i := range subCount {
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

func generateMeasurementDevices(path string, n int) []chapi.MeasurementDevice {
	mds := make([]chapi.MeasurementDevice, 0, n)

	allWalks, err := parseMIB(path)
	if err != nil || len(allWalks) == 0 {
		log.Fatalf("failed to parse MIB %s: %v\n", path, err)
		os.Exit(1)
	}

	for i := range n {
		mds = append(mds, generateMeasurementDevice(allWalks, i))
	}
	return mds
}
