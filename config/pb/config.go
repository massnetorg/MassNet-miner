package configpb

func NewConfig() *Config {
	return &Config{
		App: &AppConfig{},
		Network: &NetworkConfig{
			P2P: &P2PConfig{
				AddPeer: make([]string, 0),
			},
			API: &APIConfig{
				APIWhitelist:  make([]string, 0),
				APIAllowedLan: make([]string, 0),
			},
		},
		Db:  &DataConfig{},
		Log: &LogConfig{},
		Miner: &MinerConfig{
			MiningAddr: make([]string, 0),
			ProofDir:   make([]string, 0),
		},
	}
}
