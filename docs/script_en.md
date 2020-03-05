# P2WSH
P2WSH(pay to witness script hash) is almost the same with Bitcoin

## Sending to a P2WSH Output
Sending a transaction to a P2WSH address requires the construction of the output script:

| Transaction Element | Script |
| ---- | ---- |
| Output Script | 0 [32-byte sha256(witness script)] |

The `witness script` is a multisig script.

## Spending a P2WSH Output
Spending a P2WSH output requires constructing the transaction according to the following scheme.

| Transaction Element | Script |
| ---- | ---- |
| Output Script | According to destination address |
| Witness | [signatures] [witness script] |

# P2SWSH
P2SWSH(pay to staking witness script hash)

## Sending to a P2SWSH Output

Sending a transaction to a P2SWSH address requires the construction of the output script:

| Transaction Element | Script |
| ---- | ---- |
| Output Script | 0 [32-byte sha256(witness script)] [frozen_period] |

`frozen_period` declares how many blocks this transaction output would be confirmed before becoming spendable. 

## Spending a P2SWSH Output

Same with P2WSH

# P2BWSH
P2BWSH(pay to binding witness script hash)

## Sending to a P2BWSH Output

Sending a transaction to a P2BWSH address requires the construction of the output script:

| Transaction Element | Script |
| ---- | ---- |
| Output Script | 0 [32-byte sha256(witness script)] [20-byte hash160(miner pk)] |


## Spending a P2BWSH Output

Same with P2WSH
