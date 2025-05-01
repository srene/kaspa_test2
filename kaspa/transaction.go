package kaspa

import (
	"fmt"

	"github.com/kaspanet/kaspad/cmd/kaspawallet/libkaspawallet"
	"github.com/kaspanet/kaspad/cmd/kaspawallet/libkaspawallet/bip32"
	"github.com/kaspanet/kaspad/cmd/kaspawallet/libkaspawallet/serialization"
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/utils/consensushashing"
	"github.com/kaspanet/kaspad/domain/consensus/utils/constants"
	"github.com/kaspanet/kaspad/domain/consensus/utils/subnetworks"
	"github.com/kaspanet/kaspad/domain/consensus/utils/txscript"
	"github.com/kaspanet/kaspad/domain/consensus/utils/utxo"
	"github.com/kaspanet/kaspad/domain/miningmanager/mempool"
	"github.com/kaspanet/kaspad/util"
	"github.com/pkg/errors"
)

func (c *Client) createUnsignedTransactions(address string, blob []byte) ([][]byte, error) {

	/*amount, err := utils.KasToSompi("1")
	if err != nil {
		return nil, err
	}*/
	feeRate, maxFee, err := c.calculateFeeLimits()
	if err != nil {
		return nil, err
	}

	// make sure address string is correct before proceeding to a
	// potentially long UTXO refreshment operation
	toAddress, err := util.DecodeAddress(address, c.params.Prefix)
	if err != nil {
		return nil, err
	}

	var fromAddresses []*walletAddress

	changeAddress, err := util.DecodeAddress(fromAddress, c.params.Prefix)
	if err != nil {
		return nil, err
	}

	changeWalletAddress := &walletAddress{
		index:         1,
		cosignerIndex: 0,
		keyChain:      libkaspawallet.ExternalKeychain,
	}
	/*changeAddress, changeWalletAddress, err := s.changeAddress(true, fromAddresses)
	if err != nil {
		return nil, err
	}*/

	selectedUTXOs, spendValue, changeSompi, err := c.selectUTXOs(feeRate, maxFee, fromAddresses, blob)
	if err != nil {
		return nil, err
	}

	if len(selectedUTXOs) == 0 {
		return nil, fmt.Errorf("couldn't find funds to spend")
	}

	payments := []*libkaspawallet.Payment{{
		Address: toAddress,
		Amount:  spendValue,
	}}
	if changeSompi > 0 {
		payments = append(payments, &libkaspawallet.Payment{
			Address: changeAddress,
			Amount:  changeSompi,
		})
	}
	/*unsignedTransaction, err := createUnsignedTransaction(s.keysFile.ExtendedPublicKeys,
	payments, selectedUTXOs, blob)*/
	publickey, err := c.extendedKey.Public()
	if err != nil {
		return nil, err
	}
	unsignedTransaction, err := createUnsignedTransaction(publickey.String(), payments, selectedUTXOs, blob)
	if err != nil {
		return nil, err
	}

	unsignedTransactions, err := c.maybeAutoCompoundTransaction(unsignedTransaction, toAddress, changeAddress, changeWalletAddress, feeRate, maxFee, blob)
	if err != nil {
		return nil, err
	}

	return unsignedTransactions, nil
}

/*
func createUnsignedTransaction(
extendedPublicKeys []string,
minimumSignatures uint32,
payments []*libkaspawallet.Payment,
selectedUTXOs []*libkaspawallet.UTXO,
blob []byte) (*serialization.PartiallySignedTransaction, error) {
*/
func createUnsignedTransaction(
	extendedPublicKey string,
	payments []*libkaspawallet.Payment,
	selectedUTXOs []*libkaspawallet.UTXO,
	blob []byte) (*serialization.PartiallySignedTransaction, error) {

	inputs := make([]*externalapi.DomainTransactionInput, len(selectedUTXOs))
	partiallySignedInputs := make([]*serialization.PartiallySignedInput, len(selectedUTXOs))

	for i, utxo := range selectedUTXOs {
		//emptyPubKeySignaturePairs := make([]*serialization.PubKeySignaturePair, len(extendedPublicKeys))
		//for i, extendedPublicKey := range extendedPublicKeys {
		extendedKey, err := bip32.DeserializeExtendedKey(extendedPublicKey)
		if err != nil {
			return nil, err
		}

		derivedKey, err := extendedKey.DeriveFromPath(utxo.DerivationPath)
		if err != nil {
			return nil, err
		}
		emptyPubKeySignaturePair := []*serialization.PubKeySignaturePair{
			&serialization.PubKeySignaturePair{
				ExtendedPublicKey: derivedKey.String()},
		}
		/*emptyPubKeySignaturePairs[i] = &serialization.PubKeySignaturePair{
			ExtendedPublicKey: derivedKey.String(),
		}*/
		//}

		inputs[i] = &externalapi.DomainTransactionInput{PreviousOutpoint: *utxo.Outpoint}
		/*partiallySignedInputs[i] = &serialization.PartiallySignedInput{
			PrevOutput: &externalapi.DomainTransactionOutput{
				Value:           utxo.UTXOEntry.Amount(),
				ScriptPublicKey: utxo.UTXOEntry.ScriptPublicKey(),
			},
			MinimumSignatures:    minimumSignatures,
			PubKeySignaturePairs: emptyPubKeySignaturePairs,
			DerivationPath:       utxo.DerivationPath,
		}*/
		partiallySignedInputs[i] = &serialization.PartiallySignedInput{
			PrevOutput: &externalapi.DomainTransactionOutput{
				Value:           utxo.UTXOEntry.Amount(),
				ScriptPublicKey: utxo.UTXOEntry.ScriptPublicKey(),
			},
			MinimumSignatures:    1,
			PubKeySignaturePairs: emptyPubKeySignaturePair,
			DerivationPath:       utxo.DerivationPath,
		}
	}

	outputs := make([]*externalapi.DomainTransactionOutput, len(payments))
	for i, payment := range payments {
		scriptPublicKey, err := txscript.PayToAddrScript(payment.Address)
		if err != nil {
			return nil, err
		}

		outputs[i] = &externalapi.DomainTransactionOutput{
			Value:           payment.Amount,
			ScriptPublicKey: scriptPublicKey,
		}
	}

	domainTransaction := &externalapi.DomainTransaction{
		Version:      constants.MaxTransactionVersion,
		Inputs:       inputs,
		Outputs:      outputs,
		LockTime:     0,
		SubnetworkID: subnetworks.SubnetworkIDNative,
		Gas:          0,
		Payload:      blob,
	}

	return &serialization.PartiallySignedTransaction{
		Tx:                    domainTransaction,
		PartiallySignedInputs: partiallySignedInputs,
	}, nil

}

// maybeAutoCompoundTransaction checks if a transaction's mass is higher that what is allowed for a standard
// transaction.
// If it is - the transaction is split into multiple transactions, each with a portion of the inputs and a single output
// into a change address.
// An additional `mergeTransaction` is generated - which merges the outputs of the above splits into a single output
// paying to the original transaction's payee.
func (s *Client) maybeAutoCompoundTransaction(transaction *serialization.PartiallySignedTransaction, toAddress util.Address,
	changeAddress util.Address, changeWalletAddress *walletAddress, feeRate float64, maxFee uint64, blob []byte) ([][]byte, error) {

	splitTransactions, err := s.maybeSplitAndMergeTransaction(transaction, toAddress, changeAddress, changeWalletAddress, feeRate, maxFee, blob)
	if err != nil {
		return nil, err
	}
	splitTransactionsBytes := make([][]byte, len(splitTransactions))
	for i, splitTransaction := range splitTransactions {
		splitTransactionsBytes[i], err = serialization.SerializePartiallySignedTransaction(splitTransaction)
		if err != nil {
			return nil, err
		}
	}
	return splitTransactionsBytes, nil
}

func (s *Client) maybeSplitAndMergeTransaction(transaction *serialization.PartiallySignedTransaction, address util.Address,
	changeAddress util.Address, changeWalletAddress *walletAddress, feeRate float64, maxFee uint64, blob []byte) ([]*serialization.PartiallySignedTransaction, error) {

	mockTx := transaction.Clone()
	mockTx.Tx.Payload = nil
	transactionMass, err := s.estimateComputeMassAfterSignatures(mockTx)
	if err != nil {
		return nil, err
	}
	transientMass, err := s.estimateTransientMassAfterSignatures(transaction)
	if err != nil {
		return nil, err
	}
	if transientMass < mempool.MaximumStandardTransactionMass {
		return []*serialization.PartiallySignedTransaction{transaction}, nil
	} else {
		/*	panic("transaction mass to high")
				}
			}

			splitCount, inputCountPerSplit, err := s.splitAndInputPerSplitCounts(transaction, transactionMass, transientMass, changeAddress, feeRate, maxFee, blob)
			if err != nil {
				return nil, err
			}*/
		maxChunkSize := (int)((mempool.MaximumStandardTransactionMass - transactionMass) / TRANSIENT_BYTE_TO_MASS_FACTOR)
		splitCount := (int)(len(blob) / maxChunkSize)
		fmt.Println(maxChunkSize, splitCount, len(blob)%splitCount)

		if len(blob)%maxChunkSize > 0 {
			splitCount++
		}
		splitTransactions := make([]*serialization.PartiallySignedTransaction, splitCount)

		for i := 0; i < splitCount; i++ {

			totalSompi := uint64(0)
			startChunkIndex := i * maxChunkSize
			endChunkIndex := startChunkIndex + maxChunkSize
			if endChunkIndex > len(blob)-1 {
				endChunkIndex = len(blob)
			}
			var selectedUTXOs []*libkaspawallet.UTXO
			if i == 0 {
				partiallySignedInput := transaction.PartiallySignedInputs[0]
				selectedUTXOs = append(selectedUTXOs, &libkaspawallet.UTXO{
					Outpoint: &transaction.Tx.Inputs[0].PreviousOutpoint,
					UTXOEntry: utxo.NewUTXOEntry(
						partiallySignedInput.PrevOutput.Value, partiallySignedInput.PrevOutput.ScriptPublicKey,
						false, constants.UnacceptedDAAScore),
					DerivationPath: partiallySignedInput.DerivationPath,
				})
			} else {
				output := splitTransactions[i-1].Tx.Outputs[0]
				selectedUTXOs = append(selectedUTXOs, &libkaspawallet.UTXO{
					Outpoint: &externalapi.DomainOutpoint{
						TransactionID: *consensushashing.TransactionID(splitTransactions[i-1].Tx),
						Index:         0,
					},
					UTXOEntry:      utxo.NewUTXOEntry(output.Value, output.ScriptPublicKey, false, constants.UnacceptedDAAScore),
					DerivationPath: s.walletAddressPath(changeWalletAddress),
				})
			}

			totalSompi += selectedUTXOs[0].UTXOEntry.Amount()
			fee, err := s.estimateFee(selectedUTXOs, feeRate, maxFee, totalSompi, blob[startChunkIndex:endChunkIndex])
			if err != nil {
				return nil, err
			}

			totalSompi -= fee
			publickey, err := s.extendedKey.Public()
			if err != nil {
				return nil, err
			}
			payload := blob[startChunkIndex:endChunkIndex]
			splitTransactions[i], err = createUnsignedTransaction(publickey.String(),
				[]*libkaspawallet.Payment{{
					Address: changeAddress,
					Amount:  totalSompi,
				}}, selectedUTXOs, payload)

		}

		/*if len(splitTransactions) > 1 {
			mergeTransaction, err := s.mergeTransaction(splitTransactions, transaction, address, changeAddress, changeWalletAddress, feeRate, maxFee, blob)
			if err != nil {
				return nil, err
			}
			// Recursion will be 2-3 iterations deep even in the rarest` cases, so considered safe..
			/*splitMergeTransaction, err := s.maybeSplitAndMergeTransaction(mergeTransaction, address, changeAddress, changeWalletAddress, feeRate, maxFee, blob)
			if err != nil {
				return nil, err
			}
			splitTransactions = append(splitTransactions, splitMergeTransaction...)
			//splitTransactions = append(splitTransactions, mergeTransaction)
		}*/

		return splitTransactions, nil
	}
}

// splitAndInputPerSplitCounts calculates the number of splits to create, and the number of inputs to assign per split.
func (s *Client) splitAndInputPerSplitCounts(transaction *serialization.PartiallySignedTransaction, transactionMass uint64, transientMass uint64,
	changeAddress util.Address, feeRate float64, maxFee uint64, blob []byte) (splitCount, inputsPerSplitCount int, err error) {

	// Create a dummy transaction which is a clone of the original transaction, but without inputs,
	// to calculate how much mass do all the inputs have
	transactionWithoutInputs := transaction.Tx.Clone()
	transactionWithoutInputs.Inputs = []*externalapi.DomainTransactionInput{}
	massWithoutInputs := s.txMassCalculator.CalculateTransactionMass(transactionWithoutInputs)

	massOfAllInputs := transactionMass - massWithoutInputs

	// Since the transaction was generated by kaspawallet, we assume all inputs have the same number of signatures, and
	// thus - the same mass.
	inputCount := len(transaction.Tx.Inputs)
	massPerInput := massOfAllInputs / uint64(inputCount)
	if massOfAllInputs%uint64(inputCount) > 0 {
		massPerInput++
	}

	// Create another dummy transaction, this time one similar to the split transactions we wish to generate,
	// but with 0 inputs, to calculate how much mass for inputs do we have available in the split transactions
	splitTransactionWithoutInputs, err := s.createSplitTransaction(transaction, changeAddress, 0, 0, feeRate, maxFee, blob)
	if err != nil {
		return 0, 0, err
	}
	massForEverythingExceptInputsInSplitTransaction := s.txMassCalculator.CalculateTransactionMass(splitTransactionWithoutInputs.Tx)
	massForInputsInSplitTransaction := mempool.MaximumStandardTransactionMass - massForEverythingExceptInputsInSplitTransaction

	inputsPerSplitCount = int(massForInputsInSplitTransaction / massPerInput)
	splitCount = inputCount / inputsPerSplitCount
	if inputCount%inputsPerSplitCount > 0 {
		splitCount++
	}

	return splitCount, inputsPerSplitCount, nil
}

func (c *Client) createSplitTransaction(transaction *serialization.PartiallySignedTransaction,
	changeAddress util.Address, startIndex int, endIndex int, feeRate float64, maxFee uint64, blob []byte) (*serialization.PartiallySignedTransaction, error) {

	selectedUTXOs := make([]*libkaspawallet.UTXO, 0, endIndex-startIndex)
	totalSompi := uint64(0)

	for i := startIndex; i < endIndex && i < len(transaction.PartiallySignedInputs); i++ {
		partiallySignedInput := transaction.PartiallySignedInputs[i]
		selectedUTXOs = append(selectedUTXOs, &libkaspawallet.UTXO{
			Outpoint: &transaction.Tx.Inputs[i].PreviousOutpoint,
			UTXOEntry: utxo.NewUTXOEntry(
				partiallySignedInput.PrevOutput.Value, partiallySignedInput.PrevOutput.ScriptPublicKey,
				false, constants.UnacceptedDAAScore),
			DerivationPath: partiallySignedInput.DerivationPath,
		})

		totalSompi += selectedUTXOs[i-startIndex].UTXOEntry.Amount()
	}
	if len(selectedUTXOs) != 0 {
		fee, err := c.estimateFee(selectedUTXOs, feeRate, maxFee, totalSompi, blob)
		if err != nil {
			return nil, err
		}

		totalSompi -= fee
	}
	publickey, err := c.extendedKey.Public()
	if err != nil {
		return nil, err
	}
	return createUnsignedTransaction(publickey.String(),
		[]*libkaspawallet.Payment{{
			Address: changeAddress,
			Amount:  totalSompi,
		}}, selectedUTXOs, blob)

}

func (s *Client) mergeTransaction(
	splitTransactions []*serialization.PartiallySignedTransaction,
	originalTransaction *serialization.PartiallySignedTransaction,
	toAddress util.Address,
	changeAddress util.Address,
	changeWalletAddress *walletAddress,
	feeRate float64,
	maxFee uint64,
	blob []byte,
) (*serialization.PartiallySignedTransaction, error) {
	numOutputs := len(originalTransaction.Tx.Outputs)
	if numOutputs > 2 || numOutputs == 0 {
		// This is a sanity check to make sure originalTransaction has either 1 or 2 outputs:
		// 1. For the payment itself
		// 2. (optional) for change
		return nil, errors.Errorf("original transaction has %d outputs, while 1 or 2 are expected",
			len(originalTransaction.Tx.Outputs))
	}

	totalValue := uint64(0)
	sentValue := originalTransaction.Tx.Outputs[0].Value
	utxos := make([]*libkaspawallet.UTXO, len(splitTransactions))
	for i, splitTransaction := range splitTransactions {
		output := splitTransaction.Tx.Outputs[0]
		utxos[i] = &libkaspawallet.UTXO{
			Outpoint: &externalapi.DomainOutpoint{
				TransactionID: *consensushashing.TransactionID(splitTransaction.Tx),
				Index:         0,
			},
			UTXOEntry:      utxo.NewUTXOEntry(output.Value, output.ScriptPublicKey, false, constants.UnacceptedDAAScore),
			DerivationPath: s.walletAddressPath(changeWalletAddress),
		}
		totalValue += output.Value
	}
	// We're overestimating a bit by assuming that any transaction will have a change output
	fee, err := s.estimateFee(utxos, feeRate, maxFee, sentValue, blob)
	if err != nil {
		return nil, err
	}

	totalValue -= fee

	if totalValue < sentValue {
		// sometimes the fees from compound transactions make the total output higher than what's available from selected
		// utxos, in such cases - find one more UTXO and use it.
		additionalUTXOs, totalValueAdded, err := s.moreUTXOsForMergeTransaction(utxos, sentValue-totalValue, feeRate)
		if err != nil {
			return nil, err
		}
		utxos = append(utxos, additionalUTXOs...)
		totalValue += totalValueAdded
	}

	payments := []*libkaspawallet.Payment{{
		Address: toAddress,
		Amount:  sentValue,
	}}
	if totalValue > sentValue {
		payments = append(payments, &libkaspawallet.Payment{
			Address: changeAddress,
			Amount:  totalValue - sentValue,
		})
	}
	publickey, err := s.extendedKey.Public()
	if err != nil {
		return nil, err
	}
	return createUnsignedTransaction(publickey.String(), payments, utxos, blob)

	/*return s.CreateUnsignedTransaction(s.keysFile.ExtendedPublicKeys,
	s.keysFile.MinimumSignatures, payments, utxos)*/
}
