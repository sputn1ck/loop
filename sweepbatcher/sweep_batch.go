package sweepbatcher

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btclog"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/loop/labels"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightninglabs/loop/swap"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnrpc/walletrpc"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
)

const (
	defaultFeeRateStep     = chainfee.SatPerKWeight(100)
	defaultBatchConfTarget = 12
	batchConfHeight        = 3
	batchPublishDelay      = time.Millisecond * 100
	maxFeeToSwapAmtRatio   = 0.2
)

// sweep stores any data related to sweeping a specific outpoint.
type sweep struct {
	// swapHash is the hash of the swap that the sweep belongs to.
	swapHash lntypes.Hash

	// outpoint is the outpoint being swept.
	outpoint wire.OutPoint

	// value is the value of the outpoint being swept.
	value btcutil.Amount

	// confTarget is the confirmation target of the sweep.
	confTarget int32

	// timeout is the timeout of the swap that the sweep belongs to.
	timeout int32

	// initiationHeight is the height at which the swap was initiated.
	initiationHeight int32

	// htlc is the HTLC that is being swept.
	htlc swap.Htlc

	// preimage is the preimage of the HTLC that is being swept.
	preimage lntypes.Preimage

	// swapInvoicePaymentAddr is the payment address of the swap invoice.
	swapInvoicePaymentAddr [32]byte

	// htlcKeys is the set of keys used to sign the HTLC.
	htlcKeys loopdb.HtlcKeys

	// htlcSuccessEstimator is a function that estimates the weight of the
	// HTLC success script.
	htlcSuccessEstimator func(*input.TxWeightEstimator) error

	// protocolVersion is the protocol version of the swap that the sweep
	// belongs to.
	protocolVersion loopdb.ProtocolVersion

	// isExternalAddr is true if the sweep spends to a non-wallet address.
	isExternalAddr bool

	// destAddr is the destination address of the sweep.
	destAddr btcutil.Address

	// notifier is a collection of channels used to communicate the status
	// of the sweep back to the swap that requested it.
	notifier SpendNotifier
}

// batchState is the state of the batch.
type batchState uint8

const (
	// Open is the state in which the batch is able to accept new sweeps.
	Open batchState = 0

	// Closed is the state in which the batch is no longer able to accept
	// new sweeps.
	Closed batchState = 1

	// Confirmed is the state in which the batch transaction has reached the
	// configured conf height.
	Confirmed batchState = 2
)

// batchConfig is the configuration for a batch.
type batchConfig struct {
	// MaxTimeoutDistance is the maximum timeout distance that 2 distinct
	// sweeps can have in the same batch.
	MaxTimeoutDistance int32

	// BatchConfTarget is the confirmation target of the batch transaction.
	BatchConfTarget int32
}

// rbfCache stores data related to our last fee bump.
type rbfCache struct {
	// LastHeight is the last height at which we increased our feerate.
	LastHeight int32

	// FeeRate is the last used fee rate we used to publish a batch tx.
	FeeRate chainfee.SatPerKWeight
}

// batch is a collection of sweeps that are published together.
type batch struct {
	// id is the primary identifier of this batch.
	id int32

	// state is the current state of the batch.
	state batchState

	// primarySweepID is the swap hash of the primary sweep in the batch.
	primarySweepID lntypes.Hash

	// sweeps store the sweeps that this batch currently contains.
	sweeps map[lntypes.Hash]sweep

	// sweepsLock is a mutex that is used to protect the sweeps map from
	// concurrent access.
	sweepsLock sync.Mutex

	// currentHeight is the current block height.
	currentHeight int32

	// requestChan is the channel over which new sweeps are received from.
	requestChan chan *sweep

	// returnChan is the channel over which sweeps are returned back to the
	// batcher in order to enter a new batch. This occurs when some sweeps
	// are left out because an older version of the batch tx got confirmed.
	// This is a behavior introduced by RBF replacements.
	returnChan chan SweepRequest

	// blockEpochChan is the channel over which block epoch notifications
	// are received.
	blockEpochChan chan int32

	// spendChan is the channel over which spend notifications are received.
	spendChan chan *chainntnfs.SpendDetail

	// confChan is the channel over which confirmation notifications are
	// received.
	confChan chan *chainntnfs.TxConfirmation

	// errChan is the channel over which errors are received.
	errChan chan error

	// completeChan is the channel over which the batch sends its ID back
	// to the batcher to signal its exit.
	completeChan chan int32

	// batchTx is the transaction that is currently being monitored for
	// confirmations.
	batchTxid *chainhash.Hash

	// batchPkScript is the pkScript of the batch transaction's output.
	batchPkScript []byte

	// batchAddress is the address of the batch transaction's output.
	batchAddress btcutil.Address

	// rbfCache stores data related to the RBF fee bumping mechanism.
	rbfCache rbfCache

	// wallet is the wallet client used to create and publish the batch
	// transaction.
	wallet lndclient.WalletKitClient

	// chainNotifier is the chain notifier client used to monitor the
	// blockchain for spends and confirmations.
	chainNotifier lndclient.ChainNotifierClient

	// signerClient is the signer client used to sign the batch transaction.
	signerClient lndclient.SignerClient

	// muSig2Kit includes all the required functionality to collect
	// and verify signatures by the swap server in order to cooperatively
	// sweep funds.
	muSig2SignSweep MuSig2SignSweep

	// verifySchnorrSig is a function that verifies a schnorr signature.
	verifySchnorrSig VerifySchnorrSig

	// store includes all the database interactions that are needed by the
	// batch.
	store BatcherStore

	// Cfg is the configuration for this batch.
	Cfg *batchConfig

	// log is the logger for this batch.
	log btclog.Logger

	wg sync.WaitGroup
}

// batchKit is a kit of dependencies that are used to initialize a batch. This
// struct is only used as a wrapper for the arguments that are required to
// create a new batch.
type batchKit struct {
	id               int32
	batchTxid        *chainhash.Hash
	batchPkScript    []byte
	state            batchState
	primaryID        lntypes.Hash
	sweeps           map[lntypes.Hash]sweep
	rbfCache         rbfCache
	returnChan       chan SweepRequest
	completeChan     chan int32
	wallet           lndclient.WalletKitClient
	chainNotifier    lndclient.ChainNotifierClient
	signerClient     lndclient.SignerClient
	musig2SignSweep  MuSig2SignSweep
	verifySchnorrSig VerifySchnorrSig
	store            BatcherStore
	log              btclog.Logger
}

// NewBatch creates a new batch.
func NewBatch(cfg batchConfig, bk batchKit) *batch {
	return &batch{
		// We set the ID to a negative value to flag that this batch has
		// never been persisted, so it needs to be assigned a new ID.
		id:               -1,
		state:            Open,
		sweeps:           make(map[lntypes.Hash]sweep),
		requestChan:      make(chan *sweep),
		returnChan:       bk.returnChan,
		blockEpochChan:   make(chan int32),
		spendChan:        make(chan *chainntnfs.SpendDetail),
		confChan:         make(chan *chainntnfs.TxConfirmation),
		errChan:          make(chan error),
		completeChan:     bk.completeChan,
		batchTxid:        bk.batchTxid,
		wallet:           bk.wallet,
		chainNotifier:    bk.chainNotifier,
		signerClient:     bk.signerClient,
		muSig2SignSweep:  bk.musig2SignSweep,
		verifySchnorrSig: bk.verifySchnorrSig,
		store:            bk.store,
		Cfg:              &cfg,
	}
}

// NewBatchFromDB creates a new batch that already existed in storage.
func NewBatchFromDB(cfg batchConfig, bk batchKit) *batch {
	return &batch{
		id:               bk.id,
		state:            bk.state,
		primarySweepID:   bk.primaryID,
		sweeps:           bk.sweeps,
		requestChan:      make(chan *sweep),
		returnChan:       bk.returnChan,
		blockEpochChan:   make(chan int32),
		spendChan:        make(chan *chainntnfs.SpendDetail),
		confChan:         make(chan *chainntnfs.TxConfirmation),
		errChan:          make(chan error),
		completeChan:     bk.completeChan,
		batchTxid:        bk.batchTxid,
		batchPkScript:    bk.batchPkScript,
		rbfCache:         bk.rbfCache,
		wallet:           bk.wallet,
		chainNotifier:    bk.chainNotifier,
		signerClient:     bk.signerClient,
		muSig2SignSweep:  bk.musig2SignSweep,
		verifySchnorrSig: bk.verifySchnorrSig,
		store:            bk.store,
		log:              bk.log,
		Cfg:              &cfg,
	}
}

// addSweep adds a sweep to the batch. If this is the first sweep being added
// to the batch then it also sets the primary sweep ID.
func (b *batch) addSweep(ctx context.Context, sw sweep) error {
	b.sweepsLock.Lock()
	defer b.sweepsLock.Unlock()

	if b.primarySweepID == lntypes.ZeroHash {
		b.primarySweepID = sw.swapHash
		b.Cfg.BatchConfTarget = sw.confTarget

		// If this is the first sweep being added to the batch, we also
		// need to start the spend monitor for this new primary sweep.
		b.monitorSpend(ctx, sw)
		// b.wg.Add(1)
		// go func() {
		// 	defer b.wg.Done()
		// 	b.monitorSpend(ctx)
		// }()
	}

	if b.primarySweepID == sw.swapHash {
		b.Cfg.BatchConfTarget = sw.confTarget
	}

	_, ok := b.sweeps[sw.swapHash]
	if !ok {
		b.log.Infof("adding sweep %x", sw.swapHash[:6])
	}

	b.sweeps[sw.swapHash] = sw

	err := b.persistSweep(ctx, sw, false)
	if err != nil {
		b.log.Errorf("error while persisting: %v", err)
	}

	return err
}

// sweepExists returns true if the batch contains the sweep with the given hash.
func (b *batch) sweepExists(hash lntypes.Hash) bool {
	b.sweepsLock.Lock()
	_, ok := b.sweeps[hash]
	b.sweepsLock.Unlock()
	return ok
}

// AcceptsSweep returns true if the batch is able to accept the given sweep. To
// accept a sweep a batch needs to be in an Open state, and the incoming sweep's
// timeout must not be too far away from the other timeouts of existing sweeps.
// If this batch contains a single sweep that spends to a non-wallet address we
// are not accepting other sweeps as batching is pointless in this case.
func (b *batch) AcceptsSweep(sweep *sweep) bool {
	b.sweepsLock.Lock()
	defer b.sweepsLock.Unlock()

	if sweep == nil {
		return false
	}

	if b.state != Open {
		return false
	}

	for _, s := range b.sweeps {
		if s.isExternalAddr || sweep.isExternalAddr {
			return false
		}
	}

	for _, s := range b.sweeps {
		timeoutDistance :=
			int32(math.Abs(float64(sweep.timeout - s.timeout)))

		if timeoutDistance > b.Cfg.MaxTimeoutDistance {
			return false
		}
	}

	return true
}

// Wait waits for the batch to gracefully stop.
func (b *batch) Wait() {
	b.log.Infof("Stopping")
	b.wg.Wait()
}

// Run is the batch's main event loop.
func (b *batch) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		close(b.requestChan)
		b.wg.Wait()
	}()

	if b.muSig2SignSweep == nil {
		return fmt.Errorf("no musig2 signer available")
	}

	blockChan, blockErrChan, err :=
		b.chainNotifier.RegisterBlockEpochNtfn(runCtx)
	if err != nil {
		return err
	}

	b.sweepsLock.Lock()

	// If a primary sweep exists we immediately start monitoring for its
	// spend.
	if b.primarySweepID != lntypes.ZeroHash {
		sweep := b.sweeps[b.primarySweepID]
		b.monitorSpend(runCtx, sweep)
	}

	b.sweepsLock.Unlock()

	// We use a timer chan in order to trigger the batch publishment a few
	// seconds after a new block is received. This is done avoid publishing
	// the transaction on the same block in which it was spent. By waiting,
	// we give the tiny amount of time needed by the spend monitor to
	// receive the notification and close the batch, preventing further
	// publishments.
	var timerChan <-chan time.Time

	b.log.Infof("started, primary %x, total sweeps %v",
		b.primarySweepID[0:6], len(b.sweeps))

	for {
		select {
		case sweep := <-b.requestChan:
			err := b.addSweep(runCtx, *sweep)
			if err != nil {
				return err
			}

		case height := <-blockChan:
			b.log.Debugf("received block %v", height)

			// We skip the publish if this was the first height we
			// received, as this means we just started running. This
			// prevents immediately publishing a transaction on
			// batch creation.
			if b.currentHeight > 0 {
				timerChan = time.After(batchPublishDelay)
			} else {
				b.log.Debugf("skipping publish on first height")
			}

			b.currentHeight = height

		case <-timerChan:
			if b.state == Open {
				err := b.publish(ctx)
				if err != nil {
					return err
				}
			}

		case spend := <-b.spendChan:
			err := b.handleSpend(runCtx, spend.SpendingTx)
			if err != nil {
				return err
			}

		case <-b.confChan:
			return b.handleConf(runCtx)

		case err := <-blockErrChan:
			return err

		case err := <-b.errChan:
			return err

		case <-runCtx.Done():
			return runCtx.Err()
		}
	}
}

// publish creates and publishes the latest batch transaction to the network.
func (b *batch) publish(ctx context.Context) error {
	var (
		err         error
		fee         btcutil.Amount
		coopSuccess bool
	)

	// Run the RBF rate update.
	err = b.updateRbfRate(ctx)
	if err != nil {
		return err
	}

	fee, err, coopSuccess = b.publishBatchCoop(ctx)
	if err != nil {
		b.log.Warnf("co-op publish error: %v", err)
	}

	if !coopSuccess {
		fee, err = b.publishBatch(ctx)
	}
	if err != nil {
		b.log.Warnf("publish error: %v", err)
		return nil
	}

	b.log.Infof("published, total sweeps: %v, fees: %v", len(b.sweeps), fee)
	for _, sweep := range b.sweeps {
		b.log.Infof("published sweep %x, value: %v",
			sweep.swapHash[:6], sweep.value)
	}

	return b.persist(ctx)
}

// publishBatch creates and publishes the batch transaction. It will consult the
// RBFCache to determine the fee rate to use.
func (b *batch) publishBatch(ctx context.Context) (btcutil.Amount, error) {
	b.sweepsLock.Lock()
	defer b.sweepsLock.Unlock()

	// Create the batch transaction.
	batchTx := wire.NewMsgTx(2)
	batchTx.LockTime = uint32(b.currentHeight)

	var (
		batchAmt     btcutil.Amount
		prevOuts     = make([]*wire.TxOut, 0, len(b.sweeps))
		signDescs    = make([]*lndclient.SignDescriptor, 0, len(b.sweeps))
		sweeps       = make([]sweep, 0, len(b.sweeps))
		fee          btcutil.Amount
		inputCounter int
		addrOverride bool
	)

	var weightEstimate input.TxWeightEstimator

	// Add all the sweeps to the batch transaction.
	for _, sweep := range b.sweeps {
		if sweep.isExternalAddr {
			addrOverride = true
		}

		batchAmt += sweep.value
		batchTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: sweep.outpoint,
			Sequence:         sweep.htlc.SuccessSequence(),
		})

		err := sweep.htlcSuccessEstimator(&weightEstimate)
		if err != nil {
			return 0, err
		}

		// Append this sweep to an array of sweeps. This is needed to
		// keep the order of sweeps stored, as iterating the sweeps map
		// does not guarantee same order.
		sweeps = append(sweeps, sweep)

		// Create and store the previous outpoint for this sweep.
		prevOuts = append(prevOuts, &wire.TxOut{
			Value:    int64(sweep.value),
			PkScript: sweep.htlc.PkScript,
		})

		key, err := btcec.ParsePubKey(
			sweep.htlcKeys.ReceiverScriptKey[:],
		)
		if err != nil {
			return fee, err
		}

		// Create and store the sign descriptor for this sweep.
		signDesc := lndclient.SignDescriptor{
			WitnessScript: sweep.htlc.SuccessScript(),
			Output:        prevOuts[len(prevOuts)-1],
			HashType:      sweep.htlc.SigHash(),
			InputIndex:    inputCounter,
			KeyDesc: keychain.KeyDescriptor{
				PubKey: key,
			},
		}

		inputCounter++

		if sweep.htlc.Version == swap.HtlcV3 {
			signDesc.SignMethod = input.TaprootScriptSpendSignMethod
		}

		signDescs = append(signDescs, &signDesc)
	}

	var address btcutil.Address

	if addrOverride {
		// Sanity check, there should be exactly 1 sweep in this batch.
		if len(sweeps) != 1 {
			return 0, fmt.Errorf("external address sweep batched " +
				"with other sweeps")
		}

		address = sweeps[0].destAddr
	} else {
		var err error
		address, err = b.getBatchDestAddr(ctx)
		if err != nil {
			return fee, err
		}
	}

	batchPkScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return fee, err
	}

	weightEstimate.AddP2TROutput()

	totalWeight := int64(weightEstimate.Weight())

	fee = b.rbfCache.FeeRate.FeeForWeight(totalWeight)

	// Clamp the calculated fee to the max allowed fee amount for the batch.
	fee = clampBatchFee(fee, batchAmt)

	// Add the batch transaction output, which excludes the fees paid to
	// miners.
	batchTx.AddTxOut(&wire.TxOut{
		PkScript: batchPkScript,
		Value:    int64(batchAmt - fee),
	})

	// Collect the signatures for our inputs.
	rawSigs, err := b.signerClient.SignOutputRaw(
		ctx, batchTx, signDescs, prevOuts,
	)
	if err != nil {
		return fee, err
	}

	for i, sweep := range sweeps {
		// Generate the success witness for the sweep.
		witness, err := sweep.htlc.GenSuccessWitness(
			rawSigs[i], sweep.preimage,
		)
		if err != nil {
			return fee, err
		}

		// Add the success witness to our batch transaction's inputs.
		batchTx.TxIn[i].Witness = witness
	}

	b.log.Debugf("attempting to publish non-coop tx with feerate=%v, "+
		"totalfee=%v, sweeps=%v, destAddr=%s", b.rbfCache.FeeRate, fee,
		len(batchTx.TxIn), address.String())

	err = b.wallet.PublishTransaction(
		ctx, batchTx, labels.LoopOutBatchSweepSuccess(b.id),
	)
	if err != nil {
		return fee, err
	}

	// Store the batch transaction's txid and pkScript, for monitoring
	// purposes.
	txHash := batchTx.TxHash()
	b.batchTxid = &txHash
	b.batchPkScript = batchPkScript

	return fee, nil
}

// publishBatchCoop attempts to construct and publish a batch transaction that
// collects all the required signatures interactively from the server. This
// helps with collecting the funds immediately without revealing any information
// related to the HTLC script.
func (b *batch) publishBatchCoop(ctx context.Context) (btcutil.Amount,
	error, bool) {

	b.sweepsLock.Lock()
	defer b.sweepsLock.Unlock()

	var (
		batchAmt       = btcutil.Amount(0)
		sweeps         = make([]sweep, 0, len(b.sweeps))
		fee            = btcutil.Amount(0)
		weightEstimate input.TxWeightEstimator
		addrOverride   bool
	)

	// Sanity check, there should be at least 1 sweep in this batch.
	if len(b.sweeps) == 0 {
		return 0, fmt.Errorf("no sweeps in batch"), false
	}

	// Create the batch transaction.
	batchTx := &wire.MsgTx{
		Version:  2,
		LockTime: uint32(b.currentHeight),
	}

	for _, sweep := range b.sweeps {
		// Append this sweep to an array of sweeps. This is needed to
		// keep the order of sweeps stored, as iterating the sweeps map
		// does not guarantee same order.
		sweeps = append(sweeps, sweep)
	}

	// Add all the sweeps to the batch transaction.
	for _, sweep := range sweeps {
		if sweep.isExternalAddr {
			addrOverride = true
		}

		// Keep track of the total amount this batch is sweeping back.
		batchAmt += sweep.value

		// Add this sweep's input to the transaction.
		batchTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: sweep.outpoint,
		})

		err := sweep.htlcSuccessEstimator(&weightEstimate)
		if err != nil {
			return fee, err, false
		}
	}

	var address btcutil.Address

	if addrOverride {
		// Sanity check, there should be exactly 1 sweep in this batch.
		if len(sweeps) != 1 {
			return 0, fmt.Errorf("external address sweep batched " +
				"with other sweeps"), false
		}

		address = sweeps[0].destAddr
	} else {
		var err error
		address, err = b.getBatchDestAddr(ctx)
		if err != nil {
			return fee, err, false
		}
	}

	batchPkScript, err := txscript.PayToAddrScript(address)
	if err != nil {
		return fee, err, false
	}

	weightEstimate.AddP2TROutput()

	totalWeight := int64(weightEstimate.Weight())

	fee = b.rbfCache.FeeRate.FeeForWeight(totalWeight)

	// Clamp the calculated fee to the max allowed fee amount for the batch.
	fee = clampBatchFee(fee, batchAmt)

	// Add the batch transaction output, which excludes the fees paid to
	// miners.
	batchTx.AddTxOut(&wire.TxOut{
		PkScript: batchPkScript,
		Value:    int64(batchAmt - fee),
	})

	packet, err := psbt.NewFromUnsignedTx(batchTx)
	if err != nil {
		return fee, err, false
	}

	if len(packet.Inputs) != len(sweeps) {
		return fee, fmt.Errorf("invalid number of packet inputs"), false
	}

	prevOuts := make(map[wire.OutPoint]*wire.TxOut)

	for i, sweep := range sweeps {
		txOut := &wire.TxOut{
			Value:    int64(sweep.value),
			PkScript: sweep.htlc.PkScript,
		}

		prevOuts[sweep.outpoint] = txOut
		packet.Inputs[i].WitnessUtxo = txOut
	}

	var psbtBuf bytes.Buffer
	err = packet.Serialize(&psbtBuf)
	if err != nil {
		return fee, err, false
	}

	prevOutputFetcher := txscript.NewMultiPrevOutFetcher(prevOuts)

	// Attempt to cooperatively sign the batch tx with the server.
	err = b.coopSignBatchTx(
		ctx, packet, sweeps, prevOutputFetcher, prevOuts, psbtBuf,
	)
	if err != nil {
		return fee, err, false
	}

	b.log.Debugf("attempting to publish coop tx with feerate=%v, "+
		"totalfee=%v, sweeps=%v, destAddr=%s", b.rbfCache.FeeRate, fee,
		len(batchTx.TxIn), address.String())

	err = b.wallet.PublishTransaction(
		ctx, batchTx, labels.LoopOutBatchSweepSuccess(b.id),
	)
	if err != nil {
		return fee, err, true
	}

	// Store the batch transaction's txid and pkScript, for monitoring
	// purposes.
	txHash := batchTx.TxHash()
	b.batchTxid = &txHash
	b.batchPkScript = batchPkScript

	return fee, nil, true
}

// coopSignBatchTx collects the necessary signatures from the server in order
// to cooperatively sweep the funds.
func (b *batch) coopSignBatchTx(ctx context.Context, packet *psbt.Packet,
	sweeps []sweep, prevOutputFetcher *txscript.MultiPrevOutFetcher,
	prevOuts map[wire.OutPoint]*wire.TxOut, psbtBuf bytes.Buffer) error {

	for i, sweep := range sweeps {
		sweep := sweep

		sigHashes := txscript.NewTxSigHashes(
			packet.UnsignedTx, prevOutputFetcher,
		)

		sigHash, err := txscript.CalcTaprootSignatureHash(
			sigHashes, txscript.SigHashDefault, packet.UnsignedTx,
			i, prevOutputFetcher,
		)
		if err != nil {
			return err
		}

		var (
			signers       [][]byte
			muSig2Version input.MuSig2Version
		)

		// Depending on the MuSig2 version we either pass 32 byte
		// Schnorr public keys or normal 33 byte public keys.
		if sweep.protocolVersion >= loopdb.ProtocolVersionMuSig2 {
			muSig2Version = input.MuSig2Version100RC2
			signers = [][]byte{
				sweep.htlcKeys.SenderInternalPubKey[:],
				sweep.htlcKeys.ReceiverInternalPubKey[:],
			}
		} else {
			muSig2Version = input.MuSig2Version040
			signers = [][]byte{
				sweep.htlcKeys.SenderInternalPubKey[1:],
				sweep.htlcKeys.ReceiverInternalPubKey[1:],
			}
		}

		htlcScript, ok := sweep.htlc.HtlcScript.(*swap.HtlcScriptV3)
		if !ok {
			return fmt.Errorf("invalid htlc script version")
		}

		// Now we're creating a local MuSig2 session using the receiver
		// key's key locator and the htlc's root hash.
		musig2SessionInfo, err := b.signerClient.MuSig2CreateSession(
			ctx, muSig2Version,
			&sweep.htlcKeys.ClientScriptKeyLocator, signers,
			lndclient.MuSig2TaprootTweakOpt(
				htlcScript.RootHash[:], false,
			),
		)
		if err != nil {
			return err
		}

		// With the session active, we can now send the server our
		// public nonce and the sig hash, so that it can create it's own
		// MuSig2 session and return the server side nonce and partial
		// signature.
		serverNonce, serverSig, err := b.muSig2SignSweep(
			ctx, sweep.protocolVersion, sweep.swapHash,
			sweep.swapInvoicePaymentAddr,
			musig2SessionInfo.PublicNonce[:], psbtBuf.Bytes(),
			prevOuts,
		)
		if err != nil {
			return err
		}

		var serverPublicNonce [musig2.PubNonceSize]byte
		copy(serverPublicNonce[:], serverNonce)

		// Register the server's nonce before attempting to create our
		// partial signature.
		haveAllNonces, err := b.signerClient.MuSig2RegisterNonces(
			ctx, musig2SessionInfo.SessionID,
			[][musig2.PubNonceSize]byte{serverPublicNonce},
		)
		if err != nil {
			return err
		}

		// Sanity check that we have all the nonces.
		if !haveAllNonces {
			return fmt.Errorf("invalid MuSig2 session: " +
				"nonces missing")
		}

		var digest [32]byte
		copy(digest[:], sigHash)

		// Since our MuSig2 session has all nonces, we can now create
		// the local partial signature by signing the sig hash.
		_, err = b.signerClient.MuSig2Sign(
			ctx, musig2SessionInfo.SessionID, digest, false,
		)
		if err != nil {
			return err
		}

		// Now combine the partial signatures to use the final combined
		// signature in the sweep transaction's witness.
		haveAllSigs, finalSig, err := b.signerClient.MuSig2CombineSig(
			ctx, musig2SessionInfo.SessionID, [][]byte{serverSig},
		)
		if err != nil {
			return err
		}

		if !haveAllSigs {
			return fmt.Errorf("failed to combine signatures")
		}

		// To be sure that we're good, parse and validate that the
		// combined signature is indeed valid for the sig hash and the
		// internal pubkey.
		err = b.verifySchnorrSig(
			htlcScript.TaprootKey, sigHash, finalSig,
		)
		if err != nil {
			return err
		}

		packet.UnsignedTx.TxIn[i].Witness = wire.TxWitness{
			finalSig,
		}
	}

	return nil
}

// updateRbfRate updates the fee rate we should use for the new batch
// transaction. This fee rate does not guarantee RBF success, but the continuous
// increase leads to an eventual successful RBF replacement.
func (b *batch) updateRbfRate(ctx context.Context) error {
	// If the feeRate is unset then we never published before, so we
	// retrieve the fee estimate from our wallet.
	if b.rbfCache.FeeRate == 0 {
		b.log.Infof("initializing rbf fee rate for conf target=%v",
			b.Cfg.BatchConfTarget)
		rate, err := b.wallet.EstimateFeeRate(
			ctx, b.Cfg.BatchConfTarget,
		)
		if err != nil {
			return err
		}

		// Set the initial value for our fee rate.
		b.rbfCache.FeeRate = rate
	} else {
		// Bump the fee rate by the configured step.
		b.rbfCache.FeeRate += defaultFeeRateStep
	}

	b.rbfCache.LastHeight = b.currentHeight

	return b.persist(ctx)
}

// monitorSpend monitors the primary sweep's outpoint for spends. The reason we
// monitor the primary sweep's outpoint is because the primary sweep was the
// first sweep that entered this batch, therefore it is present in all the
// versions of the batch transaction. This means that even if an older version
// of the batch transaction gets confirmed, due to the uncertainty of RBF
// replacements and network propagation, we can always detect the transaction.
func (b *batch) monitorSpend(ctx context.Context, primarySweep sweep) {
	spendCtx, cancel := context.WithCancel(ctx)

	spendChan, spendErr, err := b.chainNotifier.RegisterSpendNtfn(
		spendCtx, &primarySweep.outpoint, primarySweep.htlc.PkScript,
		primarySweep.initiationHeight,
	)
	if err != nil {
		b.writeToErrChan(ctx, err)
		cancel()
		return
	}

	b.wg.Add(1)
	go func() {
		defer cancel()
		defer b.wg.Done()

		b.log.Infof("monitoring spend for outpoint %s",
			primarySweep.outpoint.String())

		for {
			select {
			case spend := <-spendChan:
				select {
				case b.spendChan <- spend:

				case <-ctx.Done():
				}

				return

			case err := <-spendErr:
				b.writeToErrChan(ctx, err)
				return

			case <-ctx.Done():
				return
			}
		}
	}()
}

// monitorConfirmations monitors the batch transaction for confirmations.
func (b *batch) monitorConfirmations(ctx context.Context) {
	reorgChan := make(chan struct{})

	confCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	confChan, errChan, err := b.chainNotifier.RegisterConfirmationsNtfn(
		confCtx, b.batchTxid, b.batchPkScript, batchConfHeight,
		b.currentHeight, lndclient.WithReOrgChan(reorgChan),
	)
	if err != nil {
		b.log.Errorf("unable to register for confirmation ntfn: %v", err)
		b.writeToErrChan(ctx, err)
		return
	}

	for {
		select {
		case conf := <-confChan:
			select {
			case b.confChan <- conf:

			case <-ctx.Done():
			}
			return

		case err := <-errChan:
			b.writeToErrChan(ctx, err)
			return

		case <-reorgChan:
			// A re-org has been detected. We set the batch's state
			// back to open since our batch transaction is no longer
			// present in any block. We can accept more sweeps and
			// try to publish new transactions, at this point we
			// need to monitor again for a new spend.
			b.sweepsLock.Lock()
			b.state = Open
			sweep := b.sweeps[b.primarySweepID]
			b.sweepsLock.Unlock()

			b.monitorSpend(ctx, sweep)

			b.log.Warnf("reorg detected")
			return

		case <-ctx.Done():
			return
		}
	}
}

// handleSpend handles a spend notification.
func (b *batch) handleSpend(ctx context.Context, spendTx *wire.MsgTx) error {
	// As soon as a spend arrives we should no longer accept sweeps. Also,
	// the closed state will cause the batch to start monitoring the
	// confirmation of the batch transaction.
	b.sweepsLock.Lock()

	b.state = Closed
	var (
		txHash     = spendTx.TxHash()
		purgeList  = make([]SweepRequest, 0, len(b.sweeps))
		notifyList = make([]sweep, 0, len(b.sweeps))
	)
	b.batchTxid = &txHash
	b.batchPkScript = spendTx.TxOut[0].PkScript

	// As a previous version of the batch transaction may get confirmed,
	// which does not contain the latest sweeps, we need to detect the
	// sweeps that did not make it to the confirmed transaction and feed
	// them back to the batcher. This will ensure that the sweeps will enter
	// a new batch instead of remaining dangling.
	for _, sweep := range b.sweeps {
		found := false

		for _, txIn := range spendTx.TxIn {
			if txIn.PreviousOutPoint == sweep.outpoint {
				found = true
				notifyList = append(notifyList, sweep)
			}
		}

		// If the sweep's outpoint was not found in the transaction's
		// inputs this means it was left out. So we delete it from this
		// batch and feed it back to the batcher.
		if !found {
			newSweep := sweep
			delete(b.sweeps, sweep.swapHash)
			purgeList = append(purgeList, SweepRequest{
				SwapHash: newSweep.swapHash,
				Outpoint: newSweep.outpoint,
				Value:    newSweep.value,
				Notifier: newSweep.notifier,
			})
		}
	}

	b.sweepsLock.Unlock()

	for _, sweep := range notifyList {
		// Save the sweep as completed.
		err := b.persistSweep(ctx, sweep, true)
		if err != nil {
			return err
		}

		// If the sweep's notifier is empty then this means that a swap
		// is not waiting to read an update from it, so we can skip
		// the notification part.
		if sweep.notifier == (SpendNotifier{}) {
			continue
		}

		select {
		// Try to write the update to the notification channel.
		case sweep.notifier.SpendChan <- spendTx:

		// If a quit signal was provided by the swap, continue.
		case <-sweep.notifier.QuitChan:

		// If the context was canceled, return.
		case <-ctx.Done():
		}
	}

	// If we have sweeps to purge, we send them back to the batcher.
	for _, sweep := range purgeList {
		select {
		case b.returnChan <- sweep:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	b.log.Infof("spent, total sweeps: %v, purged sweeps: %v",
		len(notifyList), len(purgeList))

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.monitorConfirmations(ctx)
	}()

	return b.persist(ctx)
}

// handleConf handles a confirmation notification. This is the final step of the
// batch. Here we signal to the batcher that this batch was completed.
func (b *batch) handleConf(ctx context.Context) error {
	b.log.Infof("confirmed")

	b.sweepsLock.Lock()
	b.state = Confirmed
	b.sweepsLock.Unlock()

	select {
	case b.completeChan <- b.id:
	case <-ctx.Done():
		return ctx.Err()
	}

	return b.store.ConfirmBatch(ctx, b.id)
}

// persist updates the batch in the database.
func (b *batch) persist(ctx context.Context) error {
	bch := &Batch{}

	bch.ID = b.id
	bch.State = stateEnumToString(b.state)

	if b.batchTxid != nil {
		bch.BatchTxid = *b.batchTxid
	}

	bch.BatchPkScript = b.batchPkScript
	bch.LastRbfHeight = b.rbfCache.LastHeight
	bch.LastRbfSatPerKw = int32(b.rbfCache.FeeRate)
	bch.MaxTimeoutDistance = b.Cfg.MaxTimeoutDistance

	return b.store.UpdateSweepBatch(ctx, bch)
}

// WriteToBatchChan writes a sweep to the batch's request channel.
func (b *batch) WriteToBatchChan(ctx context.Context, sweep *sweep) error {
	select {
	case b.requestChan <- sweep:
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

// getBatchDestAddr returns the batch's destination address. If the batch
// has already generated an address then the same one will be returned.
func (b *batch) getBatchDestAddr(ctx context.Context) (btcutil.Address, error) {
	var address btcutil.Address

	// If a batch address is set, use that. Otherwise, generate a
	// new address.
	if b.batchAddress != nil {
		address = b.batchAddress
	} else {
		var err error

		// Generate a wallet address for the batch transaction's output.
		address, err = b.wallet.NextAddr(
			ctx, "", walletrpc.AddressType_TAPROOT_PUBKEY, false,
		)
		if err != nil {
			return address, err
		}

		// Save that new address in order to re-use in future
		// versions of the batch tx.
		b.batchAddress = address
	}

	return address, nil
}

func (b *batch) insertAndAcquireID(ctx context.Context) (int32, error) {
	bch := &Batch{}
	bch.State = stateEnumToString(b.state)
	bch.MaxTimeoutDistance = b.Cfg.MaxTimeoutDistance

	id, err := b.store.InsertSweepBatch(ctx, bch)
	if err != nil {
		return 0, err
	}

	b.id = id
	b.log = batchPrefixLogger(fmt.Sprintf("%d", b.id))

	return id, nil
}

func (b *batch) writeToErrChan(ctx context.Context, err error) {
	select {
	case b.errChan <- err:
	case <-ctx.Done():
	}
}

func (b *batch) persistSweep(ctx context.Context, sweep sweep,
	completed bool) error {

	return b.store.UpsertSweep(ctx, &Sweep{
		BatchID:   b.id,
		SwapHash:  sweep.swapHash,
		Outpoint:  sweep.outpoint,
		Amount:    sweep.value,
		Completed: completed,
	})
}

// clampBatchFee takes the fee amount and total amount of the sweeps in the
// batch and makes sure the fee is not too high. If the fee is too high, it is
// clamped to the maximum allowed fee.
func clampBatchFee(fee btcutil.Amount,
	totalAmount btcutil.Amount) btcutil.Amount {

	maxFeeAmount := btcutil.Amount(float64(totalAmount) *
		maxFeeToSwapAmtRatio)

	if fee > maxFeeAmount {
		return maxFeeAmount
	}

	return fee
}

func stateEnumToString(state batchState) string {
	switch state {
	case Open:
		return BatchOpen

	case Closed:
		return BatchClosed

	case Confirmed:
		return BatchConfirmed
	}

	return ""
}
