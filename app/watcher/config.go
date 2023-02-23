package watcher

import "time"

type Config struct {
	Listen string               `yaml:"listen"`
	Jobs   map[string]JobConfig `yaml:"jobs"`
}

type JobConfig struct {
	Addr     string        `yaml:"addr"`
	Auth     string        `yaml:"auth"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}
