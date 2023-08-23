package sweepbatcher

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/lightninglabs/loop/loopdb"
	"github.com/lightninglabs/loop/utils"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
)

const (
	// DefaultMaxTimeoutDistance is the default maximum timeout distance
	// of sweeps that can appear in the same batch.
	DefaultMaxTimeoutDistance = 288

	// BatchOpen is the string representation of the state of a batch that
	// is open.
	BatchOpen = "open"

	// BatchClosed is the string representation of the state of a batch
	// that is closed.
	BatchClosed = "closed"

	// BatchConfirmed is the string representation of the state of a batch
	// that is confirmed.
	BatchConfirmed = "confirmed"

	// batcherWaitDelay is the amount of time the batcher waits after
	// resuming in-flight batches.
	batcherWaitDelay = 1 * time.Second
)

type BatcherStore interface {
	// FetchUnconfirmedSweepBatches fetches all the batches from the
	// database that are not in a confirmed state.
	FetchUnconfirmedSweepBatches(ctx context.Context) ([]*loopdb.Batch,
		error)

	// InsertSweepBatch inserts a batch into the database, returning the id
	// of the inserted batch.
	InsertSweepBatch(ctx context.Context,
		batch *loopdb.Batch) (int32, error)

	// UpdateSweepBatch updates a batch in the database.
	UpdateSweepBatch(ctx context.Context,
		batch *loopdb.Batch) error

	// ConfirmBatch confirms a batch by setting its state to confirmed.
	ConfirmBatch(ctx context.Context, id int32) error

	// FetchBatchSweeps fetches all the sweeps that belong to a batch.
	FetchBatchSweeps(ctx context.Context,
		id int32) ([]*loopdb.Sweep, error)

	// UpsertSweep inserts a sweep into the database, or updates an existing
	// sweep if it already exists.
	UpsertSweep(ctx context.Context, sweep *loopdb.Sweep) error

	// GetSweepStatus returns the completed status of the sweep.
	GetSweepStatus(ctx context.Context, swapHash lntypes.Hash) (
		bool, error)

	// FetchLoopOutSwap fetches a loop out swap from the database.
	FetchLoopOutSwap(ctx context.Context,
		hash lntypes.Hash) (*loopdb.LoopOut, error)
}

// MuSig2SignSweep is a function that can be used to sign a sweep transaction
// cooperatively with the swap server.
type MuSig2SignSweep func(ctx context.Context,
	protocolVersion loopdb.ProtocolVersion, swapHash lntypes.Hash,
	paymentAddr [32]byte, nonce []byte, sweepTxPsbt []byte,
	prevoutMap map[wire.OutPoint]*wire.TxOut) (
	[]byte, []byte, error)

// VerifySchnorrSig is a function that can be used to verify a schnorr
// signature.
type VerifySchnorrSig func(pubKey *btcec.PublicKey, hash, sig []byte) error

// SweepRequest is a request to sweep a specific outpoint.
type SweepRequest struct {
	// SwapHash is the hash of the swap that is being swept.
	SwapHash lntypes.Hash

	// Outpoint is the outpoint that is being swept.
	Outpoint wire.OutPoint

	// Value is the value of the outpoint that is being swept.
	Value btcutil.Amount

	// Notifier is a notifier that is used to notify the requester of this
	// sweep that the sweep was successful.
	Notifier SpendNotifier
}

// SpendNotifier is a notifier that is used to notify the requester of a sweep
// that the sweep was successful.
type SpendNotifier struct {
	// SpendChan is a channel where the spend details are received.
	SpendChan chan *wire.MsgTx

	// SpendErrChan is a channel where spend errors are received.
	SpendErrChan chan error

	// QuitChan is a channel that can be closed to stop the notifier.
	QuitChan chan bool
}

// Batcher is a system that is responsible for accepting sweep requests and
// placing them in appropriate batches. It will spin up new batches as needed.
type Batcher struct {
	// batches is a map of batch IDs to the currently active batches.
	batches map[int32]*batch

	// sweepReqs is a channel where sweep requests are received.
	sweepReqs chan SweepRequest

	// errChan is a channel where errors are received.
	errChan chan error

	// completeChan is a channel where batches signal their completion by
	// providing their ID.
	completeChan chan int32

	// wallet is the wallet kit client that is used by batches.
	wallet lndclient.WalletKitClient

	// chainNotifier is the chain notifier client that is used by batches.
	chainNotifier lndclient.ChainNotifierClient

	// signerClient is the signer client that is used by batches.
	signerClient lndclient.SignerClient

	// musig2ServerKit includes all the required functionality to collect
	// and verify signatures by the swap server in order to cooperatively
	// sweep funds.
	musig2ServerSign MuSig2SignSweep

	// verifySchnorrSig is a function that can be used to verify a schnorr
	// signature.
	VerifySchnorrSig VerifySchnorrSig

	// chainParams are the chain parameters of the chain that is used by
	// batches.
	chainParams *chaincfg.Params

	// store includes all the database interactions that are needed by the
	// batcher and the batches.
	store BatcherStore

	// wg is a waitgroup that is used to wait for all the goroutines to
	// exit.
	wg sync.WaitGroup
}

// NewBatcher creates a new Batcher instance.
func NewBatcher(wallet lndclient.WalletKitClient,
	chainNotifier lndclient.ChainNotifierClient,
	signerClient lndclient.SignerClient, musig2ServerSigner MuSig2SignSweep,
	verifySchnorrSig VerifySchnorrSig, chainparams *chaincfg.Params,
	store BatcherStore) *Batcher {

	return &Batcher{
		batches:          make(map[int32]*batch),
		sweepReqs:        make(chan SweepRequest),
		errChan:          make(chan error),
		completeChan:     make(chan int32),
		wallet:           wallet,
		chainNotifier:    chainNotifier,
		signerClient:     signerClient,
		musig2ServerSign: musig2ServerSigner,
		VerifySchnorrSig: verifySchnorrSig,
		chainParams:      chainparams,
		store:            store,
	}
}

// Run starts the batcher and processes incoming sweep requests.
func (b *Batcher) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()

		for _, batch := range b.batches {
			batch.Wait()
		}

		b.wg.Wait()
	}()

	// First we fetch all the batches that are not in a confirmed state from
	// the database. We will then resume the execution of these batches.
	batches, err := b.FetchUnconfirmedBatches(runCtx)
	if err != nil {
		return err
	}

	var timerChan <-chan time.Time
	timerChan = time.After(batcherWaitDelay)

	for _, batch := range batches {
		err := b.spinUpBatchFromDB(runCtx, batch)
		if err != nil {
			return err
		}
		timerChan = time.After(batcherWaitDelay)
	}

	select {
	case <-timerChan:
	case <-runCtx.Done():
	}

	for {
		select {
		case sweepReq := <-b.sweepReqs:
			sweep, err := b.fetchSweep(runCtx, sweepReq)
			if err != nil {
				return err
			}

			err = b.handleSweep(runCtx, sweep, sweepReq.Notifier)
			if err != nil {
				return err
			}

		case id := <-b.completeChan:
			delete(b.batches, id)

		case err := <-b.errChan:
			return err

		case <-runCtx.Done():
			return runCtx.Err()
		}
	}
}

// AddSweep adds a sweep request to the batcher for handling. This will either
// place the sweep in an existing batch or create a new one.
func (b *Batcher) AddSweep(ctx context.Context, sweepReq SweepRequest) error {
	select {
	case b.sweepReqs <- sweepReq:
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

// handleSweep handles a sweep request by either placing it in an existing
// batch, or by spinning up a new batch for it.
func (b *Batcher) handleSweep(ctx context.Context, s *sweep,
	notifier SpendNotifier) error {

	completed, err := b.store.GetSweepStatus(ctx, s.swapHash)
	if err != nil {
		return err
	}

	log.Infof("Batcher handling sweep %x, completed=%v", s.swapHash[:6],
		completed)

	// If the sweep has already been completed in a confirmed batch, we will
	// retrieve the spending information directly.
	if completed && notifier != (SpendNotifier{}) {
		b.wg.Add(1)
		go b.monitorSpendAndNotify(ctx, s, notifier)
		return nil
	}

	sweep := s
	sweep.notifier = notifier

	// Check if the sweep is already in a batch. If that is the case, we
	// provide the sweep to that batch and return.
	for _, batch := range b.batches {
		if batch.sweepExists(sweep.swapHash) {
			return batch.WriteToBatchChan(ctx, sweep)
		}
	}

	// If one of the batches accepts the sweep, we provide it to that batch.
	for _, batch := range b.batches {
		if batch.AcceptsSweep(sweep) {
			return batch.WriteToBatchChan(ctx, sweep)
		}
	}

	// If no batch is capable of accepting the sweep, we spin up a fresh
	// batch and hand the sweep over to it.
	batch, err := b.spinUpBatch(ctx)
	if err != nil {
		return err
	}

	return batch.WriteToBatchChan(ctx, sweep)
}

// spinUpBatch spins up a new batch and returns it.
func (b *Batcher) spinUpBatch(ctx context.Context) (*batch, error) {
	cfg := batchConfig{
		MaxTimeoutDistance: DefaultMaxTimeoutDistance,
		BatchConfTarget:    defaultBatchConfTarget,
	}

	batchKit := batchKit{
		returnChan:       b.sweepReqs,
		completeChan:     b.completeChan,
		wallet:           b.wallet,
		chainNotifier:    b.chainNotifier,
		signerClient:     b.signerClient,
		musig2SignSweep:  b.musig2ServerSign,
		verifySchnorrSig: b.VerifySchnorrSig,
		store:            b.store,
	}

	batch := NewBatch(cfg, batchKit)

	id, err := batch.insertAndAcquireID(ctx)
	if err != nil {
		return nil, err
	}

	// We add the batch to our map of batches and start it.
	b.batches[id] = batch

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()

		err := batch.Run(ctx)
		if err != nil {
			_ = b.writeToErrChan(ctx, err)
		}
	}()

	return batch, nil
}

// spinUpBatchDB spins up a batch that already existed in storage, then
// returns it.
func (b *Batcher) spinUpBatchFromDB(ctx context.Context, batch *batch) error {
	cfg := batchConfig{
		MaxTimeoutDistance: batch.Cfg.MaxTimeoutDistance,
		BatchConfTarget:    defaultBatchConfTarget,
	}

	rbfCache := rbfCache{
		LastHeight: batch.rbfCache.LastHeight,
		FeeRate:    batch.rbfCache.FeeRate,
	}

	dbSweeps, err := b.store.FetchBatchSweeps(ctx, batch.id)
	if err != nil {
		return err
	}

	if len(dbSweeps) == 0 {
		return fmt.Errorf("batch %d has no sweeps", batch.id)
	}

	primarySweep := dbSweeps[0]

	sweeps := make(map[lntypes.Hash]sweep)

	for _, dbSweep := range dbSweeps {
		sweep, err := b.convertSweep(dbSweep)
		if err != nil {
			return err
		}

		sweeps[sweep.swapHash] = *sweep
	}

	batchKit := batchKit{
		id:               batch.id,
		batchTxid:        batch.batchTxid,
		batchPkScript:    batch.batchPkScript,
		state:            batch.state,
		primaryID:        primarySweep.SwapHash,
		sweeps:           sweeps,
		rbfCache:         rbfCache,
		returnChan:       b.sweepReqs,
		completeChan:     b.completeChan,
		wallet:           b.wallet,
		chainNotifier:    b.chainNotifier,
		signerClient:     b.signerClient,
		musig2SignSweep:  b.musig2ServerSign,
		verifySchnorrSig: b.VerifySchnorrSig,
		store:            b.store,
		log:              batchPrefixLogger(fmt.Sprintf("%d", batch.id)),
	}

	newBatch := NewBatchFromDB(cfg, batchKit)

	// We add the batch to our map of batches and start it.
	b.batches[batch.id] = newBatch

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()

		err := newBatch.Run(ctx)
		if err != nil {
			_ = b.writeToErrChan(ctx, err)
		}
	}()

	return nil
}

// FetchUnconfirmedBatches fetches all the batches from the database that are
// not in a confirmed state.
func (b *Batcher) FetchUnconfirmedBatches(ctx context.Context) ([]*batch,
	error) {

	dbBatches, err := b.store.FetchUnconfirmedSweepBatches(ctx)
	if err != nil {
		return nil, err
	}

	batches := make([]*batch, 0, len(dbBatches))
	for _, bch := range dbBatches {
		bch := bch

		batch := batch{}
		batch.id = bch.ID

		switch bch.State {
		case BatchOpen:
			batch.state = Open

		case BatchClosed:
			batch.state = Closed

		case BatchConfirmed:
			batch.state = Confirmed
		}

		batch.batchTxid = &bch.BatchTxid
		batch.batchPkScript = bch.BatchPkScript

		rbfCache := rbfCache{
			LastHeight: bch.LastRbfHeight,
			FeeRate:    chainfee.SatPerKWeight(bch.LastRbfSatPerKw),
		}
		batch.rbfCache = rbfCache

		bchCfg := batchConfig{
			MaxTimeoutDistance: bch.MaxTimeoutDistance,
		}
		batch.Cfg = &bchCfg

		batches = append(batches, &batch)
	}

	return batches, nil
}

// monitorSpendAndNotify monitors the spend of a specific outpoint and writes
// the response back to the response channel.
func (b *Batcher) monitorSpendAndNotify(ctx context.Context, sweep *sweep,
	notifier SpendNotifier) {

	defer b.wg.Done()

	spendCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	spendChan, spendErr, err := b.chainNotifier.RegisterSpendNtfn(
		spendCtx, &sweep.outpoint, sweep.htlc.PkScript,
		sweep.initiationHeight,
	)
	if err != nil {
		select {
		case notifier.SpendErrChan <- err:
		case <-ctx.Done():
		}

		_ = b.writeToErrChan(ctx, err)

		return
	}

	log.Infof("Batcher monitoring spend for swap %x", sweep.swapHash[:6])

	for {
		select {
		case spend := <-spendChan:
			select {
			case notifier.SpendChan <- spend.SpendingTx:
			case <-ctx.Done():
			}

			return

		case err := <-spendErr:
			select {
			case notifier.SpendErrChan <- err:
			case <-ctx.Done():
			}

			_ = b.writeToErrChan(ctx, err)
			return

		case <-notifier.QuitChan:
			return

		case <-ctx.Done():
			return
		}
	}
}

func (b *Batcher) writeToErrChan(ctx context.Context, err error) error {
	select {
	case b.errChan <- err:
		return nil

	case <-ctx.Done():
		return ctx.Err()
	}
}

// convertSweep converts a fetched sweep from the database to a sweep that is
// ready to be processed by the batcher.
func (b *Batcher) convertSweep(dbSweep *loopdb.Sweep) (*sweep, error) {
	swap := dbSweep.LoopOut

	htlc, err := utils.GetHtlc(
		dbSweep.SwapHash, &swap.Contract.SwapContract, b.chainParams,
	)
	if err != nil {
		return nil, err
	}

	swapPaymentAddr, err := utils.ObtainSwapPaymentAddr(
		swap.Contract.SwapInvoice, b.chainParams,
	)
	if err != nil {
		return nil, err
	}

	return &sweep{
		swapHash:               swap.Hash,
		outpoint:               dbSweep.Outpoint,
		value:                  dbSweep.Amount,
		confTarget:             swap.Contract.SweepConfTarget,
		timeout:                swap.Contract.CltvExpiry,
		initiationHeight:       swap.Contract.InitiationHeight,
		htlc:                   *htlc,
		preimage:               swap.Contract.Preimage,
		swapInvoicePaymentAddr: *swapPaymentAddr,
		htlcKeys:               swap.Contract.HtlcKeys,
		htlcSuccessEstimator:   htlc.AddSuccessToEstimator,
		protocolVersion:        swap.Contract.ProtocolVersion,
		isExternalAddr:         swap.Contract.IsExternalAddr,
		destAddr:               swap.Contract.DestAddr,
	}, nil
}

// fetchSweep fetches the sweep related information from the database.
func (b *Batcher) fetchSweep(ctx context.Context,
	sweepReq SweepRequest) (*sweep, error) {

	swapHash, err := lntypes.MakeHash(sweepReq.SwapHash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to parse swapHash: %v", err)
	}

	swap, err := b.store.FetchLoopOutSwap(ctx, swapHash)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch loop out for %x: %v",
			swapHash[:6], err)
	}

	htlc, err := utils.GetHtlc(
		swapHash, &swap.Contract.SwapContract, b.chainParams,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get htlc: %v", err)
	}

	swapPaymentAddr, err := utils.ObtainSwapPaymentAddr(
		swap.Contract.SwapInvoice, b.chainParams,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get payment addr: %v", err)
	}

	return &sweep{
		swapHash:               swap.Hash,
		outpoint:               sweepReq.Outpoint,
		value:                  sweepReq.Value,
		confTarget:             swap.Contract.SweepConfTarget,
		timeout:                swap.Contract.CltvExpiry,
		initiationHeight:       swap.Contract.InitiationHeight,
		htlc:                   *htlc,
		preimage:               swap.Contract.Preimage,
		swapInvoicePaymentAddr: *swapPaymentAddr,
		htlcKeys:               swap.Contract.HtlcKeys,
		htlcSuccessEstimator:   htlc.AddSuccessToEstimator,
		protocolVersion:        swap.Contract.ProtocolVersion,
		isExternalAddr:         swap.Contract.IsExternalAddr,
		destAddr:               swap.Contract.DestAddr,
	}, nil
}
