# MassNet Cluster-Mining Guide

The deployment procedures of mining has been simplified by introducing cluster mining.

## Role Specification

In cluster-mining, a single `FullNode Miner` can interact with thousands of `Fractal Miner`.

- `FullNode Miner` maintains the peer-to-peer blockchain network and stores the full blockchain ledger. FullNode Miners build new blocks and broadcast them to the network.

- `Fractal Miner` neither joins the peer-to-peer blockchain network nor keeps the ledger. Fractal Miners only interact with FullNode Miners or Fractal Miners.

- `Superior` is an interface that dispatches mining tasks and collects mining results.

- `Collector` is an interface that works on mining tasks and reports mining results.

- `Relay` is an abstract role that relays mining tasks and mining results.

## Command Line Usage

### FullNode Miner

```
NAME:
   massminer m2 - Run massminer in m2 mode (mine blocks with Chia DiskProver)

USAGE:
   massminer m2 [command options] [arguments...]

OPTIONS:
   --pool value, -p value  specify collector_pool listen address by <ip:port>
   --max-collector value   specify the max collector number in collector_pool (default: 2000)
   --no-local-collector    do not run local collector (default: false)
   --help, -h              show help (default: false)
```

### Fractal Miner

```
NAME:
   massminer fractal - Run in fractal mode

USAGE:
   massminer fractal [command options] [arguments...]

OPTIONS:
   --superior value, -s value  specify remote_superior listen address by <ip:port>
   --pool value, -p value      specify collector_pool listen address by <ip:port>
   --max-collector value       specify the max collector number in collector_pool (default: 2000)
   --no-local-collector        do not run local collector (default: false)
   --help, -h                  show help (default: false)
```

## Examples of Mining topology

### 1. Single FullNode Miner

Considering that all we have is a single FullNode Miner.

Modify [sample-config.m2.json](../conf/sample-config.m2.json) with your payout_addresses, and the directories storing plot files.

Rename `sample-config.m2.json` to `config.json`.

Run the following command to start mining.

```shell
./massminer m2
```

### 2. FullNode Miner with Fractal Miners

Considering that we have four machines:

- `192.168.1.101` : equipped with SSD and nice network.
- `192.168.1.102`, `192.168.1.103`, `192.168.1.104` : stores plot files.

Then we should run `192.168.1.101` as FullNode Miner, and run other machines as Fractal Miners, as it shown below.

```
                                <--- 192.168.1.102 (Fractal)
                               /
192.168.1.101 (FullNode) <--- * <--- 192.168.1.103 (Fractal)
                               \
                                <--- 192.168.1.104 (Fractal)
```

**2.1 Run FullNode Miner**

Configure and run FullNode Miner on `192.168.1.101`.

Modify [sample-config.m2.json](../conf/sample-config.m2.json) with your payout_addresses, and the directories storing plot files.

Rename `sample-config.m2.json` to `config.json`.

Run the following command to start mining.

```shell
# Run this command if 192.168.1.101 has plot files
./massminer m2 -p 192.168.1.101:9690

# Or run this command if 192.168.1.101 does not have plot files
./massminer m2 -p 192.168.1.101:9690 --no-local-collector
```

**2.2 Run Fractal Miner**

Configure and run Fractal Miner on `192.168.1.102`, `192.168.1.103`, `192.168.1.104`.

Modify [sample-config.fractal.json](../conf/sample-config.fractal.json) with the directories storing plot files.

Rename `sample-config.fractal.json` to `config.json`.

Run the following command to start.

```shell
./massminer fractal -s 192.168.1.101:9690
```

### 3. FullNode Miner with Fractal Miners and Relay

Considering that we have five machines:

- `192.168.1.101` : equipped with SSD and nice network.
- `192.168.1.102` : stores plot files.
- `192.168.1.103` : stores plot files and has another network interface `172.16.1.103`.
- `172.16.1.104`, `172.16.1.105` : stores plot files.

Then we should run `192.168.1.101` as FullNode Miner, and run other machines as Fractal Miners. However, `172.16.1.104` cannot directly connect to `192.168.1.101`, so we need to use `192.168.1.103` as a `Relay`, as it shown below.

```
                                <--- 192.168.1.102 (Fractal)
                               /
192.168.1.101 (FullNode) <--- * <--- 192.168.1.103 (* 172.16.1.103) (Fractal)
                                                     \
                                                      * <--- 172.16.1.104 (Fractal)
                                                       \
                                                        <--- 172.16.1.105 (Fractal)
```

**3.1 Run FullNode Miner**

Configure and run FullNode Miner on `192.168.1.101`.

Modify [sample-config.m2.json](../conf/sample-config.m2.json) with your payout_addresses, and the directories storing plot files.

Rename `sample-config.m2.json` to `config.json`.

Run the following command to start mining.

```shell
# Run this command if 192.168.1.101 has plot files
./massminer m2 -p 192.168.1.101:9690

# Or run this command if 192.168.1.101 does not have plot files
./massminer m2 -p 192.168.1.101:9690 --no-local-collector
```

**3.2 Run Fractal Miner**

Configure and run Fractal Miner on `192.168.1.102`, `192.168.1.103`, `172.16.1.104`, `172.16.1.105`.

Modify [sample-config.fractal.json](../conf/sample-config.fractal.json) with the directories storing plot files.

Rename `sample-config.fractal.json` to `config.json`.

Run the following command to start.

**3.2.1 Run Fractal Miner on 192.168.1.102**

```shell
./massminer fractal -s 192.168.1.101:9690
```

**3.2.2 Run Fractal Miner with Relay on 192.168.1.103**

```shell
# Run this command if 192.168.1.103 has plot files
./massminer fractal -s 192.168.1.101:9690 -p 172.16.1.103:9690

# Run this command if 192.168.1.103 does not have plot files
./massminer fractal -s 192.168.1.101:9690 -p 172.16.1.103:9690 --no-local-collector
```

**3.2.3 Run Fractal Miner on 172.16.1.104 and 172.16.1.105**

```shell
./massminer fractal -s 172.16.1.103:9690
```

## FAQ

### Does cluster-mining support native MassDB files?

No. Cluster-mining supports only chia plot files for now. The supporting of native MassDB files is in WIP.

### Does Fractal Miner provide HTTP API?

No. Fractal Miner does not provide HTTP API. To collect the `binding_list.json` file from each machine, please refer to [mass-binding-target](https://github.com/massnetorg/mass-binding-target).
