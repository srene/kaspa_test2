package kaspa

import (
	"math"

	"github.com/kaspanet/kaspad/cmd/kaspawallet/libkaspawallet"
	"github.com/kaspanet/kaspad/cmd/kaspawallet/libkaspawallet/serialization"
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/utils/constants"
	"github.com/kaspanet/kaspad/util"
	"github.com/kaspanet/kaspad/util/txmass"
)

/*func (s *Client) transactionFeeRate(psTx *serialization.PartiallySignedTransaction) (float64, error) {
	totalOuts := 0
	for _, output := range psTx.Tx.Outputs {
		totalOuts += int(output.Value)
	}

	totalIns := 0
	for _, input := range psTx.PartiallySignedInputs {
		totalIns += int(input.PrevOutput.Value)
	}

	if totalIns < totalOuts {
		return 0, errors.Errorf("Transaction don't have enough funds to pay for the outputs")
	}
	fee := totalIns - totalOuts
	mass, err := s.estimateComputeMassAfterSignatures(psTx)
	if err != nil {
		return 0, err
	}
	return float64(fee) / float64(mass), nil
}*/

func (c *Client) estimateMassAfterSignatures(transaction *serialization.PartiallySignedTransaction) (uint64, error) {
	//return EstimateMassAfterSignatures(transaction, s.keysFile.ECDSA, s.keysFile.MinimumSignatures, s.txMassCalculator)
	return EstimateMassAfterSignatures(transaction, false, 1, c.txMassCalculator)
}

func (c *Client) estimateTransientMassAfterSignatures(transaction *serialization.PartiallySignedTransaction) (uint64, error) {
	//return estimateComputeMassAfterSignatures(transaction, s.keysFile.ECDSA, s.keysFile.MinimumSignatures, s.txMassCalculator)
	return estimateTransientMass(transaction)
}

func (c *Client) estimateComputeMassAfterSignatures(transaction *serialization.PartiallySignedTransaction) (uint64, error) {
	//return estimateComputeMassAfterSignatures(transaction, s.keysFile.ECDSA, s.keysFile.MinimumSignatures, s.txMassCalculator)
	return estimateComputeMassAfterSignatures(transaction, false, 1, c.txMassCalculator)
}

func createTransactionWithJunkFieldsForMassCalculation(transaction *serialization.PartiallySignedTransaction, ecdsa bool, minimumSignatures uint32, txMassCalculator *txmass.Calculator) (*externalapi.DomainTransaction, error) {
	transaction = transaction.Clone()
	signatureSize := uint64(64)

	for i, input := range transaction.PartiallySignedInputs {
		for j, pubKeyPair := range input.PubKeySignaturePairs {
			if uint32(j) >= minimumSignatures {
				break
			}
			pubKeyPair.Signature = make([]byte, signatureSize+1) // +1 for SigHashType
		}
		transaction.Tx.Inputs[i].SigOpCount = byte(len(input.PubKeySignaturePairs))
	}

	return libkaspawallet.ExtractTransactionDeserialized(transaction, ecdsa)
}

func estimateComputeMassAfterSignatures(transaction *serialization.PartiallySignedTransaction, ecdsa bool, minimumSignatures uint32, txMassCalculator *txmass.Calculator) (uint64, error) {
	transactionWithSignatures, err := createTransactionWithJunkFieldsForMassCalculation(transaction, ecdsa, minimumSignatures, txMassCalculator)
	if err != nil {
		return 0, err
	}

	return txMassCalculator.CalculateTransactionMass(transactionWithSignatures), nil
}

func estimateTransientMass(transaction *serialization.PartiallySignedTransaction) (uint64, error) {

	serializedTx, err := serialization.SerializePartiallySignedTransaction(transaction)
	if err != nil {
		return uint64(0), err
	}
	return uint64(len(serializedTx) * TRANSIENT_BYTE_TO_MASS_FACTOR), nil
}

func EstimateMassAfterSignatures(transaction *serialization.PartiallySignedTransaction, ecdsa bool, minimumSignatures uint32, txMassCalculator *txmass.Calculator) (uint64, error) {
	transactionWithSignatures, err := createTransactionWithJunkFieldsForMassCalculation(transaction, ecdsa, minimumSignatures, txMassCalculator)
	if err != nil {
		return 0, err
	}

	return txMassCalculator.CalculateTransactionOverallMass(transactionWithSignatures), nil
}

/*func (s *Client) moreUTXOsForMergeTransaction(alreadySelectedUTXOs []*libkaspawallet.UTXO, requiredAmount uint64, feeRate float64) (
	additionalUTXOs []*libkaspawallet.UTXO, totalValueAdded uint64, err error) {

	dagInfo, err := s.rpcClient.GetBlockDAGInfo()
	if err != nil {
		return nil, 0, err
	}
	alreadySelectedUTXOsMap := make(map[externalapi.DomainOutpoint]struct{}, len(alreadySelectedUTXOs))
	for _, alreadySelectedUTXO := range alreadySelectedUTXOs {
		alreadySelectedUTXOsMap[*alreadySelectedUTXO.Outpoint] = struct{}{}
	}

	feePerInput, err := s.estimateFeePerInput(feeRate)
	if err != nil {
		return nil, 0, err
	}

	for _, utxo := range s.utxosSortedByAmount {
		if _, ok := alreadySelectedUTXOsMap[*utxo.Outpoint]; ok {
			continue
		}
		if !s.isUTXOSpendable(utxo, dagInfo.VirtualDAAScore) {
			continue
		}
		additionalUTXOs = append(additionalUTXOs, &libkaspawallet.UTXO{
			Outpoint:       utxo.Outpoint,
			UTXOEntry:      utxo.UTXOEntry,
			DerivationPath: s.walletAddressPath(utxo.address)})
		totalValueAdded += utxo.UTXOEntry.Amount() - feePerInput
		if totalValueAdded >= requiredAmount {
			break
		}
	}
	if totalValueAdded < requiredAmount {
		return nil, 0, errors.Errorf("Insufficient funds for merge transaction")
	}

	return additionalUTXOs, totalValueAdded, nil
}*/

func (c *Client) isUTXOSpendable(entry *walletUTXO, virtualDAAScore uint64) bool {
	if !entry.UTXOEntry.IsCoinbase() {
		return true
	}
	return entry.UTXOEntry.BlockDAAScore()+c.coinbaseMaturity < virtualDAAScore
}

func (c *Client) calculateFeeLimits() (feeRate float64, maxFee uint64, err error) {

	estimate, err := c.rpcClient.GetFeeEstimate()
	if err != nil {
		return 0, 0, err
	}
	feeRate = estimate.Estimate.NormalBuckets[0].Feerate
	// Default to a bound of max 1 KAS as fee
	maxFee = constants.MaxSompi

	return feeRate, maxFee, nil
}

func (c *Client) estimateFee(selectedUTXOs []*libkaspawallet.UTXO, feeRate float64, maxFee uint64, recipientValue uint64, blob []byte) (uint64, error) {
	fakePubKey := [util.PublicKeySizeECDSA]byte{}
	fakeAddr, err := util.NewAddressPublicKeyECDSA(fakePubKey[:], c.params.Prefix) // We assume the worst case where the recipient address is ECDSA. In this case the scriptPubKey will be the longest.
	if err != nil {
		return 0, err
	}

	totalValue := uint64(0)
	for _, utxo := range selectedUTXOs {
		totalValue += utxo.UTXOEntry.Amount()
	}

	// This is an approximation for the distribution of value between the recipient output and the change output.
	var mockPayments []*libkaspawallet.Payment
	if totalValue > recipientValue {
		mockPayments = []*libkaspawallet.Payment{
			{
				Address: fakeAddr,
				Amount:  recipientValue,
			},
			{
				Address: fakeAddr,
				Amount:  totalValue - recipientValue, // We ignore the fee since we expect it to be insignificant in mass calculation.
			},
		}
	} else {
		mockPayments = []*libkaspawallet.Payment{
			{
				Address: fakeAddr,
				Amount:  totalValue,
			},
		}
	}

	/*mockTx, err := createUnsignedTransaction(s.keysFile.ExtendedPublicKeys,
	s.keysFile.MinimumSignatures,
	mockPayments, selectedUTXOs, blob)*/
	/*mockTx, err := createUnsignedTransaction(s.keysFile.ExtendedPublicKeys,
	1,
	mockPayments, selectedUTXOs, blob)*/
	publickey, err := c.extendedKey.Public()
	if err != nil {
		return 0, err
	}

	mockTx, err := createUnsignedTransaction(publickey.String(), mockPayments, selectedUTXOs, blob)
	if err != nil {
		return 0, err
	}

	mass, err := c.estimateMassAfterSignatures(mockTx)
	if err != nil {
		return 0, err
	}

	return min(uint64(math.Ceil(float64(mass)*feeRate)), maxFee), nil
}

/*func (s *Client) estimateFeePerInput(feeRate float64) (uint64, error) {
	mockUTXO := &libkaspawallet.UTXO{
		Outpoint: &externalapi.DomainOutpoint{
			TransactionID: externalapi.DomainTransactionID{},
			Index:         0,
		},
		UTXOEntry: utxo.NewUTXOEntry(1, &externalapi.ScriptPublicKey{
			Script:  nil,
			Version: 0,
		}, false, 0),
		DerivationPath: "m",
	}

	mockTx, err := libkaspawallet.CreateUnsignedTransaction(s.keysFile.ExtendedPublicKeys,
		s.keysFile.MinimumSignatures,
		nil, []*libkaspawallet.UTXO{mockUTXO})
	if err != nil {
		return 0, err
	}

	// Here we use compute mass to avoid dividing by zero. This is ok since `s.estimateFeePerInput` is only used
	// in the case of compound transactions that have a compute mass higher than its storage mass.
	mass, err := s.estimateComputeMassAfterSignatures(mockTx)
	if err != nil {
		return 0, err
	}

	mockTxWithoutUTXO, err := libkaspawallet.CreateUnsignedTransaction(s.keysFile.ExtendedPublicKeys,
		s.keysFile.MinimumSignatures,
		nil, nil)
	if err != nil {
		return 0, err
	}

	massWithoutUTXO, err := s.estimateComputeMassAfterSignatures(mockTxWithoutUTXO)
	if err != nil {
		return 0, err
	}

	inputMass := mass - massWithoutUTXO

	return uint64(float64(inputMass) * feeRate), nil
}*/
