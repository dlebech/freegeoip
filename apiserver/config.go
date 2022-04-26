// Copyright 2009 The freegeoip authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apiserver

import (
	"flag"
	"io"
	"log"
	"os"
	"time"

	"github.com/fiorix/freegeoip"
	"github.com/kelseyhightower/envconfig"
)

// Config is the configuration of the freegeoip server.
type Config struct {
	FastOpen         bool          `envconfig:"TCP_FAST_OPEN"`
	ServerAddr       string        `envconfig:"HTTP"`
	HSTS             string        `envconfig:"HSTS"`
	TLSServerAddr    string        `envconfig:"HTTPS"`
	TLSCertFile      string        `envconfig:"CERT"`
	TLSKeyFile       string        `envconfig:"KEY"`
	APIPrefix        string        `envconfig:"API_PREFIX"`
	CORSOrigin       string        `envconfig:"CORS_ORIGIN"`
	ReadTimeout      time.Duration `envconfig:"READ_TIMEOUT"`
	WriteTimeout     time.Duration `envconfig:"WRITE_TIMEOUT"`
	PublicDir        string        `envconfig:"PUBLIC"`
	DB               string        `envconfig:"DB"`
	UpdateInterval   time.Duration `envconfig:"UPDATE_INTERVAL"`
	RetryInterval    time.Duration `envconfig:"RETRY_INTERVAL"`
	UseXForwardedFor bool          `envconfig:"USE_X_FORWARDED_FOR"`
	Silent           bool          `envconfig:"SILENT"`
	LogToStdout      bool          `envconfig:"LOGTOSTDOUT"`
	LogTimestamp     bool          `envconfig:"LOGTIMESTAMP"`
}

// NewConfig creates and initializes a new Config with default values.
func NewConfig() *Config {
	return &Config{
		FastOpen:       false,
		ServerAddr:     ":8080",
		HSTS:           "",
		TLSCertFile:    "cert.pem",
		TLSKeyFile:     "key.pem",
		APIPrefix:      "/",
		CORSOrigin:     "*",
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   15 * time.Second,
		DB:             freegeoip.MaxMindDB,
		UpdateInterval: 24 * time.Hour,
		RetryInterval:  2 * time.Hour,
		LogTimestamp:   true,
	}
}

// AddFlags adds configuration flags to the given FlagSet.
func (c *Config) AddFlags(fs *flag.FlagSet) {
	defer envconfig.Process("freegeoip", c)
	fs.BoolVar(&c.FastOpen, "tcp-fast-open", c.FastOpen, "Enable TCP fast open")
	fs.StringVar(&c.ServerAddr, "http", c.ServerAddr, "Address in form of ip:port to listen on for HTTP")
	fs.StringVar(&c.HSTS, "hsts", c.HSTS, "Set HSTS to the value provided on all responses")
	fs.StringVar(&c.TLSServerAddr, "https", c.TLSServerAddr, "Address in form of ip:port to listen on for HTTPS")
	fs.StringVar(&c.TLSCertFile, "cert", c.TLSCertFile, "X.509 certificate file for HTTPS server")
	fs.StringVar(&c.TLSKeyFile, "key", c.TLSKeyFile, "X.509 key file for HTTPS server")
	fs.StringVar(&c.APIPrefix, "api-prefix", c.APIPrefix, "URL prefix for API endpoints")
	fs.StringVar(&c.CORSOrigin, "cors-origin", c.CORSOrigin, "Comma separated list of CORS origin API endpoints")
	fs.DurationVar(&c.ReadTimeout, "read-timeout", c.ReadTimeout, "Read timeout for HTTP and HTTPS client conns")
	fs.DurationVar(&c.WriteTimeout, "write-timeout", c.WriteTimeout, "Write timeout for HTTP and HTTPS client conns")
	fs.StringVar(&c.PublicDir, "public", c.PublicDir, "Public directory to serve at the {prefix}/ endpoint")
	fs.StringVar(&c.DB, "db", c.DB, "IP database file or URL")
	fs.DurationVar(&c.UpdateInterval, "update", c.UpdateInterval, "Database update check interval")
	fs.DurationVar(&c.RetryInterval, "retry", c.RetryInterval, "Max time to wait before retrying to download database")
	fs.BoolVar(&c.UseXForwardedFor, "use-x-forwarded-for", c.UseXForwardedFor, "Use the X-Forwarded-For header when available (e.g. behind proxy)")
	fs.BoolVar(&c.Silent, "silent", c.Silent, "Disable HTTP and HTTPS log request details")
	fs.BoolVar(&c.LogToStdout, "logtostdout", c.LogToStdout, "Log to stdout instead of stderr")
	fs.BoolVar(&c.LogTimestamp, "logtimestamp", c.LogTimestamp, "Prefix non-access logs with timestamp")
}

func (c *Config) logWriter() io.Writer {
	if c.LogToStdout {
		return os.Stdout
	}
	return os.Stderr
}

func (c *Config) errorLogger() *log.Logger {
	if c.LogTimestamp {
		return log.New(c.logWriter(), "[error] ", log.LstdFlags)
	}
	return log.New(c.logWriter(), "[error] ", 0)
}

func (c *Config) accessLogger() *log.Logger {
	return log.New(c.logWriter(), "[access] ", 0)
}
