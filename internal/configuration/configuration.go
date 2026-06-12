package configuration

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
)

const (
	ChanticoVolumeLocationEnv        = "CHANTICO_DATA_PATH"
	ChanticoVolumeClaimEnv           = "CHANTICO_PERSISTENT_VOLUME_CLAIM_NAME"
	ChanticoPrometheusServiceHostEnv = "CHANTICO_PROMETHEUS_SERVICE_HOST"
	ChanticoPrometheusServicePortEnv = "CHANTICO_PROMETHEUS_SERVICE_PORT"
)

type validatedEnv struct {
	VolumeLocation        string
	VolumeClaim           string
	PrometheusServiceHost string
	PrometheusServicePort string
}

var ValidatedEnv validatedEnv

func ValidateEnv() (validatedEnv, []error) {
	var errs []error
	var ret validatedEnv
	volumeClaim, err := validateVar(ChanticoVolumeClaimEnv, validateClaim)
	if err != nil {
		errs = append(errs, err)
	} else {
		ret.VolumeClaim = volumeClaim
	}

	volumeLocation, err := validateVar(ChanticoVolumeLocationEnv, validateLocation)
	if err != nil {
		errs = append(errs, err)
	} else {
		ret.VolumeLocation = volumeLocation
	}

	prometheusServiceHost, err := validateVar(ChanticoPrometheusServiceHostEnv, validateHost)
	if err != nil {
		errs = append(errs, err)
	} else {
		ret.PrometheusServiceHost = prometheusServiceHost
	}

	prometheusServicePort, err := validateVar(ChanticoPrometheusServicePortEnv, validatePort)
	if err != nil {
		errs = append(errs, err)
	} else {
		ret.PrometheusServicePort = prometheusServicePort
	}

	if ret.PrometheusServiceHost != "" && ret.PrometheusServicePort != "" {
		err = validateHostPort(prometheusServiceHost, prometheusServicePort)
		if err != nil {
			errs = append(errs, err)
		}
		ret.PrometheusServiceHost = ""
		ret.PrometheusServicePort = ""

	}
	if len(errs) > 0 {
		return ret, errs
	}

	return ret, nil
}

func validateHostPort(host, port string) error {
	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return fmt.Errorf("cannot connect to host '%s': %w. If this is a development environment, make sure port forwarding has started.", net.JoinHostPort(host, port), err)
	} else {
		conn.Close()
		return nil
	}
}

func validateVar(varName string, extraTest func(string) error) (string, error) {
	value, ok := os.LookupEnv(varName)
	if !ok {
		return value, fmt.Errorf("environment variable %s must be set", varName)

	}
	if value == "" {
		return value, fmt.Errorf("environment variable %s is an empty string", varName)
	}
	if err := extraTest(value); err != nil {
		fmt.Println(err)
		return value, err
	}
	return value, nil

}

func validateClaim(value string) error {
	if matched, _ := regexp.Match("^([[:alpha:]]*-)+([[:alpha:]]*)$", []byte(value)); !matched {
		return fmt.Errorf("environment variable %s ('%s') is not a valid PVC name, should look like 'chantico-snmp-prometheus-volume-claim'", ChanticoVolumeClaimEnv, value)
	}
	return nil

}

func validateLocation(value string) error {
	fileInfo, err := os.Stat(value)
	if err != nil {
		return fmt.Errorf("cannot find directory specified by environment variable %s (directory '%s') (error: '%w'), should look like '/tmp/chantico-local-path-data/pvc-e95a75f9-46fc-450c-9ef8-ba959560d515_chantico_chantico-snmp-prometheus-volume-claim'", ChanticoVolumeLocationEnv, value, err)
	}
	if !fileInfo.IsDir() {
		return fmt.Errorf("environment variable %s ('%s') is not a directory, should look like '/tmp/chantico-local-path-data/pvc-e95a75f9-46fc-450c-9ef8-ba959560d515_chantico_chantico-snmp-prometheus-volume-claim'", ChanticoVolumeLocationEnv, value)
	}
	return nil
}

func validateHost(value string) error {
	addrs, err := net.LookupHost(value)
	if err != nil {
		return fmt.Errorf("error looking up prometheus host %s ('%s'), is it a valid address?", ChanticoPrometheusServiceHostEnv, value)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("lookup for prometheus host %s ('%s') returned empty, is it a valid address?", ChanticoPrometheusServiceHostEnv, value)
	}
	return nil
}

func validatePort(value string) error {
	_, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return fmt.Errorf("error converting prometheus port %s ('%s') to a 16-bit integer, is it a valid port?", ChanticoPrometheusServicePortEnv, value)
	}
	return nil
}
