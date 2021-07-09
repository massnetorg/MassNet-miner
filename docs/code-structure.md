# MassNet-Miner Code Structure

`MassNet Miner` is a Golang implementation of MassNet full-node miner.

This project introduces some mining related modules to support solo miners.

## Package Usages

- **api**: provides gRPC and HTTP interfaces to fetch blockchain information and interact with poc-miner.

- **cmd/massminercli**: provides a command line tool that wrapped some HTTP APIs.

- **conf**: provides sample config file.

- **config**: defines the configuration structure of MassNet-miner.

- **docs**: includes release notes and API documentation.

- **mining**: is a adaptor module between API and real PoC functionalities.

- **poc/engine**: contains the implementation of miner for native MASS Plot files.

    - **poc/engine/massdb**: is the container for native MASS Plot files from which proof-of-capacity can be extracted.
    - **poc/engine/spacekeeper**: is the manager for clusters of massdb instances. SpaceKeeper supports sync/async proof query interfaces.
    - **poc/engine/pocminer**: is the producer of mining blocks. PocMiner read proofs from SpaceKeeper and submit new eligible blocks to the network.

- **poc/engine.v2**: contains the implementation of miner.v2 for Chia Plot files.

    - **poc/engine.v2/massdb**: is the container for Chia Plot files from which proof-of-capacity can be extracted.
    - **poc/engine.v2/spacekeeper**: is the manager for clusters of massdb instances. SpaceKeeper supports sync/async proof query interfaces.
    - **poc/engine.v2/pocminer**: is the producer of mining blocks. PocMiner read proofs from SpaceKeeper and submit new eligible blocks to the network.

- **poc/wallet**: contains the keystore manager for native MASS Plot files.

- **server**: initializes miner service.

- **version**: specifies the version of current MassNet-miner.
