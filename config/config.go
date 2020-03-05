package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/btcsuite/go-flags"
	configpb "massnet.org/mass/config/pb"
	"massnet.org/mass/consensus"
	"massnet.org/mass/poc"
	"massnet.org/mass/version"
	"massnet.org/mass/wire"
)

const (
	defaultChainTag           = "mainnet"
	DefaultConfigFilename     = "config.json"
	defaultShowVersion        = false
	DefaultDataDirname        = "chain"
	DefaultLogLevel           = "info"
	defaultLogDirname         = "logs"
	defaultMinerFileDirname   = "miner"
	defaultProofDirname       = "space"
	DefaultLoggingFilename    = "mass"
	defaultDbType             = "leveldb"
	defaultPoCMinerBackend    = "sync"
	defaultSpaceKeeperBackend = "spacekeeper.v1"
	defaultBlockMinSize       = 0
	defaultBlockMaxSize       = wire.MaxBlockPayload
	defaultBlockPrioritySize  = consensus.DefaultBlockPrioritySize
	defaultListenAddress      = "tcp://0.0.0.0:43453"
	defaultDialTimeout        = 3
	defaultHandshakeTimeout   = 30
	MaxMiningPayoutAddresses  = 6000
	defaultAPIPortGRPC        = "9685"
	defaultAPIPortHttp        = "9686"
)

var (
	FreeTxRelayLimit         = 15.0
	AddrIndex                = true
	NoRelayPriority          = true
	BlockPrioritySize uint32 = defaultBlockPrioritySize
	BlockMinSize      uint32 = defaultBlockMinSize
	BlockMaxSize      uint32 = defaultBlockMaxSize
	MaxPeers                 = 50
	Moniker                  = "anonymous"
	ChainTag                 = defaultChainTag
)

var (
	MassHomeDir                  = AppDataDir("mass", false)
	defaultConfigFile            = DefaultConfigFilename
	defaultDataDir               = DefaultDataDirname
	knownDbTypes                 = []string{"leveldb", "memdb"}
	defaultMinerFileDir          = defaultMinerFileDirname
	defaultProofDir              = defaultProofDirname
	defaultLogDir                = defaultLogDirname
	HDCoinTypeMassMainNet uint32 = 297
)

// RunServiceCommand is only set to a real function on Windows.  It is used
// to parse and execute service commands specified via the -s flag.
var RunServiceCommand func(string) error

// serviceOptions defines the configuration options for mass as a service on
// Windows.
type serviceOptions struct {
	ServiceCommand string `short:"s" long:"service" description:"Service command {install, remove, start, stop}"`
}

type Config struct {
	*configpb.Config
	ConfigFile  string `short:"C" long:"configfile" description:"Path to configuration file"`
	ShowVersion bool   `short:"V" long:"version" description:"Display Version information and exit"`
	Generate    bool   `long:"generate" description:"Generate (mine) coins when start"`
	Init        bool   `long:"init" description:"Init miner keystore"`
	PrivatePass string `short:"P" long:"privpass" description:"Private passphrase for miner"`
}

// newConfigParser returns a new command line flags parser.
func newConfigParser(cfg *Config, so *serviceOptions, options flags.Options) *flags.Parser {
	parser := flags.NewParser(cfg, options)
	if runtime.GOOS == "windows" {
		parser.AddGroup("Service Options", "Service Options", so)
	}
	return parser
}

// ParseConfig reads and parses the config using a Config file and command
// line options.
// This func proceeds as follows:
//  1) Start with a default config with sane settings
//  2) Pre-parse the command line to check for an alternative config file
func ParseConfig() (*Config, []string, error) {
	// Default config.
	cfg := Config{
		ConfigFile:  defaultConfigFile,
		ShowVersion: defaultShowVersion,
		Config:      configpb.NewConfig(),
	}

	// Service options which are only added on Windows.
	serviceOpts := serviceOptions{}

	// Pre-parse the command line options to see if an alternative config
	// file or the version flag was specified.  Any errors aside from the
	// help message error can be ignored here since they will be caught by
	// the final parse below.
	preCfg := cfg
	preParser := newConfigParser(&preCfg, &serviceOpts, flags.HelpFlag)
	_, err := preParser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}
	}

	// Show the version and exit if the Version flag was specified.
	//appName := filepath.Base(os.Args[0])
	//appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	appName := "mass-client"
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)
	if preCfg.ShowVersion {
		fmt.Println(appName, "version", version.GetVersion())
		os.Exit(0)
	}

	// Perform service command and exit if specified.  Invalid service
	// commands show an appropriate error.  Only runs on Windows since
	// the RunServiceCommand function will be nil when not on Windows.
	if serviceOpts.ServiceCommand != "" && RunServiceCommand != nil {
		err := RunServiceCommand(serviceOpts.ServiceCommand)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(0)
	}

	// Load additional config from file.
	parser := newConfigParser(&cfg, &serviceOpts, flags.Default)

	// Parse command line options again to ensure they take precedence.
	remainingArgs, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, usageMessage)
		}
		return nil, nil, err
	}

	return &cfg, remainingArgs, nil
}

func LoadConfig(cfg *Config) (*Config, error) {
	b, err := ioutil.ReadFile(cfg.ConfigFile)
	if err != nil {
		return cfg, err
	}

	if err := json.Unmarshal(b, cfg.Config); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func CheckConfig(cfg *Config) (*Config, error) {
	if cfg.App == nil {
		cfg.App = new(configpb.AppConfig)
	}
	if cfg.Network == nil {
		cfg.Network = new(configpb.NetworkConfig)
	}
	if cfg.Network.P2P == nil {
		cfg.Network.P2P = new(configpb.P2PConfig)
		cfg.Network.P2P.AddPeer = make([]string, 0)
	}
	if cfg.Network.API == nil {
		cfg.Network.API = new(configpb.APIConfig)
	}
	if cfg.Db == nil {
		cfg.Db = new(configpb.DataConfig)
	}
	if cfg.Log == nil {
		cfg.Log = new(configpb.LogConfig)
	}
	if cfg.Miner == nil {
		cfg.Miner = new(configpb.MinerConfig)
	}
	if cfg.Miner.MiningAddr == nil {
		cfg.Miner.MiningAddr = make([]string, 0)
	}
	if cfg.Miner.ProofDir == nil {
		cfg.Miner.ProofDir = make([]string, 0)
	}

	// Checks for AppConfig
	if cfg.App.Hostname == "" {
		cfg.App.Hostname, _ = os.Hostname()
	}

	// Checks for APIConfig
	if cfg.Network.API.APIPortHttp == "" {
		cfg.Network.API.APIPortHttp = defaultAPIPortHttp
	}
	if cfg.Network.API.APIPortGRPC == "" {
		cfg.Network.API.APIPortGRPC = defaultAPIPortGRPC
	}
	if cfg.Network.API.APIWhitelist == nil {
		cfg.Network.API.APIWhitelist = make([]string, 0)
	}
	if cfg.Network.API.APIAllowedLan == nil {
		cfg.Network.API.APIAllowedLan = make([]string, 0)
	}
	// TODO: remove duplicate items
	for i, addr := range cfg.Network.API.APIWhitelist {
		if addr == "*" {
			continue
		}
		if ip := net.ParseIP(addr); ip == nil {
			return cfg, errors.New(fmt.Sprintf("invalid api whitelist, %d, %s", i, addr))
		}
	}
	// TODO: add ip:port match
	// Checks for P2PConfig
	cfg.Network.P2P.Seeds = NormalizeSeeds(cfg.Network.P2P.Seeds, ChainParams.DefaultPort)
	if cfg.Network.P2P.ListenAddress == "" {
		cfg.Network.P2P.ListenAddress = defaultListenAddress
	}
	if cfg.Network.P2P.DialTimeout == 0 {
		cfg.Network.P2P.DialTimeout = defaultDialTimeout
	}
	if cfg.Network.P2P.HandshakeTimeout == 0 {
		cfg.Network.P2P.HandshakeTimeout = defaultHandshakeTimeout
	}

	var dealWithDir = func(path string) string {
		return cleanAndExpandPath(path)
	}

	// Checks for DataConfig
	if cfg.Db.DbType == "" {
		cfg.Db.DbType = defaultDbType
	}
	if !validDbType(cfg.Db.DbType) {
		return cfg, errors.New(fmt.Sprintf("invalid db_type %s", cfg.Db.DbType))
	}
	if cfg.Db.DataDir == "" {
		cfg.Db.DataDir = defaultDataDir
	}
	cfg.Db.DataDir = dealWithDir(cfg.Db.DataDir)

	// Checks for LogConfig
	if cfg.Log.LogDir == "" {
		cfg.Log.LogDir = defaultLogDir
	}
	cfg.Log.LogDir = dealWithDir(cfg.Log.LogDir)
	if cfg.Log.LogLevel == "" {
		cfg.Log.LogLevel = DefaultLogLevel
	}

	// Checks for MinerConfig
	if cfg.Miner.SpacekeeperBackend == "" {
		cfg.Miner.SpacekeeperBackend = defaultSpaceKeeperBackend
	}
	if cfg.Miner.PocminerBackend == "" {
		cfg.Miner.PocminerBackend = defaultPoCMinerBackend
	}
	if cfg.Generate {
		cfg.Miner.Generate = true
	}
	if cfg.Miner.Generate {
		if len(cfg.Miner.MiningAddr) == 0 {
			return cfg, errors.New("mining addr cannot be empty when generate set true")
		}
		if len(cfg.Miner.MiningAddr) > MaxMiningPayoutAddresses {
			return cfg, errors.New(fmt.Sprintln("mining addr cannot be more than", MaxMiningPayoutAddresses, "current", len(cfg.Miner.MiningAddr)))
		}
		if cfg.Miner.PocminerBackend == defaultPoCMinerBackend && cfg.Miner.PrivatePassword == "" {
			return cfg, errors.New("private password cannot be empty when generate set true")
		}
	}
	if cfg.Miner.MinerDir == "" {
		cfg.Miner.MinerDir = defaultMinerFileDir
	}
	cfg.Miner.MinerDir = dealWithDir(cfg.Miner.MinerDir)
	if len(cfg.Miner.ProofDir) == 0 {
		cfg.Miner.ProofDir = append(cfg.Miner.ProofDir, defaultProofDir)
	}
	for i, dir := range cfg.Miner.ProofDir {
		cfg.Miner.ProofDir[i] = dealWithDir(dir)
	}
	if cfg.Miner.ProofList != "" {
		_, err := DecodeProofList(cfg.Miner.ProofList)
		if err != nil {
			return cfg, err
		}
	}

	//jBytes, err := json.Marshal(cfg.Config)
	//if err != nil {
	//	fmt.Println(err)
	//	return cfg, nil
	//}
	//fmt.Println(string(jBytes))
	return cfg, nil
}

// cleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		homeDir := filepath.Dir(MassHomeDir)
		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but they variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

// DecodeProofList decodes config proof list in format like this: "<BL>:<Count>,<BL>:<Count>".
// Example: 3 spaces of BL = 24, 4 spaces of BL= 26, 5 spaces of BL = 28 would be like "24:3, 26:4, 28:5".
// Duplicate BL would be added together, "24:1, 24:2, 26:5" equals to "24:3, 26:5".
func DecodeProofList(proofList string) (map[int]int, error) {
	conf := make(map[int]int)
	proofList = strings.Replace(proofList, " ", "", -1)
	for _, item := range strings.Split(proofList, ",") {
		desc := strings.Split(item, ":")
		if len(desc) != 2 {
			return nil, errors.New(fmt.Sprintln("invalid proof list item, wrong format", proofList, desc))
		}
		bl, err := strconv.Atoi(desc[0])
		if err != nil {
			return nil, errors.New(fmt.Sprintln("invalid proof list bitLength, wrong bitLength str", desc, desc[0]))
		}
		if !poc.EnsureBitLength(bl) {
			return nil, errors.New(fmt.Sprintln("invalid proof list bitLength", desc, bl))
		}
		count, err := strconv.Atoi(desc[1])
		if err != nil {
			return nil, errors.New(fmt.Sprintln("invalid proof list spaceCount, wrong spaceCount str", desc, desc[1]))
		}
		if count < 0 {
			return nil, errors.New(fmt.Sprintln("invalid proof list spaceCount", desc, count))
		}
		if _, exists := conf[bl]; exists {
			conf[bl] += count
		} else {
			conf[bl] = count
		}
	}
	return conf, nil
}

// validDbType returns whether or not dbType is a supported database type.
func validDbType(dbType string) bool {
	for _, knownType := range knownDbTypes {
		if dbType == knownType {
			return true
		}
	}

	return false
}
