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
	os.Unsetenv(ChanticoVolumeClaimEnv)
	testUnsetBadEnv(t, ChanticoVolumeClaimEnv, "[UNSET]", testIdentity)
}

func TestEmptyVars(t *testing.T) {
	testBadEnv(t, ChanticoVolumeClaimEnv, "", testIdentity)

}

func TestAllVarsOkay(t *testing.T) {
	const goodClaim = "test-test-test"
	var goodLocation = os.Getenv("PWD")
	const goodUrl = "example.com"
	const goodPort = "80"

	t.Parallel() // This test cannot run in parallel with the others, because the environment variables may interfere

	os.Setenv(ChanticoVolumeClaimEnv, goodClaim)
	os.Setenv(ChanticoVolumeLocationEnv, goodLocation)
	os.Setenv(ChanticoPrometheusServiceHostEnv, goodUrl)
	os.Setenv(ChanticoPrometheusServicePortEnv, goodPort)

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
	defer os.Remove(file.Name())

	var fileName = file.Name()
	testBadEnv(t, ChanticoVolumeLocationEnv, fileName, testIdentity)
}

func TestParseNetwork(t *testing.T) {
	const goodUrl = "example.com"
	var testGoodUrl = func(env validatedEnv) bool { return env.PrometheusServiceHost == goodUrl }
	testGoodEnv(t, ChanticoPrometheusServiceHostEnv, goodUrl, testGoodUrl)

	const badUrl = "notavalid.website"
	var testBadUrl = func(env validatedEnv) bool { return env.PrometheusServiceHost != badUrl }
	testBadEnv(t, ChanticoPrometheusServiceHostEnv, badUrl, testBadUrl)

	const goodPort = "8000"
	var testGoodPort = func(env validatedEnv) bool { return env.PrometheusServicePort == goodPort }
	testGoodEnv(t, ChanticoPrometheusServicePortEnv, goodPort, testGoodPort)

	const badPort = "123456789"
	var testBadPort = func(env validatedEnv) bool { return env.PrometheusServicePort != badPort }
	testBadEnv(t, ChanticoPrometheusServicePortEnv, badPort, testBadPort)
}

func TestBadHostPort(t *testing.T) {
	const goodUrl = "localhost"
	const reservedPort = "2" // Port 2 is reserved by the IANA (iana.org). Should be unused on all computer environments
	os.Setenv(ChanticoPrometheusServiceHostEnv, goodUrl)
	os.Setenv(ChanticoPrometheusServicePortEnv, reservedPort)

	ValidatedEnv, errs := ValidateEnv()

	var thisErrs []error
	for _, err := range errs {
		if errString := fmt.Sprintf("%s", err); strings.Contains(errString, "cannot connect to host") {
			thisErrs = append(thisErrs, err)
		}

	}
	if len(thisErrs) == 0 {
		t.Errorf("Unacessable internet service '%s' generated no errors ('%s')", net.JoinHostPort(goodUrl, reservedPort), errs)
	}
	if ValidatedEnv.PrometheusServiceHost == goodUrl {
		t.Errorf("Bad environment var '%s=%s' was accepted", ChanticoPrometheusServiceHostEnv, ValidatedEnv.PrometheusServiceHost)
	}
	if ValidatedEnv.PrometheusServicePort == reservedPort {
		t.Errorf("Bad environment var '%s=%s' was accepted", ChanticoPrometheusServicePortEnv, ValidatedEnv.PrometheusServicePort)
	}
}

func TestGoodHostPort(t *testing.T) {
	const goodUrl = "example.com"
	const goodPort = "80"

	os.Setenv(ChanticoPrometheusServiceHostEnv, goodUrl)
	os.Setenv(ChanticoPrometheusServicePortEnv, goodPort)

	testGoodEnv(t, ChanticoPrometheusServiceHostEnv, goodUrl, testIdentity)
}
