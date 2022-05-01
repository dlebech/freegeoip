// Copyright 2009 The freegeoip authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apiserver

import (
	"flag"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fiorix/freegeoip"
	"github.com/kelseyhightower/envconfig"
)

// Config is the configuration of the freegeoip server.
type Config struct {
	FastOpen         bool          `envconfig:"TCP_FAST_OPEN"`
	Host             string        `envconfig:"HOST"`
	Port             int           `envconfig:"PORT"`
	ReadTimeout      time.Duration `envconfig:"READ_TIMEOUT"`
	WriteTimeout     time.Duration `envconfig:"WRITE_TIMEOUT"`
	DB               string        `envconfig:"DB"`
	UseXForwardedFor bool          `envconfig:"USE_X_FORWARDED_FOR"`
	Silent           bool          `envconfig:"SILENT"`
	LogToStdout      bool          `envconfig:"LOGTOSTDOUT"`
	LogTimestamp     bool          `envconfig:"LOGTIMESTAMP"`
}

func (c *Config) ServerAddr() string {
	return strings.Join([]string{c.Host, strconv.Itoa(c.Port)}, ":")
}

// NewConfig creates and initializes a new Config with default values.
func NewConfig() *Config {
	return &Config{
		FastOpen:     false,
		Host:         "",
		Port:         8080,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 15 * time.Second,
		DB:           freegeoip.MaxMindDBURL,
		LogTimestamp: true,
	}
}

// AddFlags adds configuration flags to the given FlagSet.
func (c *Config) AddFlags(fs *flag.FlagSet) {
	defer envconfig.Process("", c)
	fs.BoolVar(&c.FastOpen, "tcp-fast-open", c.FastOpen, "Enable TCP fast open")
	fs.StringVar(&c.Host, "host", c.Host, "Hostname of server. Default empty")
	fs.IntVar(&c.Port, "port", c.Port, "Port to listen to. Default 8080")
	fs.DurationVar(&c.ReadTimeout, "read-timeout", c.ReadTimeout, "Read timeout for HTTP and HTTPS client conns")
	fs.DurationVar(&c.WriteTimeout, "write-timeout", c.WriteTimeout, "Write timeout for HTTP and HTTPS client conns")
	fs.StringVar(&c.DB, "db", c.DB, "IP database file or URL")
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
