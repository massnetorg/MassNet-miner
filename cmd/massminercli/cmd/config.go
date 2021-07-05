package cmd

import (
	"github.com/massnetorg/mass-core/logging"
	"github.com/spf13/viper"
)

const (
	defaultAPIURL      = "http://localhost:9686"
	defaultLogDir      = "masscli-logs"
	defaultLogFilename = "masscli"
	defaultLogLevel    = "info"
)

var (
	flagAPIURL      string
	flagLogDir      string
	flagLogLevel    string
	cfgFile         string
	usingConfigFile bool
	config          = new(Config)
)

type Config struct {
	APIURL   string `json:"api_url"`
	LogDir   string `json:"log_dir"`
	LogLevel string `json:"log_level"`
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath("./")
		viper.SetConfigName(".masscli")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		usingConfigFile = true
	}

	// Load config to memory.
	config.APIURL = viper.GetString("api_url")
	if config.APIURL == "" {
		config.APIURL = defaultAPIURL
	}
	config.LogDir = viper.GetString("log_dir")
	if config.LogDir == "" {
		config.LogDir = defaultLogDir
	}
	config.LogLevel = viper.GetString("log_level")
	if config.LogLevel == "" {
		config.LogLevel = defaultLogLevel
	}
}

// initLogger initializes logging module by config.
func initLogger() {
	logging.Init(config.LogDir, defaultLogFilename, config.LogLevel, 1, false)
}

// logBasicInfo logs the basic info on initializing.
func logBasicInfo() {
	logging.CPrint(logging.INFO, "using config file", logging.LogFormat{"file": usingConfigFile})
}
