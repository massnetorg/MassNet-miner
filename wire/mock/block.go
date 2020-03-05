package mock

import (
	"fmt"

	"massnet.org/mass/massutil"
	"massnet.org/mass/wire"
)

func (c *Chain) constructBlock(blk *wire.MsgBlock, hgt uint64) (*wire.MsgBlock, error) {
	if blk.Header.Height != hgt {
		return nil, fmt.Errorf("invalid height %d:%d", blk.Header.Height, hgt)
	}
	blk.Transactions = make([]*wire.MsgTx, 0)
	// construct coinbase tx
	coinbase, coinbaseHash, err := c.createCoinbaseTx(blk.Header.Height, blk.Header.PubKey)
	if err != nil {
		return nil, err
	}
	blk.AddTransaction(coinbase.MsgTx())

	blkStat := &BlockTxStat{
		height: blk.Header.Height,
	}

	if blk.Header.Height >= threshold {
		// create non-coinbase txs
		txs, stat, totalFee, err := c.constructTxs1(blk.Header.Height)
		if err != nil {
			return nil, err
		}
		if totalFee < 0 {
			return nil, fmt.Errorf("negative total fee at block %d", blk.Header.Height)
		}
		minerTxOut := coinbase.MsgTx().TxOut[len(coinbase.MsgTx().TxOut)-1]
		minerTxOut.Value += totalFee

		for _, tx := range txs {
			blk.AddTransaction(tx)
		}
		blkStat.stat = stat
	}

	// re calculate utxo
	c.reCalcCoinbaseUtxo(blk, coinbaseHash)

	// fill block header
	err = c.fillBlockHeader(blk)
	if err != nil {
		return nil, err
	}
	c.stats = append(c.stats, blkStat)
	c.constructHeight = hgt
	return blk, nil
}

func (c *Chain) fillBlockHeader(blk *wire.MsgBlock) error {
	// fill previous
	blk.Header.Previous = c.blocks[blk.Header.Height-1].BlockHash()

	// fill transaction root
	merkles := wire.BuildMerkleTreeStoreTransactions(blk.Transactions, false)
	witnessMerkles := wire.BuildMerkleTreeStoreTransactions(blk.Transactions, true)
	blk.Header.TransactionRoot = *merkles[len(merkles)-1]
	blk.Header.WitnessRoot = *witnessMerkles[len(witnessMerkles)-1]

	// fill sig2
	key := pocKeys[pkStr(blk.Header.PubKey)]
	pocHash, err := blk.Header.PoCHash()
	if err != nil {
		return err
	}
	data := wire.HashH(pocHash[:])
	sig, err := key.Sign(data[:])
	if err != nil {
		return err
	}
	blk.Header.Signature = sig

	return nil
}

func copyBlock(blk *wire.MsgBlock) (*wire.MsgBlock, error) {
	buf, err := massutil.NewBlock(blk).Bytes(wire.Packet)
	if err != nil {
		return nil, err
	}
	newBlk, err := massutil.NewBlockFromBytes(buf, wire.Packet)
	if err != nil {
		return nil, err
	}
	return newBlk.MsgBlock(), nil
}
