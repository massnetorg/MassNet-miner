package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"

	"github.com/massnetorg/mass-core/config"
)

const (
	defaultChainTag           = "mainnet"
	DefaultConfigFilename     = "config.json"
	DefaultLoggingFilename    = "mass"
	defaultShowVersion        = false
	DefaultDataDirname        = "chain"
	DefaultLogLevel           = "info"
	defaultLogDirname         = "logs"
	defaultMinerFileDirname   = "miner"
	defaultProofDirname       = "space"
	defaultDbType             = "leveldb"
	defaultPoCMinerBackend    = "sync"
	defaultSpaceKeeperBackend = "spacekeeper.v1"
	defaultListenAddress      = "tcp://0.0.0.0:43453"
	defaultDialTimeout        = 3
	defaultHandshakeTimeout   = 30
	MaxMiningPayoutAddresses  = 6000
	defaultAPIPortGRPC        = "9685"
	defaultAPIPortHttp        = "9686"
)

type Config struct {
	Chain     *config.Chain     `json:"chain"`
	Metrics   *config.Metrics   `json:"metrics"`
	P2P       *config.P2P       `json:"p2p"`
	Log       *config.Log       `json:"log"`
	Datastore *config.Datastore `json:"datastore"`
	API       *API              `json:"api"`
	Miner     *Miner            `json:"miner"`
}

func (cfg *Config) CoreConfig() *config.Config {
	return &config.Config{
		Chain:     cfg.Chain,
		Metrics:   cfg.Metrics,
		P2P:       cfg.P2P,
		Log:       cfg.Log,
		Datastore: cfg.Datastore,
	}
}

func DefaultConfig() *Config {
	return &Config{
		Chain:     DefaultChain(),
		Metrics:   DefaultMetrics(),
		P2P:       DefaultP2P(),
		Log:       DefaultLog(),
		Datastore: DefaultDatastore(),
		API:       DefaultAPI(),
		Miner:     DefaultMiner(),
	}
}

func DefaultChain() *config.Chain {
	return &config.Chain{
		DisableCheckpoints: false,
		AddCheckpoints:     []string{},
	}
}

func DefaultMetrics() *config.Metrics {
	return &config.Metrics{
		ProfilePort: "6060",
	}
}

func DefaultP2P() *config.P2P {
	return &config.P2P{
		Seeds:            "",
		AddPeer:          []string{},
		SkipUpnp:         false,
		HandshakeTimeout: 30,
		DialTimeout:      3,
		VaultMode:        false,
		ListenAddress:    "tcp://0.0.0.0:43453",
	}
}

func DefaultLog() *config.Log {
	return &config.Log{
		LogDir:        "logs",
		LogLevel:      "info",
		DisableCPrint: false,
	}
}

func DefaultDatastore() *config.Datastore {
	return &config.Datastore{
		Dir:    "chain",
		DBType: "leveldb",
	}
}

type API struct {
	PortGRPC   uint16   `json:"port_grpc"`
	PortHttp   uint16   `json:"port_http"`
	Whitelist  []string `json:"whitelist"`
	AllowedLan []string `json:"allowed_lan"`
}

func DefaultAPI() *API {
	return &API{
		PortGRPC:   9685,
		PortHttp:   9686,
		Whitelist:  []string{},
		AllowedLan: []string{},
	}
}

type Miner struct {
	PocminerBackend    string   `json:"pocminer_backend"`
	SpacekeeperBackend string   `json:"spacekeeper_backend"`
	MinerDir           string   `json:"miner_dir"`
	PayoutAddresses    []string `json:"payout_addresses"`
	Generate           bool     `json:"generate"`
	AllowSolo          bool     `json:"allow_solo"`
	ProofDir           []string `json:"proof_dir"`
	ProofList          string   `json:"proof_list"`
	Plot               bool     `json:"plot"`
	PublicPassword     string   `json:"public_password"`
	PrivatePassword    string   `json:"private_password"`
	ChiaMinerKeystore  string   `json:"chia_miner_keystore"`
}

func DefaultMiner() *Miner {
	return &Miner{
		PocminerBackend:    "sync",
		SpacekeeperBackend: "spacekeeper.v1",
		MinerDir:           "miner",
		PayoutAddresses:    []string{},
		Generate:           false,
		AllowSolo:          false,
		ProofDir:           []string{},
		ProofList:          "",
		Plot:               false,
		PublicPassword:     "123456",
		PrivatePassword:    "654321",
		ChiaMinerKeystore:  "chia-miner-keystore.json",
	}
}

func LoadConfig(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig()
	if err = json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func CheckConfig(cfg *Config) error {
	if cfg.Chain == nil {
		cfg.Chain = DefaultChain()
	}

	if cfg.Metrics == nil {
		cfg.Metrics = DefaultMetrics()
	}

	if cfg.P2P == nil {
		cfg.P2P = DefaultP2P()
	}

	if cfg.Log == nil {
		cfg.Log = DefaultLog()
	}

	if cfg.Datastore == nil {
		cfg.Datastore = DefaultDatastore()
	}

	if cfg.API == nil {
		cfg.API = DefaultAPI()
	}

	if cfg.Miner == nil {
		cfg.Miner = DefaultMiner()
	}

	// Checks for chain
	if !cfg.Chain.DisableCheckpoints {
		_, err := config.ParseCheckpoints(cfg.Chain.AddCheckpoints)
		if err != nil {
			return err
		}
	}

	// Checks for P2P
	cfg.P2P.Seeds = config.NormalizeSeeds(cfg.P2P.Seeds, "43453")

	// Checks for API
	for i, addr := range cfg.API.Whitelist {
		if addr == "*" {
			continue
		}
		if ip := net.ParseIP(addr); ip == nil {
			return errors.New(fmt.Sprintf("invalid api whitelist, %d, %s", i, addr))
		}
	}

	if cfg.Miner.Generate {
		if len(cfg.Miner.PayoutAddresses) == 0 {
			return errors.New("mining addr cannot be empty when generate set true")
		}
		if len(cfg.Miner.PayoutAddresses) > MaxMiningPayoutAddresses {
			return errors.New(fmt.Sprintln("mining addr cannot be more than", MaxMiningPayoutAddresses, "current", len(cfg.Miner.PayoutAddresses)))
		}
	}
	if cfg.Miner.ProofList != "" {
		_, err := DecodeProofList(cfg.Miner.ProofList)
		if err != nil {
			return err
		}
	}

	return nil
}
