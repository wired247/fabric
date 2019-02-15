/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package plain

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/core/ledger/customtx"
	"github.com/hyperledger/fabric/protos/token"
	"github.com/hyperledger/fabric/protos/utils"
	"github.com/hyperledger/fabric/token/identity"
	"github.com/hyperledger/fabric/token/ledger"
	"github.com/pkg/errors"
)

const (
	minUnicodeRuneValue   = 0            //U+0000
	maxUnicodeRuneValue   = utf8.MaxRune //U+10FFFF - maximum (and unallocated) code point
	compositeKeyNamespace = "\x00"
	tokenOutput           = "tokenOutput"
	tokenRedeem           = "tokenRedeem"
	tokenInput            = "tokenInput"
	tokenDelegatedInput   = "tokenDelegateInput"
	tokenNameSpace        = "_fabtoken"
)

var verifierLogger = flogging.MustGetLogger("token.tms.plain.verifier")

// A Verifier validates and commits token transactions.
type Verifier struct {
	IssuingValidator    identity.IssuingValidator
	TokenOwnerValidator identity.TokenOwnerValidator
}

// ProcessTx checks that transactions are correct wrt. the most recent ledger state.
// ProcessTx checks are ones that shall be done sequentially, since transactions within a block may introduce dependencies.
func (v *Verifier) ProcessTx(txID string, creator identity.PublicInfo, ttx *token.TokenTransaction, simulator ledger.LedgerWriter) error {
	verifierLogger.Debugf("checking transaction with txID '%s'", txID)
	err := v.checkProcess(txID, creator, ttx, simulator)
	if err != nil {
		return err
	}

	verifierLogger.Debugf("committing transaction with txID '%s'", txID)
	err = v.commitProcess(txID, creator, ttx, simulator)
	if err != nil {
		verifierLogger.Errorf("error committing transaction with txID '%s': %s", txID, err)
		return err
	}
	verifierLogger.Debugf("successfully processed transaction with txID '%s'", txID)
	return nil
}

func (v *Verifier) checkProcess(txID string, creator identity.PublicInfo, ttx *token.TokenTransaction, simulator ledger.LedgerReader) error {
	action := ttx.GetPlainAction()
	if action == nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("check process failed for transaction '%s': missing token action", txID)}
	}

	err := v.checkAction(creator, action, txID, simulator)
	if err != nil {
		return err
	}

	return nil
}

func (v *Verifier) checkAction(creator identity.PublicInfo, plainAction *token.PlainTokenAction, txID string, simulator ledger.LedgerReader) error {
	switch action := plainAction.Data.(type) {
	case *token.PlainTokenAction_PlainImport:
		return v.checkImportAction(creator, action.PlainImport, txID, simulator)
	case *token.PlainTokenAction_PlainTransfer:
		return v.checkTransferAction(creator, action.PlainTransfer, txID, simulator)
	case *token.PlainTokenAction_PlainRedeem:
		return v.checkRedeemAction(creator, action.PlainRedeem, txID, simulator)
	default:
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("unknown plain token action: %T", action)}
	}
}

func (v *Verifier) checkImportAction(creator identity.PublicInfo, importAction *token.PlainImport, txID string, simulator ledger.LedgerReader) error {
	err := v.checkImportOutputs(importAction.GetOutputs(), txID, simulator)
	if err != nil {
		return err
	}
	return v.checkImportPolicy(creator, txID, importAction)
}

func (v *Verifier) checkImportOutputs(outputs []*token.PlainOutput, txID string, simulator ledger.LedgerReader) error {
	if len(outputs) == 0 {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("no outputs in transaction: %s", txID)}
	}
	for i, output := range outputs {
		err := v.checkOutputDoesNotExist(i, output, txID, simulator)
		if err != nil {
			return err
		}

		if output.Quantity == 0 {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("output %d quantity is 0 in transaction: %s", i, txID)}
		}

		if output.Owner == nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("missing owner in output for txID '%s'", txID)}
		}
	}
	return nil
}

func (v *Verifier) checkTransferAction(creator identity.PublicInfo, transferAction *token.PlainTransfer, txID string, simulator ledger.LedgerReader) error {
	return v.checkInputsAndOutputs(creator, transferAction.GetInputs(), transferAction.GetOutputs(), txID, simulator, true)
}

func (v *Verifier) checkRedeemAction(creator identity.PublicInfo, redeemAction *token.PlainTransfer, txID string, simulator ledger.LedgerReader) error {
	err := v.checkInputsAndOutputs(creator, redeemAction.GetInputs(), redeemAction.GetOutputs(), txID, simulator, false)
	if err != nil {
		return err
	}

	// perform additional checking for redeem outputs
	// redeem transaction should not have more than 2 outputs.
	outputs := redeemAction.GetOutputs()
	if len(outputs) > 2 {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("too many outputs (%d) in a redeem transaction", len(outputs))}
	}

	// output[0] should always be a redeem output - i.e., owner should be nil
	if outputs[0].Owner != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("owner should be nil in a redeem output")}
	}

	// if output[1] presents, its owner must be same as the creator
	if len(outputs) == 2 && !bytes.Equal(creator.Public(), outputs[1].Owner.Raw) {
		println(hex.EncodeToString(creator.Public()))
		println(hex.EncodeToString(outputs[1].Owner.Raw))
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("wrong owner for remaining tokens, should be original owner %s, but got %s", creator.Public(), outputs[1].Owner.Raw)}
	}

	return nil
}

// checkInputsAndOutputs checks that inputs and outputs are valid and have same type and sum of quantity
func (v *Verifier) checkInputsAndOutputs(
	creator identity.PublicInfo,
	tokenIds []*token.TokenId,
	outputs []*token.PlainOutput,
	txID string,
	simulator ledger.LedgerReader,
	ownerRequired bool) error {

	outputType, outputSum, err := v.checkOutputs(outputs, txID, simulator, ownerRequired)
	if err != nil {
		return err
	}
	inputType, inputSum, err := v.checkInputs(creator, tokenIds, txID, simulator)
	if err != nil {
		return err
	}
	if outputType != inputType {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("token type mismatch in inputs and outputs for transaction ID %s (%s vs %s)", txID, outputType, inputType)}
	}
	if outputSum != inputSum {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("token sum mismatch in inputs and outputs for transaction ID %s (%d vs %d)", txID, outputSum, inputSum)}
	}
	return nil
}

func (v *Verifier) checkOutputDoesNotExist(index int, output *token.PlainOutput, txID string, simulator ledger.LedgerReader) error {
	var outputID string
	var err error
	if output.Owner != nil {
		outputID, err = createOutputKey(txID, index)
	} else {
		outputID, err = createRedeemKey(txID, index)
	}
	if err != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
	}

	existingOutputBytes, err := simulator.GetState(tokenNameSpace, outputID)
	if err != nil {
		return err
	}

	if existingOutputBytes != nil {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("output already exists: %s", outputID)}
	}
	return nil
}

func (v *Verifier) checkOutputs(outputs []*token.PlainOutput, txID string, simulator ledger.LedgerReader, ownerRequired bool) (string, uint64, error) {
	tokenType := ""
	tokenSum := uint64(0)
	for i, output := range outputs {
		err := v.checkOutputDoesNotExist(i, output, txID, simulator)
		if err != nil {
			return "", 0, err
		}
		if tokenType == "" {
			tokenType = output.GetType()
		} else if tokenType != output.GetType() {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("multiple token types ('%s', '%s') in output for txID '%s'", tokenType, output.GetType(), txID)}
		}
		if ownerRequired {
			err = v.TokenOwnerValidator.Validate(output.GetOwner())
			if err != nil {
				return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("invalid owner in output for txID '%s', err '%s'", txID, err)}
			}
		}
		tokenSum += output.GetQuantity()
	}
	return tokenType, tokenSum, nil
}

func (v *Verifier) checkInputs(creator identity.PublicInfo, tokenIds []*token.TokenId, txID string, simulator ledger.LedgerReader) (string, uint64, error) {
	tokenType := ""
	inputSum := uint64(0)
	processedIDs := make(map[string]bool)
	for _, id := range tokenIds {
		inputKey, err := createOutputKey(id.TxId, int(id.Index))
		if err != nil {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID for transfer input: %s", err)}
		}
		input, err := v.getOutput(inputKey, simulator)
		if err != nil {
			return "", 0, err
		}
		if input == nil {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("input with ID %s for transfer does not exist", inputKey)}
		}
		err = v.checkInputOwner(creator, input, inputKey)
		if err != nil {
			return "", 0, err
		}
		if tokenType == "" {
			tokenType = input.GetType()
		} else if tokenType != input.GetType() {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("multiple token types in input for txID: %s (%s, %s)", txID, tokenType, input.GetType())}
		}
		if processedIDs[inputKey] {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("token input '%s' spent more than once in transaction ID '%s'", inputKey, txID)}
		}
		processedIDs[inputKey] = true
		inputSum += input.GetQuantity()
		spentKey, err := createSpentKey(id.TxId, int(id.Index))
		if err != nil {
			return "", 0, err
		}
		spent, err := v.isSpent(spentKey, simulator)
		if err != nil {
			return "", 0, err
		}
		if spent {
			return "", 0, &customtx.InvalidTxError{Msg: fmt.Sprintf("input with ID %s for transfer has already been spent", inputKey)}
		}
	}
	return tokenType, inputSum, nil
}

func (v *Verifier) checkInputOwner(creator identity.PublicInfo, input *token.PlainOutput, tokenId string) error {
	if !bytes.Equal(creator.Public(), input.Owner.Raw) {
		return &customtx.InvalidTxError{Msg: fmt.Sprintf("transfer input with ID %s not owned by creator", tokenId)}
	}
	return nil
}

func (v *Verifier) checkImportPolicy(creator identity.PublicInfo, txID string, importData *token.PlainImport) error {
	for _, output := range importData.Outputs {
		err := v.IssuingValidator.Validate(creator, output.Type)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("import policy check failed: %s", err)}
		}
	}
	return nil
}

func (v *Verifier) commitProcess(txID string, creator identity.PublicInfo, ttx *token.TokenTransaction, simulator ledger.LedgerWriter) error {
	verifierLogger.Debugf("committing action with txID '%s'", txID)
	err := v.commitAction(ttx.GetPlainAction(), txID, simulator)
	if err != nil {
		verifierLogger.Errorf("error committing action with txID '%s': %s", txID, err)
		return err
	}

	verifierLogger.Debugf("action with txID '%s' committed successfully", txID)
	return nil
}

func (v *Verifier) commitAction(plainAction *token.PlainTokenAction, txID string, simulator ledger.LedgerWriter) (err error) {
	switch action := plainAction.Data.(type) {
	case *token.PlainTokenAction_PlainImport:
		err = v.commitImportAction(action.PlainImport, txID, simulator)
	case *token.PlainTokenAction_PlainTransfer:
		err = v.commitTransferAction(action.PlainTransfer, txID, simulator)
	case *token.PlainTokenAction_PlainRedeem:
		// call the same commit method as transfer because PlainRedeem points to the same type of outputs as transfer
		err = v.commitTransferAction(action.PlainRedeem, txID, simulator)
	}
	return
}

func (v *Verifier) commitImportAction(importAction *token.PlainImport, txID string, simulator ledger.LedgerWriter) error {
	for i, output := range importAction.GetOutputs() {
		outputID, err := createOutputKey(txID, i)
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
		}

		err = v.addOutput(outputID, output, simulator)
		if err != nil {
			return err
		}
	}
	return nil
}

// commitTransferAction is called for both transfer and redeem transactions
// Check the owner of each output to determine how to generate the key
func (v *Verifier) commitTransferAction(transferAction *token.PlainTransfer, txID string, simulator ledger.LedgerWriter) error {
	var outputID string
	var err error
	for i, output := range transferAction.GetOutputs() {
		if output.Owner != nil {
			outputID, err = createOutputKey(txID, i)
		} else {
			outputID, err = createRedeemKey(txID, i)
		}
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating output ID: %s", err)}
		}

		err = v.addOutput(outputID, output, simulator)
		if err != nil {
			return err
		}
	}
	return v.markInputsSpent(txID, transferAction.GetInputs(), simulator)
}

func (v *Verifier) addOutput(outputID string, output *token.PlainOutput, simulator ledger.LedgerWriter) error {
	outputBytes := utils.MarshalOrPanic(output)

	return simulator.SetState(tokenNameSpace, outputID, outputBytes)
}

var TokenInputSpentMarker = []byte{1}

func (v *Verifier) markInputsSpent(txID string, inputs []*token.TokenId, simulator ledger.LedgerWriter) error {
	for _, id := range inputs {
		tokenId, err := createSpentKey(id.TxId, int(id.Index))
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating spent key: %s", err)}
		}
		verifierLogger.Debugf("marking input '%s' as spent", tokenId)
		err = simulator.SetState(tokenNameSpace, tokenId, TokenInputSpentMarker)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *Verifier) markDelegatedInputsSpent(txID string, inputs []*token.TokenId, simulator ledger.LedgerWriter) error {
	for _, id := range inputs {
		tokenId, err := createSpentDelegatedOutputKey(id.TxId, int(id.Index))
		if err != nil {
			return &customtx.InvalidTxError{Msg: fmt.Sprintf("error creating delegated input spent key: %s", err)}
		}

		err = simulator.SetState(tokenNameSpace, tokenId, TokenInputSpentMarker)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *Verifier) getOutput(outputID string, simulator ledger.LedgerReader) (*token.PlainOutput, error) {
	if !strings.Contains(outputID, tokenOutput) {
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("input with ID %s for transfer is not an output", outputID)}
	}
	outputBytes, err := simulator.GetState(tokenNameSpace, outputID)
	if err != nil {
		return nil, err
	}
	if outputBytes == nil {
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("input with ID %s for transfer does not exist", outputID)}
	}
	if len(outputBytes) == 0 {
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("input with ID %s for transfer does not exist", outputID)}
	}
	output := &token.PlainOutput{}
	err = proto.Unmarshal(outputBytes, output)
	if err != nil {
		return nil, &customtx.InvalidTxError{Msg: fmt.Sprintf("unmarshaling error: %s", err)}
	}
	return output, nil
}

// isSpent checks whether an output token with identifier outputID has been spent.
func (v *Verifier) isSpent(spentKey string, simulator ledger.LedgerReader) (bool, error) {
	verifierLogger.Debugf("checking if input with ID '%s' has been spent", spentKey)
	result, err := simulator.GetState(tokenNameSpace, spentKey)
	return result != nil, err
}

// Create a ledger key for an individual output in a token transaction, as a function of
// the transaction ID, and the index of the output
func createOutputKey(txID string, index int) (string, error) {
	return createCompositeKey(tokenOutput, []string{txID, strconv.Itoa(index)})
}

// Create a ledger key for a redeem output in a token transaction, as a function of
// the transaction ID, and the index of the output
func createRedeemKey(txID string, index int) (string, error) {
	return createCompositeKey(tokenRedeem, []string{txID, strconv.Itoa(index)})
}

// Create a ledger key for a spent individual output in a token transaction, as a function of
// the transaction ID, and the index of the output
func createSpentKey(txID string, index int) (string, error) {
	return createCompositeKey(tokenInput, []string{txID, strconv.Itoa(index)})
}

// Create a ledger key for a spent individual delegated output in a token transaction, as a function of
// the transaction ID, and the index of the delegated output
func createSpentDelegatedOutputKey(txID string, index int) (string, error) {
	return createCompositeKey(tokenDelegatedInput, []string{txID, strconv.Itoa(index)})
}

// createCompositeKey and its related functions and consts copied from core/chaincode/shim/chaincode.go
func createCompositeKey(objectType string, attributes []string) (string, error) {
	if err := validateCompositeKeyAttribute(objectType); err != nil {
		return "", err
	}
	ck := compositeKeyNamespace + objectType + string(minUnicodeRuneValue)
	for _, att := range attributes {
		if err := validateCompositeKeyAttribute(att); err != nil {
			return "", err
		}
		ck += att + string(minUnicodeRuneValue)
	}
	return ck, nil
}

func validateCompositeKeyAttribute(str string) error {
	if !utf8.ValidString(str) {
		return errors.Errorf("not a valid utf8 string: [%x]", str)
	}
	for index, runeValue := range str {
		if runeValue == minUnicodeRuneValue || runeValue == maxUnicodeRuneValue {
			return errors.Errorf(`input contain unicode %#U starting at position [%d]. %#U and %#U are not allowed in the input attribute of a composite key`,
				runeValue, index, minUnicodeRuneValue, maxUnicodeRuneValue)
		}
	}
	return nil
}
