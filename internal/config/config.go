package config

import (
	"fmt"
	"os"

	"github.com/wiz/sendsmtp/internal/inboxcheck"
	"gopkg.in/yaml.v3"
)

type Paths struct {
	Smtps    string `yaml:"smtps" json:"smtps"`
	Emails   string `yaml:"emails" json:"emails"`
	Subjects string `yaml:"subjects" json:"subjects"`
	Links    string `yaml:"links" json:"links"`
	HTML     string `yaml:"html" json:"html"`
}

type InboxCheckConfig struct {
	Headless   bool              `yaml:"headless" json:"headless"`
	WaitSec    int               `yaml:"wait_sec" json:"wait_sec"`
	TimeoutSec int               `yaml:"timeout_sec" json:"timeout_sec"`
	Seeds      []inboxcheck.Seed `yaml:"seeds" json:"seeds"`
}

// ShortenerConfig controls abre.ai link rotation during campaigns.
type ShortenerConfig struct {
	Enabled     bool `yaml:"enabled" json:"enabled"`
	EveryN      int  `yaml:"every_n" json:"every_n"`           // refresh pool after N successful sends
	BatchSize   int  `yaml:"batch_size" json:"batch_size"`     // shorts to generate per refresh
	Concurrency int  `yaml:"concurrency" json:"concurrency"` // parallel abre.ai calls
}

type Config struct {
	Database              string           `yaml:"database" json:"database"`
	Workers               int              `yaml:"workers" json:"workers"`
	SMTPMaxConn           int              `yaml:"smtp_max_conn" json:"smtp_max_conn"`
	DialTimeoutSec        int              `yaml:"dial_timeout_sec" json:"dial_timeout_sec"`
	SendTimeoutSec        int              `yaml:"send_timeout_sec" json:"send_timeout_sec"`
	RetryMax              int              `yaml:"retry_max" json:"retry_max"`
	RetryBackoffSec       int              `yaml:"retry_backoff_sec" json:"retry_backoff_sec"`
	SMTPDisableAfterFails int              `yaml:"smtp_disable_after_fails" json:"smtp_disable_after_fails"`
	BatchCommit           int              `yaml:"batch_commit" json:"batch_commit"`
	FromName              string           `yaml:"from_name" json:"from_name"`
	RatePerSMTPPerMin     int              `yaml:"rate_per_smtp_per_min" json:"rate_per_smtp_per_min"`
	InboxCheck            InboxCheckConfig `yaml:"inbox_check" json:"inbox_check"`
	Shortener             ShortenerConfig  `yaml:"shortener" json:"shortener"`
	Paths                 Paths            `yaml:"paths" json:"paths"`
}

func Default() Config {
	return Config{
		Database:              "data/sendsmtp.db",
		Workers:               50,
		SMTPMaxConn:           2,
		DialTimeoutSec:        10,
		SendTimeoutSec:        30,
		RetryMax:              3,
		RetryBackoffSec:       2,
		SMTPDisableAfterFails: 5,
		BatchCommit:           100,
		FromName:              "",
		RatePerSMTPPerMin:     0,
		InboxCheck: InboxCheckConfig{
			Headless:   true,
			WaitSec:    60,
			TimeoutSec: 240,
			Seeds:      nil,
		},
		Shortener: ShortenerConfig{
			Enabled:     false,
			EveryN:      100,
			BatchSize:   10,
			Concurrency: 6,
		},
		Paths: Paths{
			Smtps:    "data/smtps.txt",
			Emails:   "data/emails.txt",
			Subjects: "data/assuntos.txt",
			Links:    "data/links.txt",
			HTML:     "data/msg.html",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	cfg.ApplyDefaults()
	return cfg, nil
}

func (c *Config) ApplyDefaults() {
	d := Default()
	if c.Database == "" {
		c.Database = d.Database
	}
	if c.Workers <= 0 {
		c.Workers = d.Workers
	}
	if c.SMTPMaxConn <= 0 {
		c.SMTPMaxConn = d.SMTPMaxConn
	}
	if c.DialTimeoutSec <= 0 {
		c.DialTimeoutSec = d.DialTimeoutSec
	}
	if c.SendTimeoutSec <= 0 {
		c.SendTimeoutSec = d.SendTimeoutSec
	}
	if c.RetryMax < 0 {
		c.RetryMax = d.RetryMax
	}
	if c.RetryBackoffSec <= 0 {
		c.RetryBackoffSec = d.RetryBackoffSec
	}
	if c.SMTPDisableAfterFails <= 0 {
		c.SMTPDisableAfterFails = d.SMTPDisableAfterFails
	}
	if c.BatchCommit <= 0 {
		c.BatchCommit = d.BatchCommit
	}
	if c.Paths.Smtps == "" {
		c.Paths.Smtps = d.Paths.Smtps
	}
	if c.Paths.Emails == "" {
		c.Paths.Emails = d.Paths.Emails
	}
	if c.Paths.Subjects == "" {
		c.Paths.Subjects = d.Paths.Subjects
	}
	if c.Paths.Links == "" {
		c.Paths.Links = d.Paths.Links
	}
	if c.Paths.HTML == "" {
		c.Paths.HTML = d.Paths.HTML
	}
	if c.InboxCheck.WaitSec <= 0 {
		c.InboxCheck.WaitSec = d.InboxCheck.WaitSec
	}
	if c.InboxCheck.TimeoutSec <= 0 {
		c.InboxCheck.TimeoutSec = d.InboxCheck.TimeoutSec
	}
	if c.Shortener.EveryN <= 0 {
		c.Shortener.EveryN = d.Shortener.EveryN
	}
	if c.Shortener.BatchSize <= 0 {
		c.Shortener.BatchSize = d.Shortener.BatchSize
	}
	if c.Shortener.Concurrency <= 0 {
		c.Shortener.Concurrency = d.Shortener.Concurrency
	}
}

func Save(path string, cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
