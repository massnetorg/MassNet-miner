# MASS Miner API

MASS Miner exposes a number of APIs over **gRPC** and **HTTP** for developers to interact with.

[gRPC](https://grpc.io/) is a high-performance open-source RPC framework could be easily implemented across different languages and platforms.

HTTP API provides another clear and universal way to interact by wrapping gRPC. HTTP API's supported MIME content type is `application/json`.

## Usage

### Endpoints

By default, both gPRC and HTTP API will be served at the following ports when MASS Miner starts.

| Supported Protocols | Default URL and port     |
|---------------------|--------------------------|
| gRPC                | `http://localhost:9685` |
| HTTP                | `http://localhost:9686` |

### Configuration

| Item            | Default   | Description                                           |
|-----------------|-----------|-------------------------------------------------------|
| api_port_grpc   | `9685`    | gRPC port                                             |
| api_port_http   | `9686`    | HTTP port                                             |
| api_whitelist   | `false`   | whitelist(IP), `*` means allow all                    |
| api_allowed_lan | `(empty)` | whitelist for IPs with LAN prefix(`10`, `172`, `192`) |

### API Documentation

MASS Miner provides a configuration file for [Swagger](https://swagger.io/) which provides a user-friendly HTTP API documentation accessible from web browser.

1. Register an account at [Swagger Hub](https://app.swaggerhub.com).
2. Login and `Import API` from `./api/proto/api.swagger.json`.
3. `Preview Docs`.

Alternatively, check `./api/proto/api.swagger.json` directly for full definition of all RPC APIs.

### Errors

#### Errors in Response Code

| Response Code | Error message                                         |
|---------------|-------------------------------------------------------|
| `400`         | Invalid request.                                      |
| `403`         | User does not have permission to access the resource. |
| `404`         | Resource does not exist.                              |
| `503`         | Service unavailable.                                  |

## API Methods

### client related APIs
- client
  * [GetClientStatus](#getclientstatus)
### blockchain related APIs
- blocks
  * [GetBestBlock](#getbestblock)
  * [GetBlock](#getblock)
  * [GetBlockHashByHeight](#getblockhashbyheight)
  * [GetBlockByHeight](#getblockbyheight)
  * [GetBlockHeader](#getblockheader)
- transactions
  * [GetTxPool](#gettxpool)
### mining related APIs
- spaces
  * [ConfigureCapacity](#configurecapacity)
  * [GetCapacitySpaces](#getcapacityspaces)
  * [ConfigureCapacityByDirs](#configurecapacitybydirs)
  * [GetCapacitySpacesByDirs](#getcapacityspacesbydirs)
  * [GetCapacitySpace](#getcapacityspace)
  * [PlotCapacitySpaces](#plotcapacityspaces)
  * [PlotCapacitySpace](#plotcapacityspace)
  * [MineCapacitySpaces](#minecapacityspaces)
  * [MineCapacitySpace](#minecapacityspace)
  * [StopCapacitySpaces](#stopcapacityspaces)
  * [StopCapacitySpace](#stopcapacityspace)
- wallets
  * [GetKeystore](#getkeystore)
  * [ExportKeystore](#exportkeystore)
  * [ImportKeystore](#importkeystore)
  * [UnlockWallet](#unlockwallet)
  * [LockWallet](#lockwallet)
  * [ChangePrivatePass](#changeprivatepass)
  * [ChangePublicPass](#changepublicpass)

---

#### GetClientStatus
    GET /v1/client/status
It is to get the status of current MASS Miner Client.
##### Parameters
null
##### Returns
- `String` - `version`, version of current MASS Miner Client
- `Boolean` - `peer_listening`, is listening peer or not
- `Boolean` - `syncing`, is syncing blocks with peers or not
- `Boolean` - `mining`, is mining new blocks or not
- `Boolean` - `space_keeping`, is SpaceKeeper running or not
- `String` - `chain_id`, chain_id of the blockchain
- `Integer` - `local_best_height`
- `Integer` - `known_best_height`
- `String` - `p2p_id`
- `Object` - `peer_count`, count of peers
  - `Integer` - `total`
  - `Integer` - `outbound`
  - `Integer` - `inbound`
- `Object` - `peers`, peer infos
  - `Array of Object` - `outbound`, infos of outbound peers 
    - `String` - `id`, p2p id
    - `String` - `address`, ip address
    - `String` - `direction`, outbound
  - `Array of Object` - `inbound`, infos of inbound peers 
    - `String` - `id`, p2p id
    - `String` - `address`, ip address
    - `String` - `direction`, inbound
  - `Array of Object` - `other`, infos of other(maybe dialing) peers 
    - `String` - `id`, p2p id
    - `String` - `address`, ip address
    - `String` - `direction`
##### Example
```json
{
    "version": "1.0.0+61248729",
    "peer_listening": true,
    "syncing": false,
    "mining": false,
    "space_keeping": false,
    "chain_id": "5433524b370b149007ba1d06225b5d8e53137a041869834cff5860b02bebc5c7",
    "local_best_height": "192000",
    "known_best_height": "192000",
    "p2p_id": "2E8B3F2C926F491E85B048705B3BBB33F7429F9E4CFC52AAD37598281E33D58C",
    "peer_count": {
        "total": 5,
        "outbound": 5,
        "inbound": 0
    },
    "peers": {
        "outbound": [
            {
                "id": "4C321F5FD1E274A68FC13DA078B0C04B9059E6843505A7D33FE1B42F56715D65",
                "address": "106.15.233.21:43453",
                "direction": "outbound"
            },
            {
                "id": "61258089AC8AB407D094D9E1E45A168383A611F6A739A7361E8B55BF46618C94",
                "address": "47.108.89.132:43453",
                "direction": "outbound"
            },
            {
                "id": "012C69A46122A1292D4BE87E120E6C6F1EB7E77F9E3ECEA5540E2D5B14AC776D",
                "address": "39.108.215.150:43453",
                "direction": "outbound"
            },
            {
                "id": "42651DC7A0BB9D524B6D8D0E46167891395F44E7705D2EAFBDC60B541272D25E",
                "address": "47.108.88.140:43453",
                "direction": "outbound"
            },
            {
                "id": "FC62D88E9CD3BC967486C4209ED1FC59C2A34C95913ADFCA75BD7A592A7B5001",
                "address": "39.100.57.28:43453",
                "direction": "outbound"
            }
        ],
        "inbound": [],
        "other": []
    }
}
```

---

#### GetBestBlock
    GET /v1/blocks/best
It is to get the best block of current node.
##### Parameters
null
##### Returns
- `String` - `hash`
- `Integer ` - `height`
##### Example
```json
{
    "height": "192000",
    "hash": "b3d895147da4c896c72ef4d28d8750765da9bd5c2672c6d20dd4676300b2c432"
}
```

---

#### GetBlock
    GET /v1/blocks/{hash}
It is to get block data by block hash.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| hash | string | required | block hash | |
##### Returns
- `String` - `hash`
- `String` - `chain_id`
- `Integer` - `version`
- `Integer` - `height`
- `Integer` - `confirmations`
- `Integer` - `time`
- `String` - `previous_hash`
- `String` - `next_hash`
- `String` - `transaction_root`
- `String` - `witness_root`
- `String` - `proposal_root`
- `String` - `target`
- `String` - `quality`
- `String` - `challenge`
- `String` - `public_key`
- `Object` - `proof`
    - `String` - `x`
    - `String` - `x_prime`
    - `Integer` - `bit_length`
- `Object` - `block_signature`
    - `String` - `r`
    - `String` - `s`
- `Array of String` - `ban_list`
- `Object` - `proposal_area`
    - `Array of Object` - `punishment_area`
        - `Integer` - `version`
        - `Integer` - `proposal_type`
        - `String` - `public_key`
        - `Array of Object` - `testimony`
            - `String` - `hash`
            - `String` - `chain_id`
            - `Integer` - `version`
            - `Integer` - `height`
            - `Integer` - `time`
            - `String` - `previous_hash`
            - `String` - `transaction_root`
            - `String` - `witness_root`
            - `String` - `proposal_root`
            - `String` - `target`
            - `String` - `challenge`
            - `String` - `public_key`
            - `Object` - `proof`
                - `String` - `x`
                - `String` - `x_prime`
                - `Integer` - `bit_length`
            - `Object` - `block_signature`
                - `String` - `r`
                - `String` - `s`
            - `Array of String` - `ban_list`
    - `Array of Object` - `other_area`
        - `Integer` - `version`
        - `Integer` - `proposal_type`
        - `String` - `data`
- `Array of String` - `tx`
- `Array of object` - `raw_tx`
    - `String` - `txid`
    - `Integer` - `version`
    - `Integer` - `lock_time`
    - `Object` - `block`
        - `Integer` - `height`
        - `String` - `block_hash`
        - `Integer` - `timestamp`
    - `Array of Object` - `vin`
        - `String` - `txid`
        - `Integer` - `vout`
        - `Integer` - `sequence`
        - `Array of String` - `witness`
    - `Array of Object` - `vout`
        - `String` - `value`
        - `Integer` - `n`
        - `Object` - `script_public_key`
            - `String` - `asm`
            - `String` - `hex`
            - `Integer` - `req_sigs`
            - `String` - `type`
            - `Integer` - `frozen_period`
            - `String` - `reward_address`
            - `Array of String` - `addresses`
    - `Array of String` - `from_address`
    - `Array of Object` - `to`
        - `Array of String` - `address`
        - `String` - `value`
    - `Array of Object` - `inputs`
        - `String` - `txid`
        - `Integer` - `index`
        - `Array of String` - `address`
        - `String` - `value`
    - `String` - `payload`
    - `Integer` - `confirmations`
    - `Integer` - `size`
    - `String` - `fee`
    - `Integer` - `status`
    - `Integer` - `type`
- `Integer` - `size`
- `String` - `time_utc`
- `Integer` - `tx_count`
##### Example
```bash
$ curl localhost:9686/v1/blocks/b3d895147da4c896c72ef4d28d8750765da9bd5c2672c6d20dd4676300b2c432
```
```json
{
    "hash": "b3d895147da4c896c72ef4d28d8750765da9bd5c2672c6d20dd4676300b2c432",
    "chain_id": "5433524b370b149007ba1d06225b5d8e53137a041869834cff5860b02bebc5c7",
    "version": "1",
    "height": "192000",
    "confirmations": "4204",
    "time": "1575078576",
    "previous_hash": "9a1b91850598b397c36be4cb34a99c183812ce05a8c3c6afdf244f654d738422",
    "next_hash": "772220d2dfca2d319f1699ded69a634458ca987c343f4ed8a73c1d05f74c6357",
    "transaction_root": "965f0c473adca41416436906a4af88708fe454382deef128139b006576882bd9",
    "witness_root": "965f0c473adca41416436906a4af88708fe454382deef128139b006576882bd9",
    "proposal_root": "9663440551fdcd6ada50b1fa1b0003d19bc7944955820b54ab569eb9a7ab7999",
    "target": "900151a5dfab34",
    "quality": "9ba22b427ac198",
    "challenge": "2b71e099dc30234bf86aefe06ba92cf91c6cc1c1557debd453017c5159fcaf08",
    "public_key": "023ae4379095eed46d30503384a8624ae7dc31cc361865b1ef6154e1fb6ceb284b",
    "proof": {
        "x": "79c8525f",
        "x_prime": "425d779f",
        "bit_length": 32
    },
    "block_signature": {
        "r": "f1dec9ce034fde28906e07bc3539dad633c0347f69c6f19b85685db2ee31202c",
        "s": "68590736858f9df3318c112e3a59a896a9abd6a3f49387776bf826b2af1b62d1"
    },
    "ban_list": [],
    "proposal_area": {
        "punishment_area": [],
        "other_area": []
    },
    "tx": [],
    "raw_tx": [
        {
            "txid": "965f0c473adca41416436906a4af88708fe454382deef128139b006576882bd9",
            "version": 1,
            "lock_time": "0",
            "block": {
                "height": "192000",
                "block_hash": "b3d895147da4c896c72ef4d28d8750765da9bd5c2672c6d20dd4676300b2c432",
                "timestamp": "1575078576"
            },
            "vin": [
                {
                    "txid": "0000000000000000000000000000000000000000000000000000000000000000",
                    "vout": 0,
                    "sequence": "18446744073709551615",
                    "witness": []
                }
            ],
            "vout": [
                {
                    "value": "104",
                    "n": 0,
                    "script_public_key": {
                        "asm": "0 6dae90c56ddac07b5df818e035793c7114fd9545a986d442cc83466ef0a2b5fc",
                        "hex": "00206dae90c56ddac07b5df818e035793c7114fd9545a986d442cc83466ef0a2b5fc",
                        "req_sigs": 1,
                        "type": "witness_v0_scripthash",
                        "frozen_period": 0,
                        "reward_address": "",
                        "addresses": [
                            "ms1qqdkhfp3tdmtq8kh0crrsr27fuwy20m9294xrdgskvsdrxau9zkh7qs7v3yd"
                        ]
                    }
                },
                {
                    "value": "24",
                    "n": 1,
                    "script_public_key": {
                        "asm": "0 54fb35b5bd9ad0eecae51a679b0d364343cfa10f7d2d9e83b98c0e82f3a7c64c",
                        "hex": "002054fb35b5bd9ad0eecae51a679b0d364343cfa10f7d2d9e83b98c0e82f3a7c64c",
                        "req_sigs": 1,
                        "type": "witness_v0_scripthash",
                        "frozen_period": 0,
                        "reward_address": "",
                        "addresses": [
                            "ms1qq2nantddantgwajh9rfnekrfkgdpulgg005keaqae3s8g9ua8cexqgqeh4z"
                        ]
                    }
                }
            ],
            "from_address": [],
            "to": [
                {
                    "address": [
                        "ms1qqdkhfp3tdmtq8kh0crrsr27fuwy20m9294xrdgskvsdrxau9zkh7qs7v3yd"
                    ],
                    "value": "104"
                },
                {
                    "address": [
                        "ms1qq2nantddantgwajh9rfnekrfkgdpulgg005keaqae3s8g9ua8cexqgqeh4z"
                    ],
                    "value": "24"
                }
            ],
            "inputs": [],
            "payload": "00ee02000000000001000000",
            "confirmations": "4204",
            "size": 152,
            "fee": "0",
            "status": 4,
            "type": 4
        }
    ],
    "size": 1491,
    "time_utc": "2019-11-30T01:49:36Z",
    "tx_count": 1
}
```

---

#### GetBlockHashByHeight
    GET /v1/blocks/hash/{height}
It is to get block hash by block height.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| height | uint64 | required | block height | |
##### Returns
- `Integer ` - `height`
##### Example
```bash
$ curl localhost:9686/v1/blocks/hash/192000
```
```json
{
    "hash": "b3d895147da4c896c72ef4d28d8750765da9bd5c2672c6d20dd4676300b2c432"
}
```

---

#### GetBlockByHeight
    GET /v1/blocks/height/{height}
It is to get block data by block height.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| height | uint64 | required | block height | |
##### Returns
same as [GetBlock](#GetBlock).
- `String` - `hash`
- `String` - `chain_id`
- `Integer` - `version`
- `Integer` - `height`
- `Integer` - `confirmations`
- `Integer` - `time`
- `String` - `previous_hash`
- `String` - `next_hash`
- `String` - `transaction_root`
- `String` - `witness_root`
- `String` - `proposal_root`
- `String` - `target`
- `String` - `quality`
- `String` - `challenge`
- `String` - `public_key`
- `Object` - `proof`
    - `String` - `x`
    - `String` - `x_prime`
    - `Integer` - `bit_length`
- `Object` - `block_signature`
    - `String` - `r`
    - `String` - `s`
- `Array of String` - `ban_list`
- `Object` - `proposal_area`
    - `Array of Object` - `punishment_area`
        - `Integer` - `version`
        - `Integer` - `proposal_type`
        - `String` - `public_key`
        - `Array of Object` - `testimony`
            - `String` - `hash`
            - `String` - `chain_id`
            - `Integer` - `version`
            - `Integer` - `height`
            - `Integer` - `time`
            - `String` - `previous_hash`
            - `String` - `transaction_root`
            - `String` - `witness_root`
            - `String` - `proposal_root`
            - `String` - `target`
            - `String` - `challenge`
            - `String` - `public_key`
            - `Object` - `proof`
                - `String` - `x`
                - `String` - `x_prime`
                - `Integer` - `bit_length`
            - `Object` - `block_signature`
                - `String` - `r`
                - `String` - `s`
            - `Array of String` - `ban_list`
    - `Array of Object` - `other_area`
        - `Integer` - `version`
        - `Integer` - `proposal_type`
        - `String` - `data`
- `Array of String` - `tx`
- `Array of object` - `raw_tx`
    - `String` - `txid`
    - `Integer` - `version`
    - `Integer` - `lock_time`
    - `Object` - `block`
        - `Integer` - `height`
        - `String` - `block_hash`
        - `Integer` - `timestamp`
    - `Array of Object` - `vin`
        - `String` - `txid`
        - `Integer` - `vout`
        - `Integer` - `sequence`
        - `Array of String` - `witness`
    - `Array of Object` - `vout`
        - `String` - `value`
        - `Integer` - `n`
        - `Object` - `script_public_key`
            - `String` - `asm`
            - `String` - `hex`
            - `Integer` - `req_sigs`
            - `String` - `type`
            - `Integer` - `frozen_period`
            - `String` - `reward_address`
            - `Array of String` - `addresses`
    - `Array of String` - `from_address`
    - `Array of Object` - `to`
        - `Array of String` - `address`
        - `String` - `value`
    - `Array of Object` - `inputs`
        - `String` - `txid`
        - `Integer` - `index`
        - `Array of String` - `address`
        - `String` - `value`
    - `String` - `payload`
    - `Integer` - `confirmations`
    - `Integer` - `size`
    - `String` - `fee`
    - `Integer` - `status`
    - `Integer` - `type`
- `Integer` - `size`
- `String` - `time_utc`
- `Integer` - `tx_count`
##### Example
```bash
$ curl localhost:9686/v1/blocks/height/192000
```
```json
{
    "hash": "b3d895147da4c896c72ef4d28d8750765da9bd5c2672c6d20dd4676300b2c432",
    "chain_id": "5433524b370b149007ba1d06225b5d8e53137a041869834cff5860b02bebc5c7",
    "version": "1",
    "height": "192000",
    "confirmations": "4276",
    "time": "1575078576",
    "previous_hash": "9a1b91850598b397c36be4cb34a99c183812ce05a8c3c6afdf244f654d738422",
    "next_hash": "772220d2dfca2d319f1699ded69a634458ca987c343f4ed8a73c1d05f74c6357",
    "transaction_root": "965f0c473adca41416436906a4af88708fe454382deef128139b006576882bd9",
    "witness_root": "965f0c473adca41416436906a4af88708fe454382deef128139b006576882bd9",
    "proposal_root": "9663440551fdcd6ada50b1fa1b0003d19bc7944955820b54ab569eb9a7ab7999",
    "target": "900151a5dfab34",
    "quality": "9ba22b427ac198",
    "challenge": "2b71e099dc30234bf86aefe06ba92cf91c6cc1c1557debd453017c5159fcaf08",
    "public_key": "023ae4379095eed46d30503384a8624ae7dc31cc361865b1ef6154e1fb6ceb284b",
    "proof": {
        "x": "79c8525f",
        "x_prime": "425d779f",
        "bit_length": 32
    },
    "block_signature": {
        "r": "f1dec9ce034fde28906e07bc3539dad633c0347f69c6f19b85685db2ee31202c",
        "s": "68590736858f9df3318c112e3a59a896a9abd6a3f49387776bf826b2af1b62d1"
    },
    "ban_list": [],
    "proposal_area": {
        "punishment_area": [],
        "other_area": []
    },
    "tx": [],
    "raw_tx": [
        {
            "txid": "965f0c473adca41416436906a4af88708fe454382deef128139b006576882bd9",
            "version": 1,
            "lock_time": "0",
            "block": {
                "height": "192000",
                "block_hash": "b3d895147da4c896c72ef4d28d8750765da9bd5c2672c6d20dd4676300b2c432",
                "timestamp": "1575078576"
            },
            "vin": [
                {
                    "txid": "0000000000000000000000000000000000000000000000000000000000000000",
                    "vout": 0,
                    "sequence": "18446744073709551615",
                    "witness": []
                }
            ],
            "vout": [
                {
                    "value": "104",
                    "n": 0,
                    "script_public_key": {
                        "asm": "0 6dae90c56ddac07b5df818e035793c7114fd9545a986d442cc83466ef0a2b5fc",
                        "hex": "00206dae90c56ddac07b5df818e035793c7114fd9545a986d442cc83466ef0a2b5fc",
                        "req_sigs": 1,
                        "type": "witness_v0_scripthash",
                        "frozen_period": 0,
                        "reward_address": "",
                        "addresses": [
                            "ms1qqdkhfp3tdmtq8kh0crrsr27fuwy20m9294xrdgskvsdrxau9zkh7qs7v3yd"
                        ]
                    }
                },
                {
                    "value": "24",
                    "n": 1,
                    "script_public_key": {
                        "asm": "0 54fb35b5bd9ad0eecae51a679b0d364343cfa10f7d2d9e83b98c0e82f3a7c64c",
                        "hex": "002054fb35b5bd9ad0eecae51a679b0d364343cfa10f7d2d9e83b98c0e82f3a7c64c",
                        "req_sigs": 1,
                        "type": "witness_v0_scripthash",
                        "frozen_period": 0,
                        "reward_address": "",
                        "addresses": [
                            "ms1qq2nantddantgwajh9rfnekrfkgdpulgg005keaqae3s8g9ua8cexqgqeh4z"
                        ]
                    }
                }
            ],
            "from_address": [],
            "to": [
                {
                    "address": [
                        "ms1qqdkhfp3tdmtq8kh0crrsr27fuwy20m9294xrdgskvsdrxau9zkh7qs7v3yd"
                    ],
                    "value": "104"
                },
                {
                    "address": [
                        "ms1qq2nantddantgwajh9rfnekrfkgdpulgg005keaqae3s8g9ua8cexqgqeh4z"
                    ],
                    "value": "24"
                }
            ],
            "inputs": [],
            "payload": "00ee02000000000001000000",
            "confirmations": "4276",
            "size": 152,
            "fee": "0",
            "status": 4,
            "type": 4
        }
    ],
    "size": 1491,
    "time_utc": "2019-11-30T01:49:36Z",
    "tx_count": 1
}
```

---

#### GetBlockHeader
    GET /v1/blocks/{hash}/header
It is to get block header by block hash.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| hash | string | required | block hash | |
##### Returns
- `String` - `hash`
- `String` - `chain_id`
- `Integer` - `version`
- `Integer` - `height`
- `Integer` - `confirmations`
- `Integer` - `time`
- `String` - `previous_hash`
- `String` - `next_hash`
- `String` - `transaction_root`
- `String` - `witness_root`
- `String` - `proposal_root`
- `String` - `target`, hex string
- `String` - `quality`, decimal string
- `String` - `challenge`
- `String` - `public_key`
- `Object` - `proof`
    - `String` - `x`
    - `String` - `x_prime`
    - `Integer` - `bit_length`
- `Object` - `block_signature`
    - `String` - `r`
    - `String` - `s`
- `Array of String` - `ban_list`
- `String` - `time_utc`
##### Example
```bash
$ curl localhost:9686/v1/blocks/b3d895147da4c896c72ef4d28d8750765da9bd5c2672c6d20dd4676300b2c432/header
```
```json
{
    "hash": "b3d895147da4c896c72ef4d28d8750765da9bd5c2672c6d20dd4676300b2c432",
    "chain_id": "5433524b370b149007ba1d06225b5d8e53137a041869834cff5860b02bebc5c7",
    "version": "1",
    "height": "192000",
    "confirmations": "4276",
    "timestamp": "1575078576",
    "previous_hash": "9a1b91850598b397c36be4cb34a99c183812ce05a8c3c6afdf244f654d738422",
    "next_hash": "772220d2dfca2d319f1699ded69a634458ca987c343f4ed8a73c1d05f74c6357",
    "transaction_root": "965f0c473adca41416436906a4af88708fe454382deef128139b006576882bd9",
    "witness_root": "",
    "proposal_root": "9663440551fdcd6ada50b1fa1b0003d19bc7944955820b54ab569eb9a7ab7999",
    "target": "900151a5dfab34",
    "quality": "43806928072786328",
    "challenge": "2b71e099dc30234bf86aefe06ba92cf91c6cc1c1557debd453017c5159fcaf08",
    "public_key": "023ae4379095eed46d30503384a8624ae7dc31cc361865b1ef6154e1fb6ceb284b",
    "proof": {
        "x": "79c8525f",
        "x_prime": "425d779f",
        "bit_length": 32
    },
    "block_signature": {
        "r": "f1dec9ce034fde28906e07bc3539dad633c0347f69c6f19b85685db2ee31202c",
        "s": "68590736858f9df3318c112e3a59a896a9abd6a3f49387776bf826b2af1b62d1"
    },
    "ban_list": [],
    "time_utc": "2019-11-30T01:49:36Z"
}
```

#### GetTxPool
    GET /v1/transactions/pool
It is to get a brief summary of transaction pool.
##### Parameters
null
##### Returns
- `Integer` - `tx_count`
- `Integer` - `orphan_count`
- `Integer` - `tx_plain_size`
- `Integer` - `tx_packet_size`
- `Integer` - `orphan_plain_size`
- `Integer` - `orphan_packet_size`
- `Array of String` - `txs`, an array of txids
- `Array of String` - `orphans`, an array of orphan txids
##### Example
```json
{
    "tx_count": 0,
    "orphan_count": 0,
    "tx_plain_size": "0",
    "tx_packet_size": "0",
    "orphan_plain_size": "0",
    "orphan_packet_size": "0",
    "txs": [],
    "orphans": []
}
```

---

#### ConfigureCapacity
    POST /v1/spaces
It is to configure miner by required DiskSize.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| capacity | int | required | overall mining disk size | represented by MiB |
| payout_addresses | []string | required | array of payout addresses | |
| passphrase | string | required | passphrase to unlock wallet | same as what -P argument set |
##### Returns
- `Integer` - `space_count` 
- `Array of Object`, `spaces`
    - `Integer` - `ordinal`
    - `String` - `public_key`
    - `String` - `address`
    - `Integer` - `bit_length`
    - `String` - `state`
    - `Float` - `progress`
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
**request**
```json
{
	"capacity": 400,
	"payout_addresses": ["ms1qq7xrd32awhvaj9lkgn6tzm2n76gcu5zglxguzyu3kxrs9pz7tk32qvw70ky"],
	"passphrase": "123456"
}
```
**response**
```json
{
    "space_count": 4,
    "spaces": [
        {
            "ordinal": "0",
            "public_key": "02905d92f83d1519fa4f9b9e8bf2b91361438eeb7d74223c3f0371460e62cfa441",
            "address": "165xgckgUQym6wvmtn8adXnpsQcZhPJ2pV",
            "bit_length": 24,
            "state": "registered",
            "progress": 0
        },
        {
            "ordinal": "1",
            "public_key": "0325f91597e68f0427300ad35b40dc91373b9c3264a745f25908cb5ceb12a96be4",
            "address": "1EjyGZLJnrHDkDT6A54XzsQNRoXsMLVSiM",
            "bit_length": 24,
            "state": "registered",
            "progress": 0
        },
        {
            "ordinal": "2",
            "public_key": "03ae8e4eb760487b774e3a6125180ccc924c96b7feb54abb17dcc7d9c5db0ee58e",
            "address": "1CB2n7iMYUKyy6e49PykNtXXhyP4aFppvK",
            "bit_length": 24,
            "state": "registered",
            "progress": 0
        },
        {
            "ordinal": "3",
            "public_key": "02854885799adb8953c588a0f170149e6b48d979c4d74486dcc446d91835a42c40",
            "address": "1CpD8DW8XX59tJwQr9Cyem7nDn7rTbfoSy",
            "bit_length": 24,
            "state": "registered",
            "progress": 0
        }
    ],
    "error_code": 0,
    "error_message": ""
}
```

---

#### GetCapacitySpaces
    GET /v1/spaces
It is to get all configured miner spaces.
##### Parameters
null
##### Returns
- `Integer` - `space_count` 
- `Array of Object`, `spaces`
    - `Integer` - `ordinal`
    - `String` - `public_key`
    - `String` - `address`
    - `Integer` - `bit_length`
    - `String` - `state`
    - `Float` - `progress`
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
```json
{
    "space_count": 4,
    "spaces": [
        {
            "ordinal": "0",
            "public_key": "02905d92f83d1519fa4f9b9e8bf2b91361438eeb7d74223c3f0371460e62cfa441",
            "address": "165xgckgUQym6wvmtn8adXnpsQcZhPJ2pV",
            "bit_length": 24,
            "state": "registered",
            "progress": 0
        },
        {
            "ordinal": "1",
            "public_key": "0325f91597e68f0427300ad35b40dc91373b9c3264a745f25908cb5ceb12a96be4",
            "address": "1EjyGZLJnrHDkDT6A54XzsQNRoXsMLVSiM",
            "bit_length": 24,
            "state": "registered",
            "progress": 0
        },
        {
            "ordinal": "2",
            "public_key": "03ae8e4eb760487b774e3a6125180ccc924c96b7feb54abb17dcc7d9c5db0ee58e",
            "address": "1CB2n7iMYUKyy6e49PykNtXXhyP4aFppvK",
            "bit_length": 24,
            "state": "registered",
            "progress": 0
        },
        {
            "ordinal": "3",
            "public_key": "02854885799adb8953c588a0f170149e6b48d979c4d74486dcc446d91835a42c40",
            "address": "1CpD8DW8XX59tJwQr9Cyem7nDn7rTbfoSy",
            "bit_length": 24,
            "state": "registered",
            "progress": 0
        }
    ],
    "error_code": 0,
    "error_message": ""
}
```

---

#### ConfigureCapacityByDirs
    POST /v1/spaces/directory
It is to configure miner by required directories and corresponding disk sizes.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| allocations | []struct | required | consists of directory and capacity | |
| directory | string | required | directory to save MassDBs | path should be existed |
| capacity | int | required | mining disk size for corresponding directory | represented by MiB |
| payout_addresses | []string | required | array of payout addresses | |
| passphrase | string | required | passphrase to unlock wallet | same as what -P argument set |
- `Array of Object`, `allocations`
    - `String` - `directory`
    - `Int` - `capacity`
- `Array of String`, `payout_addresses`
- `String` - `passphrase`
##### Returns
- `Integer` - `directory_count`
- `Array of Object`, `allocations`
    - `String` - `directory`
    - `String` - `capacity`
    - `Integer` - `space_count` 
    - `Array of Object`, `spaces`
        - `Integer` - `ordinal`
        - `String` - `public_key`
        - `String` - `address`
        - `Integer` - `bit_length`
        - `String` - `state`
        - `Float` - `progress`
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
**request**
```json
{
    "allocations": [
        {
            "directory": "spaces1",
            "capacity": 100
        },
        {
            "directory": "spaces2",
            "capacity": 200
        }
    ],
    "payout_addresses": [
        "ms1qqamtl9nd8x38plenkgdzecpvsfuaqw3szhglhffcxler0jmsd840sueckv0"
    ],
    "passphrase": "123456"
}
```
**response**
```json
{
    "directory_count": 2,
    "allocations": [
        {
            "directory": "/root/spaces1",
            "capacity": "96",
            "space_count": 1,
            "spaces": [
                {
                    "ordinal": "0",
                    "public_key": "032d5465c60bf7e43b96b39f77fa433a2c083e055860e50d332b279f0e78fb336d",
                    "address": "181zD466dKPEDFebeVXG7uJwKsjnbXTCtv",
                    "bit_length": 24,
                    "state": "mining",
                    "progress": 100
                }
            ]
        },
        {
            "directory": "/root/spaces2",
            "capacity": "192",
            "space_count": 2,
            "spaces": [
                {
                    "ordinal": "1",
                    "public_key": "03d18ce607b80af663da3e47ce7b7b53a068d56dfc95603e57ee87616a9894c2b9",
                    "address": "1KZg1RfPL9GkQoN6yFcC2R6N8HfQrUGAcv",
                    "bit_length": 24,
                    "state": "mining",
                    "progress": 100
                },
                {
                    "ordinal": "2",
                    "public_key": "02eb0aefdc273a5083b3bf03b21bdd1159479e6eedc4d5c7a41595229d43f6d45f",
                    "address": "1Mvn6yva8q9Le5eKCU7pR3G1ryYqY7oCVP",
                    "bit_length": 24,
                    "state": "mining",
                    "progress": 100
                }
            ]
        }
    ],
    "error_code": 0,
    "error_message": ""
}
```

---

#### GetCapacitySpacesByDirs
    GET /v1/spaces/directory
It is to get all configured miner spaces.
##### Parameters
null
##### Returns
- `Integer` - `directory_count`
- `Array of Object`, `allocations`
    - `String` - `directory`
    - `String` - `capacity`
    - `Integer` - `space_count` 
    - `Array of Object`, `spaces`
        - `Integer` - `ordinal`
        - `String` - `public_key`
        - `String` - `address`
        - `Integer` - `bit_length`
        - `String` - `state`
        - `Float` - `progress`
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
```json
{
    "directory_count": 2,
    "allocations": [
        {
            "directory": "/root/spaces1",
            "capacity": "96",
            "space_count": 1,
            "spaces": [
                {
                    "ordinal": "0",
                    "public_key": "032d5465c60bf7e43b96b39f77fa433a2c083e055860e50d332b279f0e78fb336d",
                    "address": "181zD466dKPEDFebeVXG7uJwKsjnbXTCtv",
                    "bit_length": 24,
                    "state": "mining",
                    "progress": 100
                }
            ]
        },
        {
            "directory": "/root/spaces2",
            "capacity": "192",
            "space_count": 2,
            "spaces": [
                {
                    "ordinal": "1",
                    "public_key": "03d18ce607b80af663da3e47ce7b7b53a068d56dfc95603e57ee87616a9894c2b9",
                    "address": "1KZg1RfPL9GkQoN6yFcC2R6N8HfQrUGAcv",
                    "bit_length": 24,
                    "state": "mining",
                    "progress": 100
                },
                {
                    "ordinal": "2",
                    "public_key": "02eb0aefdc273a5083b3bf03b21bdd1159479e6eedc4d5c7a41595229d43f6d45f",
                    "address": "1Mvn6yva8q9Le5eKCU7pR3G1ryYqY7oCVP",
                    "bit_length": 24,
                    "state": "mining",
                    "progress": 100
                }
            ]
        }
    ],
    "error_code": 0,
    "error_message": ""
}
```

---

#### GetCapacitySpace
    GET /v1/spaces/{space_id}
It is to get configured miner space by space_id.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| space_id | string | required | Space ID, formatted as ("%s-%d", public_key, bit_length) | |
##### Returns
- `Object`, `space`
    - `Integer` - `ordinal`
    - `String` - `public_key`
    - `String` - `address`
    - `Integer` - `bit_length`
    - `String` - `state`
    - `Float` - `progress`
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
```bash
$ curl localhost:9686/v1/spaces/02905d92f83d1519fa4f9b9e8bf2b91361438eeb7d74223c3f0371460e62cfa441-24
```
```json
{
    "space": {
        "ordinal": "0",
        "public_key": "02905d92f83d1519fa4f9b9e8bf2b91361438eeb7d74223c3f0371460e62cfa441",
        "address": "165xgckgUQym6wvmtn8adXnpsQcZhPJ2pV",
        "bit_length": 24,
        "state": "registered",
        "progress": 0
    },
    "error_code": 0,
    "error_message": ""
}
```

---

#### PlotCapacitySpaces
    POST /v1/spaces/plot
It is to plot all configured miner spaces.
##### Parameters
null
##### Returns
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
```json
{
    "error_code": 0,
    "error_message": ""
}
```

---

#### PlotCapacitySpace
    POST /v1/spaces/{space_id}/plot
It is to plot configured miner space by space_id.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| space_id | string | required | Space ID, formatted as ("%s-%d", public_key, bit_length) | |
##### Returns
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
```json
{
    "error_code": 0,
    "error_message": ""
}
```

---

#### MineCapacitySpaces
    POST /v1/spaces/mine
It is to mine all configured miner spaces.
##### Parameters
null
##### Returns
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
```json
{
    "error_code": 0,
    "error_message": ""
}
```

---

#### MineCapacitySpace
    POST /v1/spaces/{space_id}/mine
It is to mine configured miner space by space_id.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| space_id | string | required | Space ID, formatted as ("%s-%d", public_key, bit_length) | |
##### Returns
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
```json
{
    "error_code": 0,
    "error_message": ""
}
```

---

#### StopCapacitySpaces
    POST /v1/spaces/stop
It is to stop all configured miner spaces.
##### Parameters
null
##### Returns
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
```json
{
    "error_code": 0,
    "error_message": ""
}
```

---

#### StopCapacitySpace
    POST /v1/spaces/{space_id}/stop
It is to stop configured miner space by space_id.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| space_id | string | required | Space ID, formatted as ("%s-%d", public_key, bit_length) | |
##### Returns
- `Integer` - `error_code`
- `String` - `error_message`
##### Example
```json
{
    "error_code": 0,
    "error_message": ""
}
```

---

#### GetKeystore
    GET /v1/wallets
It is to get all keystore in the wallet.
##### Parameters
null
##### Returns
- `Array of object` - `wallets`
    - `String` - `wallet_id`
    - `String` - `remark`
##### Example
```json
{
    "wallets": [
        {
            "wallet_id": "ac108dg4my870c0t07u22y6kurkfwjvk2580lk98m0",
            "remark": ""
        }
    ]
}
```

---

#### ExportKeystore
    POST /v1/wallets/export
It is to export keystore.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| wallet_id | string | required | wallet_id to be exported | |
| passphrase | string | required | passphrase | |
| export_path | string | required | file path of exported keystore | |
##### Returns
- `String` - `keystore` 
##### Example
**request**
```json
{
	"wallet_id": "ac108dg4my870c0t07u22y6kurkfwjvk2580lk98m0",
	"passphrase": "123456",
	"export_path": "."
}
```
**response**
```json
{
    "keystore": "{\"remark\":\"\",\"crypto\":{\"cipher\":\"Stream cipher\",\"masterHDPrivKeyEnc\":\"2bc4592eb1e8b5e3aede665c310b3da4b8670e8315e717896b4ce35d26cef8c59a43e5b3de6dfde0f8aadbf77a2e2a87cb5948e622c7d4a198b60ef134f4d20675589753dcc1327a412f95ef111d08a2becb3d10dcbd31cc3eba6208b9cf98f3cffd3b38b1246c85230161e52de51587fa5c8ef066720e2e5bf7fd65c12eaff99494add5a334c8879924c50a6299ff22f51988eeb5c40b\",\"kdf\":\"scrypt\",\"pubParams\":\"8b6672e43801b807cc8e2b84e7451140f4a0ccefd95fd0517645a3fa46c1212941e809953ee35ad776bd5454011e55edb677d02b9226a002d288cde88b33a3f5000004000000000008000000000000000100000000000000\",\"privParams\":\"2db906258e0a0430d64dcd88cc9234cf56642dc0075ae2def430018b21962bf8fb5c380ce4a3d229f930776a1965a4a03448d46796afe9a5e1fe985f3d129df7000004000000000008000000000000000100000000000000\",\"cryptoKeyPubEnc\":\"bd59ebac79b1a20a8d666ccf734b86e928f2b48419db9409c0c17b9a9cdad39fc71eec5d03ef09536b224b60c322c9e8eb5f72452b379a502009cb9d6c1c4e0a4c67cbb0f7130582\",\"cryptoKeyPrivEnc\":\"b324172e95537af9406ce89a4ae439804bb90585027d18648f62bddd7d38600fdf41de5f6fcf74b1206ae173b4f3c2200c8bd385b472968bff931e2492e0cb5f27b651bac01e482c\"},\"hdPath\":{\"Purpose\":44,\"Coin\":297,\"Account\":0,\"ExternalChildNum\":4,\"InternalChildNum\":0}}"
}
```

---

#### ImportKeystore
    POST /v1/wallets/import
It is to import keystore.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| import_path | string | required | filepath of target keystore | |
| old_passphrase | string | required | old passphrase | |
| new_passphrase | string | required | new passphrase | |
##### Returns
- `Boolean` - `status` 
- `String` - `wallet_id`
- `String` - `remark`  
##### Example
**request**
```json
{
	"import_path": "./keystore-ac108dg4my870c0t07u22y6kurkfwjvk2580lk98m0.json",
	"old_passphrase": "654321",
	"new_passphrase": "123456"
}
```
**response**
```json
{
    "status": true,
    "wallet_id": "ac108dg4my870c0t07u22y6kurkfwjvk2580lk98m0",
    "remark": ""
}
```

---

#### UnlockWallet
    POST /v1/wallets/unlocking
It is to unlock wallet.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| passphrase | string | required | passphrase | |
##### Returns
- `Boolean` - `success` 
- `String` - `error`  
##### Example
**request**
```json
{
	"passphrase": "123456"
}
```
**response**
```json
{
    "success": true,
    "error": ""
}
```

---

#### LockWallet
    POST /v1/wallets/locking
It is to lock wallet.
##### Parameters
null
##### Returns
- `Boolean` - `success` 
- `String` - `error`  
##### Example
```json
{
    "success": true,
    "error": ""
}
```

---

#### ChangePrivatePass
    POST /v1/wallets/privpass/changing
It is to change private pass of the wallet.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| old_privpass | string | required | old private password | |
| new_privpass | string | required | new private password | |
##### Returns
- `Boolean` - `success` 
##### Example
**request**
```json
{
	"old_privpass": "123456",
	"new_privpass": "654321"
}
```
**response**
```json
{
    "success": true
}
```

---

#### ChangePublicPass
    POST /v1/wallets/pubpass/changing
It is to change public pass of the wallet.
##### Parameters
| Parameter | Type | Attribute | Usage | Note |
| :----: | :----: | :----: | ------ | ------|
| old_pubpass | string | required | old pubpass | |
| new_pubpass | string | required | new pubpass | |
##### Returns
- `Boolean` - `success` 
##### Example
**request**
```json
{
	"old_pubpass": "pubPassword",
	"new_pubpass": "newPubPassword"
}
````
**response**
```json
{
    "success": true
}
```
