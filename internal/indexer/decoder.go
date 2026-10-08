// Package indexer classifies raw Ethereum logs into stablecoin supply events.
// Topic0 hashes are the verified Mainnet values recorded in
// .planning/phases/01-live-supply-event-path/01-RESEARCH.md.
package indexer

import (
	"fmt"
	"math/big"
	"strings"

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

// Event type vocabulary stored in stablecoin_events.event_type.
const (
	EventMint                = "mint"
	EventRedeem              = "redeem"
	EventDestroyedBlackFunds = "destroyed_black_funds"
	EventBurn                = "burn"
)

// Token symbols with a supply classification path (MVP scope: USDT and USDC).
const (
	SymbolUSDT = "USDT"
	SymbolUSDC = "USDC"
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

// SupplyTopics returns the topic0 set worth fetching for a token: USDT's
// three supply events, or the ERC20 Transfer for USDC (whose zero-address
// direction distinguishes mint from burn). Nil means nothing to fetch.
func SupplyTopics(token config.Token) []common.Hash {
	switch token.Symbol {
	case SymbolUSDT:
		return []common.Hash{IssueTopic, RedeemTopic, DestroyedBlackFundsTopic}
	case SymbolUSDC:
		return []common.Hash{TransferTopic}
	default:
		return nil
	}
}

// DecodeSupplyEvent classifies one log against the token's configured
// contract. A nil result with a nil error means "recognized but not
// supply-changing" (USDT Transfer, USDC ordinary Transfer, USDC paired
// Mint/Burn corroboration logs). Malformed shapes of recognized events are
// errors naming the event and its expected shape — never a zero amount.
func DecodeSupplyEvent(token config.Token, log types.Log) (*SupplyEvent, error) {
	want := common.HexToAddress(token.Contract)
	if log.Address != want {
		return nil, fmt.Errorf("decode %s: log address %s does not match configured contract %s",
			token.Symbol, addrHex(log.Address), addrHex(want))
	}
	if len(log.Topics) == 0 {
		return nil, nil
	}
	switch log.Topics[0] {
	case IssueTopic:
		if len(log.Topics) != 1 || len(log.Data) != 32 {
			return nil, fmt.Errorf("decode %s Issue: want 1 topic and 32 data bytes, got %d topics and %d data bytes",
				token.Symbol, len(log.Topics), len(log.Data))
		}
		return &SupplyEvent{Token: token.Symbol, EventType: EventMint, Amount: new(big.Int).SetBytes(log.Data)}, nil
	case RedeemTopic:
		if len(log.Topics) != 1 || len(log.Data) != 32 {
			return nil, fmt.Errorf("decode %s Redeem: want 1 topic and 32 data bytes, got %d topics and %d data bytes",
				token.Symbol, len(log.Topics), len(log.Data))
		}
		return &SupplyEvent{Token: token.Symbol, EventType: EventRedeem, Amount: new(big.Int).SetBytes(log.Data)}, nil
	case DestroyedBlackFundsTopic:
		if len(log.Topics) != 1 || len(log.Data) != 64 {
			return nil, fmt.Errorf("decode %s DestroyedBlackFunds: want 1 topic and 64 data bytes, got %d topics and %d data bytes",
				token.Symbol, len(log.Topics), len(log.Data))
		}
		return &SupplyEvent{
			Token:       token.Symbol,
			EventType:   EventDestroyedBlackFunds,
			FromAddress: addrHex(common.BytesToAddress(log.Data[0:32])),
			Amount:      new(big.Int).SetBytes(log.Data[32:64]),
		}, nil
	case TransferTopic:
		return decodeTransfer(token, log)
	default:
		// Not a supply-classified topic for this token: includes USDC's
		// paired Mint/Burn logs, which corroborate but never add rows.
		return nil, nil
	}
}

// decodeTransfer applies the USDC zero-address rule. A USDT Transfer —
// zero-address or not — is never a supply row: USDT changes supply only via
// Issue/Redeem/DestroyedBlackFunds.
func decodeTransfer(token config.Token, log types.Log) (*SupplyEvent, error) {
	if token.Symbol != SymbolUSDC {
		return nil, nil
	}
	if len(log.Topics) != 3 || len(log.Data) != 32 {
		return nil, fmt.Errorf("decode %s Transfer: want 3 topics and 32 data bytes, got %d topics and %d data bytes",
			token.Symbol, len(log.Topics), len(log.Data))
	}
	from := common.BytesToAddress(log.Topics[1].Bytes())
	to := common.BytesToAddress(log.Topics[2].Bytes())
	amount := new(big.Int).SetBytes(log.Data)
	var zero common.Address
	switch {
	case from == zero && to == zero:
		return nil, fmt.Errorf("decode %s Transfer: from and to are both the zero address; refusing to classify", token.Symbol)
	case from == zero:
		return &SupplyEvent{Token: token.Symbol, EventType: EventMint, ToAddress: addrHex(to), Amount: amount}, nil
	case to == zero:
		return &SupplyEvent{Token: token.Symbol, EventType: EventBurn, FromAddress: addrHex(from), Amount: amount}, nil
	default:
		return nil, nil // ordinary transfer: not a supply change
	}
}

// addrHex renders an address as a lowercase 0x-hex string.
func addrHex(a common.Address) string {
	return strings.ToLower(a.Hex())
}
