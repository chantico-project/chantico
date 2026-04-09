package ipmidevice

import (
	"maps"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v2"

	chantico "chantico/api/v1alpha1"
	vol "chantico/internal/volumes"
)

const (
	ipmiDir    = "ipmi"
	ipmiYmlDir = "ipmi/yml"
)

func getConfigPath() string {
	return filepath.Join(
		os.Getenv(vol.ChanticoVolumeLocationEnv),
		ipmiYmlDir,
		"config.yml",
	)
}

type ipmiConfig struct {
	Modules map[string]chantico.IPMIConfig `yaml:"modules"`
}

func MergeIPMIConfigs(fileContents [][]byte) (string, error) {
	acc := ipmiConfig{Modules: map[string]chantico.IPMIConfig{}}
	for _, fileContent := range fileContents {
		ipmiconfig := ipmiConfig{Modules: map[string]chantico.IPMIConfig{}}
		err := yaml.Unmarshal(fileContent, &ipmiconfig)
		if err != nil {
			return "", err
		}
		maps.Copy(acc.Modules, ipmiconfig.Modules)
	}
	out, err := yaml.Marshal(acc)
	if err != nil {
		return "", err
	}
	return string(out), err
}

func DeleteIPMIConfigModule(
	fileContent []byte,
	moduleName string,
) (string, error) {
	acc := ipmiConfig{Modules: map[string]chantico.IPMIConfig{}}
	ipmiconfig := ipmiConfig{Modules: map[string]chantico.IPMIConfig{}}
	err := yaml.Unmarshal(fileContent, &ipmiconfig)
	if err != nil {
		return "", err
	}
	for name, config := range ipmiconfig.Modules {
		if name == moduleName {
			continue
		}
		acc.Modules[name] = config
	}
	out, err := yaml.Marshal(acc)
	if err != nil {
		return "", err
	}
	return string(out), err
}
