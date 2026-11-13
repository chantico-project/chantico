package configuration

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
)

func testGoodEnv(t *testing.T, env string, val string, tester func(validatedEnv) bool) {
	t.Setenv(env, val)
	ValidatedEnv, errs := ValidateEnv()
	for _, err := range errs {
		if errString := fmt.Sprintf("%s", err); strings.Contains(errString, env) {
			t.Errorf("Good env raised error '%s'", errString)
		}

	}

	if !tester(ValidatedEnv) {
		t.Errorf("Good environment var '%s' was not accepted, parsed env is '%+v'", val, ValidatedEnv)
	}

}

func testBadEnv(t *testing.T, env string, val string, tester func(validatedEnv) bool) {
	t.Setenv(env, val)
	testUnsetBadEnv(t, env, val, tester)
}

func testUnsetBadEnv(t *testing.T, env string, val string, tester func(validatedEnv) bool) {
	ValidatedEnv, errs := ValidateEnv()
	var thisErrs []error
	for _, err := range errs {
		if errString := fmt.Sprintf("%s", err); strings.Contains(errString, env) {
			thisErrs = append(thisErrs, err)
		}

	}
	if len(thisErrs) == 0 {
		t.Errorf("Bad environment var '%s=%s' generated no errors ('%s')", env, val, errs)

	}
	if !tester(ValidatedEnv) {
		t.Errorf("Bad environment var '%s=%s' was accepted", env, val)
	}
}

func testIdentity(env validatedEnv) bool { return true }

func TestUnsetVar(t *testing.T) {
	_ = os.Unsetenv(ChanticoVolumeClaimEnv)
	testUnsetBadEnv(t, ChanticoVolumeClaimEnv, "[UNSET]", testIdentity)
}

func TestEmptyVars(t *testing.T) {
	testBadEnv(t, ChanticoVolumeClaimEnv, "", testIdentity)

}

func TestAllVarsOkay(t *testing.T) {
	const goodClaim = "test-test-test"
	var goodLocation = os.Getenv("PWD")
	const goodUrl = "github.com"
	const goodPort = "443"

	t.Parallel() // This test cannot run in parallel with the others, because the environment variables may interfere

	_ = os.Setenv(ChanticoVolumeClaimEnv, goodClaim)
	_ = os.Setenv(ChanticoVolumeLocationEnv, goodLocation)
	_ = os.Setenv(ChanticoSnmpExporterServiceHostEnv, goodUrl)
	_ = os.Setenv(ChanticoSnmpExporterServicePortEnv, goodPort)
	_ = os.Setenv(ChanticoNamespaceEnv, "chantico")
	_ = os.Setenv(ChanticoWatchNamespaceEnv, AllNamespaces)

	_, errs := ValidateEnv()

	if len(errs) > 0 {
		t.Errorf("Good environment variables generated errors: '%s'", errs)
	}
}

func TestParseVolume(t *testing.T) {
	const goodClaim = "test-test"
	var testGoodClaim = func(env validatedEnv) bool { return env.VolumeClaim == goodClaim }
	testGoodEnv(t, ChanticoVolumeClaimEnv, goodClaim, testGoodClaim)

	const badClaim = "asdfghjkl"
	var testBadClaim = func(env validatedEnv) bool { return env.VolumeClaim != badClaim }
	testBadEnv(t, ChanticoVolumeClaimEnv, badClaim, testBadClaim)

	var goodLocation = os.Getenv("PWD")
	var testGoodLocation = func(env validatedEnv) bool { return env.VolumeLocation == goodLocation }
	testGoodEnv(t, ChanticoVolumeLocationEnv, goodLocation, testGoodLocation)

	var badLocation = "/definitely/not/a/path"
	var testBadLocation = func(env validatedEnv) bool { return env.VolumeLocation != badLocation }
	testBadEnv(t, ChanticoVolumeLocationEnv, badLocation, testBadLocation)
}

func TestVolumeIsDir(t *testing.T) {
	file, err := os.CreateTemp("", "chantico-test")
	if err != nil {
		t.Error("Error creating temporary file, this should not happen")
	}
	defer func() {
		_ = os.Remove(file.Name())
	}()

	var fileName = file.Name()
	testBadEnv(t, ChanticoVolumeLocationEnv, fileName, testIdentity)
}

func TestParseNetwork(t *testing.T) {
	const goodUrl = "chantico-snmp.chantico.svc.cluster.local"
	t.Setenv(ChanticoSnmpExporterServicePortEnv, "443")
	var testGoodUrl = func(env validatedEnv) bool { return env.SnmpExporterServiceHost == goodUrl }
	testGoodEnv(t, ChanticoSnmpExporterServiceHostEnv, goodUrl, testGoodUrl)

	const badUrl = "not a valid host"
	var testBadUrl = func(env validatedEnv) bool { return env.SnmpExporterServiceHost != badUrl }
	testBadEnv(t, ChanticoSnmpExporterServiceHostEnv, badUrl, testBadUrl)

	const goodPort = "8000"
	var testGoodPort = func(env validatedEnv) bool { return env.SnmpExporterServicePort == goodPort }
	testGoodEnv(t, ChanticoSnmpExporterServicePortEnv, goodPort, testGoodPort)

	const badPort = "123456789"
	var testBadPort = func(env validatedEnv) bool { return env.SnmpExporterServicePort != badPort }
	testBadEnv(t, ChanticoSnmpExporterServicePortEnv, badPort, testBadPort)
}

func TestUnresolvableHostIsAccepted(t *testing.T) {
	const goodUrl = "chantico-snmp.nonexistent.svc.cluster.local"
	const goodPort = "9116"

	t.Setenv(ChanticoSnmpExporterServiceHostEnv, goodUrl)
	t.Setenv(ChanticoSnmpExporterServicePortEnv, goodPort)

	var testGoodHostPort = func(env validatedEnv) bool {
		return env.SnmpExporterServiceHost == goodUrl && env.SnmpExporterServicePort == goodPort
	}
	testGoodEnv(t, ChanticoSnmpExporterServiceHostEnv, goodUrl, testGoodHostPort)
}

func TestValidateHostLookup(t *testing.T) {
	if err := validateHostLookup(ChanticoSnmpExporterServiceHostEnv, "localhost"); err != nil {
		t.Fatalf("localhost lookup failed: %v", err)
	}
}

func TestValidateHostPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not start test listener: %v", err)
	}
	defer listener.Close()

	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("could not read test listener address: %v", err)
	}
	if err := validateHostPort([]string{ChanticoSnmpExporterServiceHostEnv, ChanticoSnmpExporterServicePortEnv}, host, port); err != nil {
		t.Fatalf("reachable host and port failed validation: %v", err)
	}
}

func TestPodNamespace(t *testing.T) {
	cases := []struct {
		name string
		val  string
		good bool
	}{
		{"ValidNamespace", "chantico-system", true},
		{"InvalidNamespace", "Not_A_Valid_Namespace!", false},
		{"AsteriskNamespace", "*", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(ChanticoNamespaceEnv, c.val)
			if c.good {
				var testPodNamespace = func(env validatedEnv) bool { return env.PodNamespace == c.val }
				testGoodEnv(t, ChanticoNamespaceEnv, c.val, testPodNamespace)
			} else {
				var testPodNamespace = func(env validatedEnv) bool { return env.PodNamespace != c.val }
				testBadEnv(t, ChanticoNamespaceEnv, c.val, testPodNamespace)
			}
		})
	}
}

func TestWatchNamespace(t *testing.T) {
	t.Setenv(ChanticoNamespaceEnv, "")

	cases := []struct {
		name         string
		podNamespace string
		val          string
		good         bool
	}{
		{"AllNamespaces", "chantico", "*", true},
		{"SpecificNamespace", "chantico", "chantico-system", true},
		{"InvalidNamespace", "", "Not_A_Valid_Namespace!", false},
		{"InvalidNamespaceNoFallback", "chantico-ignore", "No_Fallback!", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(ChanticoNamespaceEnv, c.podNamespace)
			t.Setenv(ChanticoWatchNamespaceEnv, c.val)
			if c.good {
				var testWatchNamespace = func(env validatedEnv) bool { return env.WatchNamespace == c.val }
				testGoodEnv(t, ChanticoWatchNamespaceEnv, c.val, testWatchNamespace)
			} else {
				var testWatchNamespace = func(env validatedEnv) bool { return env.WatchNamespace != c.val }
				testBadEnv(t, ChanticoWatchNamespaceEnv, c.val, testWatchNamespace)
			}
		})
	}
}

func TestWatchNamespaceDefaultsToPodNamespace(t *testing.T) {
	const podNamespace = "chantico-system"
	_ = os.Unsetenv(ChanticoWatchNamespaceEnv)
	t.Setenv(ChanticoNamespaceEnv, podNamespace)

	var testDefaultsToPodNamespace = func(env validatedEnv) bool { return env.WatchNamespace == podNamespace }
	testGoodEnv(t, ChanticoNamespaceEnv, podNamespace, testDefaultsToPodNamespace)
}

func TestWatchNamespaceMissingBothVars(t *testing.T) {
	_ = os.Unsetenv(ChanticoWatchNamespaceEnv)
	_ = os.Unsetenv(ChanticoNamespaceEnv)

	testUnsetBadEnv(t, ChanticoWatchNamespaceEnv, "[UNSET]", testIdentity)
}
