package wire

import (
	"testing"
)

func isMerkleStoreEqual(src, target []*Hash) bool {
	if len(src) != len(target) {
		return false
	}
	for i := range src {
		if !src[i].IsEqual(target[i]) {
			return false
		}
	}
	return true
}

func TestBuildMerkleTreeStoreTransactions(t *testing.T) {
	witnessTx := MsgTx{
		Version: 1,
		TxIn: []*TxIn{
			{
				PreviousOutPoint: OutPoint{
					Hash:  Hash{},
					Index: MaxPrevOutIndex,
				},
				Sequence: MaxTxInSequenceNum,
				Witness: TxWitness{
					mustDecodeString("df03875e006baf9e4b410a69181f804920d499d9bcac689081a39bd0e546f2a01cb848390d6c5e0bd318f95648a4c34eb79e82a8645a1ce64a0cd715721cd758e4b410a69181f804920d49"),
					mustDecodeString("22f52c7e4f53be67a07a71875d878f4df9364172b43efc302f67c7f34d93008d22d2d31e76654a1b8176d9d0e1"),
				},
			},
		},
		TxOut: []*TxOut{
			{
				Value:    0x47868c000,
				PkScript: mustDecodeString("0020ba60494593fe65bea35fe9e118c129e5478ce660cec07c8ea8a7e2ec841fccd2"),
			},
		},
		LockTime: 0,
		Payload:  mustDecodeString("000000000000000000000000"),
	}

	tests := []*struct {
		txs     []*MsgTx
		witness bool
		merkles []*Hash
	}{
		{
			txs:     []*MsgTx{&tstGenesisCoinbaseTx},
			witness: true,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&tstGenesisCoinbaseTx},
			witness: false,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&tstGenesisCoinbaseTx, &tstGenesisCoinbaseTx},
			witness: true,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("68a9f72be29811f8e36319f89230f046f4c585a70720ecde4498d7b6f8fa8f74").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&tstGenesisCoinbaseTx, &tstGenesisCoinbaseTx, &tstGenesisCoinbaseTx},
			witness: true,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				nil,
				mustDecodeHash("68a9f72be29811f8e36319f89230f046f4c585a70720ecde4498d7b6f8fa8f74").Ptr(),
				mustDecodeHash("68a9f72be29811f8e36319f89230f046f4c585a70720ecde4498d7b6f8fa8f74").Ptr(),
				mustDecodeHash("31db74d79a776243fd441e37d79e8ae51c91e650e05ef5f8cecf534d0a46b812").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&tstGenesisCoinbaseTx, &tstGenesisCoinbaseTx, &tstGenesisCoinbaseTx, &tstGenesisCoinbaseTx},
			witness: true,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("68a9f72be29811f8e36319f89230f046f4c585a70720ecde4498d7b6f8fa8f74").Ptr(),
				mustDecodeHash("68a9f72be29811f8e36319f89230f046f4c585a70720ecde4498d7b6f8fa8f74").Ptr(),
				mustDecodeHash("31db74d79a776243fd441e37d79e8ae51c91e650e05ef5f8cecf534d0a46b812").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&witnessTx},
			witness: true,
			merkles: []*Hash{
				mustDecodeHash("21498b7668beeded2e9dddc61152c28994fcd776674fd2f61c343decf4b592b1").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&witnessTx},
			witness: false,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&tstGenesisCoinbaseTx, &witnessTx},
			witness: true,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("21498b7668beeded2e9dddc61152c28994fcd776674fd2f61c343decf4b592b1").Ptr(),
				mustDecodeHash("09408bf5b020efa52d0916316ef07f04de37debf6679b96d360e5b1ff3bf8efa").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&tstGenesisCoinbaseTx, &witnessTx},
			witness: false,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("68a9f72be29811f8e36319f89230f046f4c585a70720ecde4498d7b6f8fa8f74").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&tstGenesisCoinbaseTx, &witnessTx, &witnessTx},
			witness: true,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("21498b7668beeded2e9dddc61152c28994fcd776674fd2f61c343decf4b592b1").Ptr(),
				mustDecodeHash("21498b7668beeded2e9dddc61152c28994fcd776674fd2f61c343decf4b592b1").Ptr(),
				nil,
				mustDecodeHash("09408bf5b020efa52d0916316ef07f04de37debf6679b96d360e5b1ff3bf8efa").Ptr(),
				mustDecodeHash("844acf001e2c4f4ee83b98dc1d0d6047f928a0fab592bba85911aa122698d70d").Ptr(),
				mustDecodeHash("a8f830a34ab0e05209d2c79bfc12ec6632f28292f73fd70811fdb5607042380b").Ptr(),
			},
		},
		{
			txs:     []*MsgTx{&tstGenesisCoinbaseTx, &witnessTx, &witnessTx, &tstGenesisCoinbaseTx},
			witness: true,
			merkles: []*Hash{
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("21498b7668beeded2e9dddc61152c28994fcd776674fd2f61c343decf4b592b1").Ptr(),
				mustDecodeHash("21498b7668beeded2e9dddc61152c28994fcd776674fd2f61c343decf4b592b1").Ptr(),
				mustDecodeHash("0a084691f90ffd9ac09db47648327b6df15334ad95388badbc9edb963f6f82cb").Ptr(),
				mustDecodeHash("09408bf5b020efa52d0916316ef07f04de37debf6679b96d360e5b1ff3bf8efa").Ptr(),
				mustDecodeHash("cff7f8b1af5d10ff79d3ea1a2f9d23f28c494648465468795b2b34e161161db0").Ptr(),
				mustDecodeHash("2d7954f1466ef99aeaeade145683798e853df43341dca060f293bdef1f32009d").Ptr(),
			},
		},
	}

	for i, test := range tests {
		if merkles := BuildMerkleTreeStoreTransactions(test.txs, test.witness); !isMerkleStoreEqual(merkles, test.merkles) {
			t.Errorf("%d, BuildMerkleTreeStoreTransactions not equal, got = %v, want = %v", i, merkles, test.merkles)
		}
	}
}

func TestBuildMerkleTreeStoreForProposal(t *testing.T) {
	var emptyProposal = newEmptyProposalArea()
	var fpk = &FaultPubKey{
		version:   ProposalVersion,
		PubKey:    tstGenesisHeader.PubKey,
		Testimony: [2]*BlockHeader{&tstGenesisHeader, &tstGenesisHeader},
	}
	var anyMessage = &NormalProposal{
		version:      ProposalVersion,
		proposalType: typeAnyMessage,
		content:      mustDecodeString("ee26300e0f068114a680a772e080507c0f9c0ca4335c382c42b78e2eafbebaa3"),
	}

	tests := []*struct {
		pa      *ProposalArea
		merkles []*Hash
	}{
		{
			pa: emptyProposal,
			merkles: []*Hash{
				mustDecodeHash("9663440551fdcd6ada50b1fa1b0003d19bc7944955820b54ab569eb9a7ab7999").Ptr(),
			},
		},
		{
			pa: &ProposalArea{
				PunishmentArea: []*FaultPubKey{fpk},
				OtherArea:      []*NormalProposal{},
			},
			merkles: []*Hash{
				mustDecodeHash("0702b3269218ab4292e6925ca5d467661b280c747992880bfc5d9e19a2899619").Ptr(),
			},
		},
		{
			pa: &ProposalArea{
				PunishmentArea: []*FaultPubKey{},
				OtherArea:      []*NormalProposal{anyMessage},
			},
			merkles: []*Hash{
				mustDecodeHash("9663440551fdcd6ada50b1fa1b0003d19bc7944955820b54ab569eb9a7ab7999").Ptr(),
				mustDecodeHash("8d6e1280a9d3a42d84b66ef15914081410998a3d4ea202324186b2ef37a7f21c").Ptr(),
				mustDecodeHash("23e1f1d9f27af39a28153547cfaea9dbdaa02805a1e0b0e380982dee130f6a4e").Ptr(),
			},
		},
		{
			pa: &ProposalArea{
				PunishmentArea: []*FaultPubKey{fpk},
				OtherArea:      []*NormalProposal{anyMessage},
			},
			merkles: []*Hash{
				mustDecodeHash("0702b3269218ab4292e6925ca5d467661b280c747992880bfc5d9e19a2899619").Ptr(),
				mustDecodeHash("8d6e1280a9d3a42d84b66ef15914081410998a3d4ea202324186b2ef37a7f21c").Ptr(),
				mustDecodeHash("68ba6b123985a31055187b8d72e06eb23219fe2fabe2004bc3d49b77a6b287b5").Ptr(),
			},
		},
		{
			pa: &ProposalArea{
				PunishmentArea: []*FaultPubKey{fpk},
				OtherArea:      []*NormalProposal{anyMessage, anyMessage},
			},
			merkles: []*Hash{
				mustDecodeHash("0702b3269218ab4292e6925ca5d467661b280c747992880bfc5d9e19a2899619").Ptr(),
				mustDecodeHash("8d6e1280a9d3a42d84b66ef15914081410998a3d4ea202324186b2ef37a7f21c").Ptr(),
				mustDecodeHash("fbe1cade5a8724f5d182a5755e6cf73abe7bc4a5f22a9416c17b1a547597c815").Ptr(),
				nil,
				mustDecodeHash("68ba6b123985a31055187b8d72e06eb23219fe2fabe2004bc3d49b77a6b287b5").Ptr(),
				mustDecodeHash("3a67d5c3919fed431536424bf71759a1b34717302360976c8a63974d7b541f45").Ptr(),
				mustDecodeHash("ebe68c2665d8060344efb1eabfaf66f35b4b2262af2cc820e4ee00bf6e114a23").Ptr(),
			},
		},
		{
			pa: &ProposalArea{
				PunishmentArea: []*FaultPubKey{fpk},
				OtherArea:      []*NormalProposal{anyMessage, anyMessage, anyMessage},
			},
			merkles: []*Hash{
				mustDecodeHash("0702b3269218ab4292e6925ca5d467661b280c747992880bfc5d9e19a2899619").Ptr(),
				mustDecodeHash("8d6e1280a9d3a42d84b66ef15914081410998a3d4ea202324186b2ef37a7f21c").Ptr(),
				mustDecodeHash("fbe1cade5a8724f5d182a5755e6cf73abe7bc4a5f22a9416c17b1a547597c815").Ptr(),
				mustDecodeHash("dad91c9382559fb727edb83872258b2e8b8ce33aac9025934d37da75b6903d26").Ptr(),
				mustDecodeHash("68ba6b123985a31055187b8d72e06eb23219fe2fabe2004bc3d49b77a6b287b5").Ptr(),
				mustDecodeHash("4c7434b71a9c7f516c6159e99c414e0fa628831f8d3f54b4e537a0e100186a60").Ptr(),
				mustDecodeHash("203bc9a651edc733a095da93384f0d88de3d80fcff6dddf0e0d6a99fd59feae7").Ptr(),
			},
		},
		{
			pa: &ProposalArea{
				PunishmentArea: []*FaultPubKey{fpk, fpk, fpk},
				OtherArea:      []*NormalProposal{},
			},
			merkles: []*Hash{
				mustDecodeHash("0702b3269218ab4292e6925ca5d467661b280c747992880bfc5d9e19a2899619").Ptr(),
				mustDecodeHash("2eea13989c950de52c785edbfa5978744fbb244563878d5e637f89c37032ac5c").Ptr(),
				mustDecodeHash("a01debb91a5bbf3f7a0f66f4fc12a1a522533edc5d5becf74c2ad90b7bc917be").Ptr(),
				nil,
				mustDecodeHash("31d49ecd641b49290b4023ffbd28dc32ed629834ddf1d9f8f29c8335fc0817c6").Ptr(),
				mustDecodeHash("df54a14663383b26bcabb6acec56a01a63bf1aebfd5eb58f756bea9d27b50042").Ptr(),
				mustDecodeHash("d0056ebcbb1619319a8628a612d3812d080bbb43603f4a6ecbcfb0d548b03226").Ptr(),
			},
		},
		{
			pa: &ProposalArea{
				PunishmentArea: []*FaultPubKey{fpk, fpk, fpk, fpk},
				OtherArea:      []*NormalProposal{},
			},
			merkles: []*Hash{
				mustDecodeHash("0702b3269218ab4292e6925ca5d467661b280c747992880bfc5d9e19a2899619").Ptr(),
				mustDecodeHash("2eea13989c950de52c785edbfa5978744fbb244563878d5e637f89c37032ac5c").Ptr(),
				mustDecodeHash("a01debb91a5bbf3f7a0f66f4fc12a1a522533edc5d5becf74c2ad90b7bc917be").Ptr(),
				mustDecodeHash("61b37b1323bfa9cb199589572bff148ffdc1f2e005a4020edb2ac9ee43f264a6").Ptr(),
				mustDecodeHash("31d49ecd641b49290b4023ffbd28dc32ed629834ddf1d9f8f29c8335fc0817c6").Ptr(),
				mustDecodeHash("c1dbbdb34fec592314cecd43db3d64be2d516f92f391564028493c95adbeb6ed").Ptr(),
				mustDecodeHash("c34a6593085c8e7cd365ee8f0574e8329deddcaf324aaf2a94273730a21bf864").Ptr(),
			},
		},
	}

	for i, test := range tests {
		if merkles := BuildMerkleTreeStoreForProposal(test.pa); !isMerkleStoreEqual(merkles, test.merkles) {
			t.Errorf("%d, BuildMerkleTreeStoreTransactions not equal, got = %v, want = %v", i, merkles, test.merkles)
		}
	}
}
