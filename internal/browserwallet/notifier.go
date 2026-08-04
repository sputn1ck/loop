package browserwallet

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/lndclient"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/lnrpc/chainrpc"
)

// ChainNotifier implements Loop's chain-notifier dependency by polling
// Esplora. Each registration owns a bounded polling goroutine tied to the
// caller's context, so reloading the embedded daemon cancels all old watches.
type ChainNotifier struct {
	lndclient.ChainNotifierClient

	esplora      *EsploraClient
	pollInterval time.Duration
}

// NewChainNotifier creates an Esplora-backed chain notifier.
func NewChainNotifier(esplora *EsploraClient,
	pollInterval time.Duration) (*ChainNotifier, error) {

	if esplora == nil {
		return nil, errors.New("Esplora client is required")
	}
	if pollInterval <= 0 {
		return nil, errors.New("poll interval must be positive")
	}

	return &ChainNotifier{
		esplora:      esplora,
		pollInterval: pollInterval,
	}, nil
}

// RawClientWithMacAuth satisfies lndclient's service wrapper. There is no raw
// RPC client because this notifier runs in process.
func (n *ChainNotifier) RawClientWithMacAuth(ctx context.Context) (
	context.Context, time.Duration, chainrpc.ChainNotifierClient) {

	return ctx, 0, nil
}

// RegisterBlockEpochNtfn emits the current height immediately, then emits each
// observed height change. Same-height reorganizations are handled by the
// confirmation registrations, whose block hash tracking carries more detail
// than the height-only LND interface.
func (n *ChainNotifier) RegisterBlockEpochNtfn(ctx context.Context) (
	chan int32, chan error, error) {

	heights := make(chan int32, 1)
	errs := make(chan error, 1)

	go func() {
		var lastHeight int32 = -1
		ticker := time.NewTicker(n.pollInterval)
		defer ticker.Stop()

		for {
			height, err := n.esplora.TipHeight(ctx)
			if err != nil {
				if !sendNotifierError(ctx, errs, err) {
					return
				}
			} else if height != lastHeight {
				select {
				case heights <- height:
					lastHeight = height

				case <-ctx.Done():
					return
				}
			}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()

	return heights, errs, nil
}

// RegisterConfirmationsNtfn watches a txid, or discovers it by output script,
// until the requested confirmation depth is reached. WithReOrgChan keeps the
// watcher active and reports a later disconnect.
func (n *ChainNotifier) RegisterConfirmationsNtfn(ctx context.Context,
	txid *chainhash.Hash, pkScript []byte, numConfs, _ int32,
	optFuncs ...lndclient.NotifierOption) (
	chan *chainntnfs.TxConfirmation, chan error, error) {

	if txid == nil && len(pkScript) == 0 {
		return nil, nil, errors.New("txid or output script is required")
	}
	if numConfs <= 0 {
		return nil, nil, errors.New("confirmation count must be positive")
	}

	opts := lndclient.DefaultNotifierOptions()
	for _, option := range optFuncs {
		option(opts)
	}
	if opts.IncludeBlock {
		return nil, nil, errors.New("including the full block is unsupported")
	}

	confirmations := make(chan *chainntnfs.TxConfirmation, 1)
	errs := make(chan error, 1)

	go n.watchConfirmations(
		ctx, txid, pkScript, numConfs, opts, confirmations, errs,
	)

	return confirmations, errs, nil
}

func (n *ChainNotifier) watchConfirmations(ctx context.Context,
	txid *chainhash.Hash, pkScript []byte, numConfs int32,
	opts *lndclient.NotifierOptions,
	confirmations chan *chainntnfs.TxConfirmation, errs chan error) {

	var (
		explicitTxID   = txid != nil
		watchedTx      = txid
		delivered      bool
		deliveredBlock string
	)
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	for {
		lookupTx := watchedTx
		if !explicitTxID {
			lookupTx = nil
		}

		status, err := n.confirmationStatus(ctx, lookupTx, pkScript)
		if err != nil {
			if !sendNotifierError(ctx, errs, err) {
				return
			}
		} else if status == nil {
			if delivered {
				if opts.ReOrgChan != nil {
					select {
					case opts.ReOrgChan <- struct{}{}:
					case <-ctx.Done():
						return
					}
				}

				delivered = false
				deliveredBlock = ""
			}

			if !explicitTxID {
				watchedTx = nil
			}
		} else if status != nil {
			txChanged := watchedTx != nil &&
				*status.txid != *watchedTx
			watchedTx = status.txid
			isConfirmed := status.status.Confirmed
			blockChanged := deliveredBlock != "" &&
				status.status.BlockHash != deliveredBlock

			if delivered && (!isConfirmed || txChanged || blockChanged) {
				if opts.ReOrgChan != nil {
					select {
					case opts.ReOrgChan <- struct{}{}:
					case <-ctx.Done():
						return
					}
				}
				delivered = false
				deliveredBlock = ""
			}

			if isConfirmed && !delivered {
				tipHeight, err := n.esplora.TipHeight(ctx)
				if err != nil {
					if !sendNotifierError(ctx, errs, err) {
						return
					}
				} else if tipHeight-status.status.BlockHeight+1 >=
					numConfs {

					confirmation, err := n.txConfirmation(
						ctx, *watchedTx, status.status,
					)
					if err != nil {
						if !sendNotifierError(ctx, errs, err) {
							return
						}
					} else {
						select {
						case confirmations <- confirmation:
							delivered = true
							deliveredBlock =
								status.status.BlockHash

						case <-ctx.Done():
							return
						}

						if opts.ReOrgChan == nil {
							return
						}
					}
				}
			}
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

type confirmationStatus struct {
	txid   *chainhash.Hash
	status *TxStatus
}

func (n *ChainNotifier) confirmationStatus(ctx context.Context,
	txid *chainhash.Hash, pkScript []byte) (*confirmationStatus, error) {

	if txid == nil {
		foundTxID, status, err := n.esplora.FindTransactionByScript(
			ctx, pkScript,
		)
		if err != nil || foundTxID == nil {
			return nil, err
		}

		return &confirmationStatus{
			txid:   foundTxID,
			status: status,
		}, nil
	}

	status, err := n.esplora.TransactionStatus(ctx, *txid)
	if err != nil {
		return nil, err
	}

	return &confirmationStatus{
		txid:   txid,
		status: status,
	}, nil
}

func (n *ChainNotifier) txConfirmation(ctx context.Context,
	txid chainhash.Hash, status *TxStatus) (*chainntnfs.TxConfirmation,
	error) {

	tx, err := n.esplora.RawTransaction(ctx, txid)
	if err != nil {
		return nil, err
	}

	blockHash, err := chainhash.NewHashFromStr(status.BlockHash)
	if err != nil {
		return nil, fmt.Errorf("parse confirmation block hash: %w", err)
	}

	return &chainntnfs.TxConfirmation{
		BlockHash:   blockHash,
		BlockHeight: uint32(status.BlockHeight),
		Tx:          tx,
	}, nil
}

// RegisterSpendNtfn watches an explicit outpoint until Esplora reports a
// confirmed spending transaction. WithReOrgChan keeps the watcher active and
// reports when the spender is disconnected or replaced.
func (n *ChainNotifier) RegisterSpendNtfn(ctx context.Context,
	outpoint *wire.OutPoint, _ []byte, _ int32,
	optFuncs ...lndclient.NotifierOption) (
	chan *chainntnfs.SpendDetail, chan error, error) {

	if outpoint == nil {
		return nil, nil, errors.New("outpoint is required")
	}

	opts := lndclient.DefaultNotifierOptions()
	for _, option := range optFuncs {
		option(opts)
	}
	if opts.IncludeBlock {
		return nil, nil, errors.New(
			"including the full block is unsupported",
		)
	}

	spends := make(chan *chainntnfs.SpendDetail, 1)
	errs := make(chan error, 1)

	go n.watchSpend(ctx, outpoint, opts, spends, errs)

	return spends, errs, nil
}

func (n *ChainNotifier) watchSpend(ctx context.Context,
	outpoint *wire.OutPoint, opts *lndclient.NotifierOptions,
	spends chan *chainntnfs.SpendDetail, errs chan error) {

	var (
		delivered      bool
		deliveredTxID  string
		deliveredBlock string
	)
	ticker := time.NewTicker(n.pollInterval)
	defer ticker.Stop()

	for {
		outspend, err := n.esplora.OutputSpend(ctx, *outpoint)
		if err != nil {
			if !sendNotifierError(ctx, errs, err) {
				return
			}
		} else {
			confirmed := outspend.Spent && outspend.Status != nil &&
				outspend.Status.Confirmed
			spendChanged := confirmed &&
				(outspend.TxID != deliveredTxID ||
					outspend.Status.BlockHash != deliveredBlock)

			if delivered && (!confirmed || spendChanged) {
				select {
				case opts.ReOrgChan <- struct{}{}:
				case <-ctx.Done():
					return
				}

				delivered = false
				deliveredTxID = ""
				deliveredBlock = ""
			}

			if confirmed && !delivered {
				txid, err := chainhash.NewHashFromStr(outspend.TxID)
				if err != nil {
					if !sendNotifierError(ctx, errs, err) {
						return
					}
				} else {
					tx, err := n.esplora.RawTransaction(ctx, *txid)
					if err != nil {
						if !sendNotifierError(ctx, errs, err) {
							return
						}
					} else {
						detail := &chainntnfs.SpendDetail{
							SpentOutPoint:     outpoint,
							SpenderTxHash:     txid,
							SpendingTx:        tx,
							SpenderInputIndex: outspend.Vin,
							SpendingHeight: outspend.Status.
								BlockHeight,
						}
						select {
						case spends <- detail:
							delivered = true
							deliveredTxID = outspend.TxID
							deliveredBlock = outspend.Status.
								BlockHash

						case <-ctx.Done():
							return
						}

						if opts.ReOrgChan == nil {
							return
						}
					}
				}
			}
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

func sendNotifierError(ctx context.Context, errs chan<- error,
	err error) bool {

	select {
	case errs <- err:
		return true

	case <-ctx.Done():
		return false
	}
}
