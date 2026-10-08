// Package indexer classifies raw Ethereum logs into stablecoin supply events.
// Topic0 hashes are the verified Mainnet values recorded in
// .planning/phases/01-live-supply-event-path/01-RESEARCH.md.
package indexer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"TOkenMonitor/internal/config"
)

// Verified topic0 constants (Keccak-256 of the canonical event signatures).
var (
	IssueTopic               = common.HexToHash("0xcb8241adb0c3fdb35b70c24ce35c5eb0c17af7431c99f827d44a445ca624176a") // USDT Issue(uint256)
	RedeemTopic              = common.HexToHash("0x702d5967f45f6513a38ffc42d6ba9bf230bd40e8f53b16363c7eb4fd2deb9a44") // USDT Redeem(uint256)
	DestroyedBlackFundsTopic = common.HexToHash("0x61e6e66b0d6339b2980aecc6ccc0039736791f0ccde9ed512e789a7fbdd698c6") // USDT DestroyedBlackFunds(address,uint256)
	TransferTopic            = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef") // ERC20 Transfer(address,address,uint256)
)

// SupplyEvent is one classified supply-changing event. Addresses are
// lowercase hex strings, empty when the event carries none.
type SupplyEvent struct {
	Token       string
	EventType   string // mint | redeem | destroyed_black_funds | burn
	Amount      *big.Int
	FromAddress string
	ToAddress   string
}

// SupplyTopics returns the topic0 set worth fetching for a token:
// USDT's three supply events, or the ERC20 Transfer for USDC.
func SupplyTopics(token config.Token) []common.Hash {
	return nil // stub: not implemented yet (RED phase)
}

// DecodeSupplyEvent classifies one log against the token's contract.
// A nil result with a nil error means "recognized but not supply-changing".
func DecodeSupplyEvent(token config.Token, log types.Log) (*SupplyEvent, error) {
	return nil, nil // stub: not implemented yet (RED phase)
}
