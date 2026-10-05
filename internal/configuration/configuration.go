package configuration

import (
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	ChanticoVolumeLocationEnv        = "CHANTICO_DATA_PATH"
	ChanticoVolumeClaimEnv           = "CHANTICO_PERSISTENT_VOLUME_CLAIM_NAME"
	ChanticoPrometheusServiceHostEnv = "CHANTICO_PROMETHEUS_SERVICE_HOST"
	ChanticoPrometheusServicePortEnv = "CHANTICO_PROMETHEUS_SERVICE_PORT"
	ChanticoNamespaceEnv             = "CHANTICO_NAMESPACE"
	ChanticoWatchNamespaceEnv        = "CHANTICO_WATCH_NAMESPACE"
	ValidateHostPortTimeout          = 5 * time.Second

	// AllNamespaces is the sentinel value for ChanticoWatchNamespaceEnv that makes the operator
	// watch resources in every namespace instead of a single one.
	AllNamespaces = "*"
)

type UnsetError struct {
	VarName string
}

func (e UnsetError) Error() string {
	return fmt.Sprintf("environment variable %s is not set", e.VarName)
}

type EmptyError struct {
	VarName string
}

func (e EmptyError) Error() string {
	return fmt.Sprintf("environment variable %s is an empty string", e.VarName)
}

type InvalidError struct {
	VarName string
	Value   string
	Reason  string
	Err     error
}

func (e InvalidError) Error() string {
	msg := fmt.Sprintf("environment variable %s ('%s') is not valid: %s", e.VarName, e.Value, e.Reason)
	if e.Err != nil {
		msg = fmt.Sprintf("%s. %v", msg, e.Err)
	}
	return msg
}

type ConnectErrorType string

const (
	ConnectErrorTypeHost    ConnectErrorType = "host"
	ConnectErrorTypePort    ConnectErrorType = "port"
	ConnectErrorTypeAddress ConnectErrorType = "address"
)

type ConnectError struct {
	Type     ConnectErrorType
	VarNames []string
	Value    string
	Reason   string
	Err      error
}

func (e ConnectError) Error() string {
	return fmt.Sprintf("cannot connect to %s %s (from environment variable %v): %s. %v", e.Type, e.Value, e.VarNames, e.Err, e.Reason)
}

type validatedEnv struct {
	VolumeLocation        string
	VolumeClaim           string
	PrometheusServiceHost string
	PrometheusServicePort string
	// PodNamespace is the namespace the operator is deployed in.
	PodNamespace string
	// WatchNamespace is either a single namespace name to restrict watches to,
	// or `config.AllNamespaces` to watch every namespace. It defaults to the
	// namespace the operator is deployed in.
	WatchNamespace string
}

var ValidatedEnv validatedEnv

func ValidateEnv() (validatedEnv, []error) {
	var errs []error
	var ret validatedEnv

	podNamespace, watchNamespace, nsErrs := validateNamespaces()
	ret.PodNamespace = podNamespace
	ret.WatchNamespace = watchNamespace
	if nsErrs != nil {
		errs = append(errs, nsErrs...)
	}

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
		err = validateHostPort([]string{ChanticoPrometheusServiceHostEnv, ChanticoPrometheusServicePortEnv}, prometheusServiceHost, prometheusServicePort)
		if err != nil {
			errs = append(errs, err)
			ret.PrometheusServiceHost = ""
			ret.PrometheusServicePort = ""
		}
	}
	if len(errs) > 0 {
		return ret, errs
	}

	return ret, nil
}

func validateHostPort(varNames []string, host, port string) error {
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, ValidateHostPortTimeout)
	if err != nil {
		return ConnectError{Type: ConnectErrorTypeAddress, VarNames: varNames, Value: addr, Reason: "If this is a development environment, make sure port forwarding has started", Err: err}
	} else {
		err = conn.Close()
		if err != nil {
			return ConnectError{Type: ConnectErrorTypeAddress, VarNames: varNames, Value: addr, Reason: "error closing connection to host", Err: err}
		}
		return nil
	}
}

func validateVar(varName string, extraTest func(string, string) error) (string, error) {
	value, ok := os.LookupEnv(varName)
	if !ok {
		return value, UnsetError{VarName: varName}
	}
	if value == "" {
		return value, EmptyError{VarName: varName}
	}
	if err := extraTest(varName, value); err != nil {
		fmt.Println(err)
		return value, err
	}
	return value, nil

}

func validateClaim(varName string, value string) error {
	if matched, _ := regexp.Match("^([[:alpha:]]*-)+([[:alpha:]]*)$", []byte(value)); !matched {
		return InvalidError{VarName: varName, Value: value, Reason: "PVC name should look like 'chantico-snmp-prometheus-volume-claim'"}
	}
	return nil

}

func validateLocation(varName string, value string) error {
	fileInfo, err := os.Stat(value)
	if err != nil {
		return InvalidError{VarName: varName, Value: value, Reason: "cannot find directory, should look like '.chantico-persistent-volume' or an absolute path", Err: err}
	}
	if !fileInfo.IsDir() {
		return InvalidError{VarName: varName, Value: value, Reason: "must be a directory, should look like '.chantico-persistent-volume' or an absolute path"}
	}
	return nil
}

func validateHost(varName string, value string) error {
	addrs, err := net.LookupHost(value)
	if err != nil {
		return ConnectError{Type: ConnectErrorTypeHost, VarNames: []string{varName}, Value: value, Reason: "error looking up host (is it a valid address?)", Err: err}
	}
	if len(addrs) == 0 {
		return ConnectError{Type: ConnectErrorTypeHost, VarNames: []string{varName}, Value: value, Reason: "lookup for host returned empty, is it a valid address?"}
	}
	return nil
}

func validatePort(varName string, value string) error {
	_, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return InvalidError{VarName: varName, Value: value, Reason: "error converting to a 16-bit integer, is it a valid port?", Err: err}
	}
	return nil
}

func lookupNamespace(envName string, allNamespacesAllowed bool) (string, error) {
	namespace, ok := os.LookupEnv(envName)
	if !ok {
		return namespace, UnsetError{VarName: envName}
	}
	if namespace == "" {
		return namespace, EmptyError{VarName: envName}
	}
	if allNamespacesAllowed && namespace == AllNamespaces {
		return namespace, nil
	}
	if errMsgs := validation.IsDNS1123Label(namespace); len(errMsgs) > 0 {
		return namespace, InvalidError{VarName: envName, Value: namespace, Reason: strings.Join(errMsgs, "; ")}
	}
	return namespace, nil
}

func validateNamespaces() (string, string, []error) {
	podNamespace, podErr := lookupNamespace(ChanticoNamespaceEnv, false)
	watchNamespace, watchErr := lookupNamespace(ChanticoWatchNamespaceEnv, true)
	if podErr != nil {
		return "", "", []error{podErr, watchErr}
	}
	if watchErr != nil {
		if errors.Is(watchErr, UnsetError{VarName: ChanticoWatchNamespaceEnv}) {
			return podNamespace, podNamespace, nil
		} else {
			return "", "", []error{watchErr}
		}
	}

	return podNamespace, watchNamespace, nil
}
