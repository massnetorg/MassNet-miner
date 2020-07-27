# v1.1.0

## How to Upgrade
See [cmd/database-upgrade/1.1.0/USAGE.md](https://github.com/massnetorg/MassNet-miner/tree/1.1/cmd/database-upgrade/1.1.0/USAGE.md).

## Notable Changes

### Consensus
* Staking nodes share reward by weight since height 694000. And
* Staking period will be treated as 1474560 if it's over 1474560 when calculating weight.

### Storage
* Save blocks to disk instead of database.

### Mining
* Support configuring capacity for each mining path.
