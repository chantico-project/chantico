package prometheus

type ScrapeConfigFile struct {
	ScrapeConfigs []ScrapeConfig `yaml:"scrape_configs" json:"scrape_configs"`
}

type ScrapeConfig struct {
	JobName        string          `yaml:"job_name" json:"job_name"`
	StaticConfigs  []StaticConfig  `yaml:"static_configs,omitempty" json:"static_configs,omitempty"`
	FileSdConfigs  []FileSdConfig  `yaml:"file_sd_configs,omitempty" json:"file_sd_configs,omitempty"`
	MetricsPath    *string         `yaml:"metrics_path,omitempty" json:"metrics_path,omitempty"`
	RelabelConfigs []RelabelConfig `yaml:"relabel_configs,omitempty" json:"relabel_configs,omitempty"`
}

type StaticConfig struct {
	Targets []string `yaml:"targets,omitempty" json:"targets,omitempty"`
}

type FileSdConfig struct {
	Files           []string `yaml:"files,omitempty" json:"files,omitempty"`
	RefreshInterval *string  `yaml:"refresh_interval,omitempty" json:"refresh_interval,omitempty"`
}

type RelabelConfig struct {
	SourceLabels []string `yaml:"source_labels,omitempty" json:"source_labels,omitempty"`
	TargetLabel  *string  `yaml:"target_label,omitempty" json:"target_label,omitempty"`
	Replacement  *string  `yaml:"replacement,omitempty" json:"replacement,omitempty"`
}
