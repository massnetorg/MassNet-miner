package errors

const (
	// transaction err
	ErrAPINoTxInfo        = 1101
	ErrAPINoTxOut         = 1102
	ErrAPIRawTx           = 1103
	ErrAPIDuplicateTx     = 1104
	ErrAPIInsufficient    = 1105
	ErrAPIFailedToMaxwell = 1106
	ErrAPIFindingUtxo     = 1107
	ErrAPIFindingBalance  = 1108
	ErrAPIEstimateTxFee   = 1109

	// block err
	ErrAPINewestHash          = 1201
	ErrAPIBlockNotFound       = 1202
	ErrAPINextBlock           = 1203
	ErrAPIBlockHashByHeight   = 1204
	ErrAPIBlockHeaderNotFound = 1205

	// wallet err
	ErrAPIWalletInternal     = 1301
	ErrAPICreateRedeemScript = 1302
	ErrAPICreatePubKey       = 1303
	ErrAPINoAddressInWallet  = 1304
	ErrAPICreateAddress      = 1305
	ErrAPINoPrivKeyByPubKey  = 1306
	ErrAPINoScriptByAddress  = 1307
	ErrAPINoSeedsInWallet    = 1310

	// txScript
	ErrAPICreatePkScript  = 1401
	ErrAPISignTx          = 1402
	ErrAPINewEngine       = 1403
	ErrAPIExecute         = 1404
	ErrAPIRejectTx        = 1405
	ErrAPIExtractPKScript = 1406

	// Invalid Parameter
	ErrAPIInvalidParameter = 1501
	ErrAPIInvalidLockTime  = 1502
	ErrAPIInvalidAmount    = 1503
	ErrAPIInvalidAddress   = 1504
	ErrAPIInvalidFlag      = 1505
	ErrAPIInvalidIndex     = 1506

	// Decode, Encode and deserialize err
	ErrAPIFailedDecodeAddress = 1601
	ErrAPIDecodeHexString     = 1602
	ErrAPIShaHashFromStr      = 1603
	ErrAPIEncode              = 1604
	ErrAPIDeserialization     = 1605
	ErrAPIDecodePrivKey       = 1606
	ErrAPIDisasmScript        = 1607

	// other err
	ErrAPIUnknownErr     = 1701
	ErrAPINet            = 1702
	ErrNoMinningAddrress = 1703
)

var ErrCode = map[uint32]string{

	ErrAPINoTxInfo:            "No information available about transaction",
	ErrAPIInvalidIndex:        "Invalid OutPoint index",
	ErrAPINoTxOut:             "Invalid preOutPoint",
	ErrAPIDuplicateTx:         "OutPoint index has been spent",
	ErrAPIInsufficient:        "Insufficient balance",
	ErrAPIFailedToMaxwell:     "Failed convert the amount",
	ErrAPIFindingUtxo:         "Failed to find Utxo",
	ErrAPIFindingBalance:      "Failed to find balance",
	ErrAPIWalletInternal:      "Error in wallet internal",
	ErrAPICreateRedeemScript:  "Failed to create redeem script",
	ErrAPICreatePubKey:        "Failed to create pubkey",
	ErrAPICreateAddress:       "Failed to create address",
	ErrAPINoAddressInWallet:   "There is no such address in the wallet",
	ErrAPIInvalidParameter:    "Invalid parameter",
	ErrAPIInvalidLockTime:     "Invalid locktime",
	ErrAPIInvalidAmount:       "Invalid amount",
	ErrAPIInvalidAddress:      "Invalid address",
	ErrAPIInvalidFlag:         "Invalid sighash parameter",
	ErrAPICreatePkScript:      "Failed to create pkScript",
	ErrAPIFailedDecodeAddress: "Failed to decode address",
	ErrAPIDecodeHexString:     "Argument must be hexadecimal string",
	ErrAPIShaHashFromStr:      "Failed to decode hash from string",
	ErrAPIEncode:              "Failed to encode data",
	ErrAPIDeserialization:     "Failed to deserialize",
	ErrAPIDecodePrivKey:       "Failed to decode WIF for the privkey",
	ErrAPIDisasmScript:        "Failed to disasm script to string",
	ErrAPINet:                 "Mismatched network",
	ErrAPINoPrivKeyByPubKey:   "No privkey for the pubkey found",
	ErrAPINoScriptByAddress:   "No redeem script for the address found",
	ErrAPISignTx:              "Failed to sign transaction",
	ErrAPINewEngine:           "Failed to create new engine",
	ErrAPIExecute:             "Failed to execute engine",
	ErrAPIRejectTx:            "Reject receive transaction",
	ErrAPIExtractPKScript:     "Failed to extract info from pkScript",
	ErrAPINewestHash:          "Failed to get newest hash",
	ErrAPIBlockNotFound:       "Failed to find block",
	ErrAPIRawTx:               "Failed to create raw transaction",
	ErrAPINextBlock:           "No next block",
	ErrAPIBlockHashByHeight:   "Failed to get block hash by height",
	ErrAPIBlockHeaderNotFound: "Failed to find block header",
	ErrNoMinningAddrress:      "No payment addresses specified via --miningaddr",
	ErrAPIUnknownErr:          "Unknown error",
	ErrAPIEstimateTxFee:       "Failed to estimateTxFee",
	ErrAPINoSeedsInWallet:     "No seeds",
}

//	ErrAPIType                APIErrorCode = -3
//	ErrAPIOutOfMemory         APIErrorCode = -7
//	ErrAPIDatabase            APIErrorCode = -20
//	ErrAPIVerify              APIErrorCode = -25

//
//// Peer-to-peer client errors.
//const (
//	ErrAPIClientNotConnected      APIErrorCode = -9
//	ErrAPIClientInInitialDownload APIErrorCode = -10
//)
//
//// Wallet JSON errors
//const (
//	ErrAPIWallet                    APIErrorCode = -4
//	ErrAPIWalletInvalidAccountName  APIErrorCode = -11
//	ErrAPIWalletKeypoolRanOut       APIErrorCode = -12
//	ErrAPIWalletUnlockNeeded        APIErrorCode = -13
//	ErrAPIWalletPassphraseIncorrect APIErrorCode = -14
//	ErrAPIWalletWrongEncState       APIErrorCode = -15
//	ErrAPIWalletEncryptionFailed    APIErrorCode = -16
//	ErrAPIWalletAlreadyUnlocked     APIErrorCode = -17
//)

//const (

//	ErrAPIBlockCount        APIErrorCode = -5
//	ErrAPIBestBlockHash     APIErrorCode = -5
//	ErrAPIDifficulty        APIErrorCode = -5
//	ErrAPIOutOfRange        APIErrorCode = -1

//)
//
//// Errors that are specific to mass.
//const (
//	ErrAPINoWallet      APIErrorCode = -1
//	ErrAPIUnimplemented APIErrorCode = -1
//)
