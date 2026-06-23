package snmp

// Courtesy of the snmp_exporter repository: https://github.com/prometheus/snmp_exporter/blob/main/config/config.go
type GeneratorAuth struct {
	Community     string `yaml:"community,omitempty" json:"community,omitempty"`
	SecurityLevel string `yaml:"security_level,omitempty" json:"security_level,omitempty"`
	Username      string `yaml:"username,omitempty" json:"username,omitempty"`
	Password      string `yaml:"password,omitempty" json:"password,omitempty"`
	AuthProtocol  string `yaml:"auth_protocol,omitempty" json:"auth_protocol,omitempty"`
	PrivProtocol  string `yaml:"priv_protocol,omitempty" json:"priv_protocol,omitempty"`
	PrivPassword  string `yaml:"priv_password,omitempty" json:"priv_password,omitempty"`
	ContextName   string `yaml:"context_name,omitempty" json:"context_name,omitempty"`
	Version       int    `yaml:"version,omitempty" json:"version,omitempty"`
}

type GeneratorModule struct {
	Walk []string `yaml:"walk"`
}

type GeneratorConfig struct {
	Auths   map[string]*GeneratorAuth   `yaml:"auths"`
	Modules map[string]*GeneratorModule `yaml:"modules"`
	Version int                         `yaml:"version,omitempty"`
}

type MergedConfig struct {
	Auths   map[string]any `yaml:"auths" json:"auths"`
	Modules map[string]any `yaml:"modules" json:"modules"`
	Version int            `yaml:"version,omitempty" json:"version,omitempty"`
}
