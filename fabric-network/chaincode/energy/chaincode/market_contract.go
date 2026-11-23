package chaincode

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// MarketContract is a FUTURE, legally dependent capability.
//
// Under Lei 14.300 (art. 28) and REN ANEEL 1.059/2023, energy credits can only
// offset consumption of the same holder: selling or transferring them to a
// neighbor is not allowed today. This contract therefore refuses every order
// unless the utility enables the regulatory sandbox in governance-cc, and all
// money positions it creates are simulated (they never reach a real bill).
type MarketContract struct {
	contractapi.Contract
}

// Order sides and statuses.
const (
	SideBid = "BID" // consumer wants to buy credits
	SideAsk = "ASK" // prosumer wants to sell credits

	StatusOpen      = "OPEN"
	StatusPartial   = "PARTIAL"
	StatusFilled    = "FILLED"
	StatusCancelled = "CANCELLED"
)

// Order is a limit order on the local market of a substation.
type Order struct {
	OrderID    string  `json:"orderID"`
	Side       string  `json:"side"`
	Owner      string  `json:"owner"`
	Substation string  `json:"substation"`
	Quantity   float64 `json:"quantity"`  // kWh / kWh
	Remaining  float64 `json:"remaining"` // kWh / kWh still open
	PriceBRL   float64 `json:"priceBRL"`  // limit price per kWh
	Status     string  `json:"status"`
	CreatedAt  int64   `json:"createdAt"`
}

// Trade is an executed match between a bid and an ask.
type Trade struct {
	TradeID    string  `json:"tradeID"`
	Substation string  `json:"substation"`
	Buyer      string  `json:"buyer"`
	Seller     string  `json:"seller"`
	BidID      string  `json:"bidID"`
	AskID      string  `json:"askID"`
	Quantity   float64 `json:"quantity"`
	PriceBRL   float64 `json:"priceBRL"`
	ValueBRL   float64 `json:"valueBRL"`
	FeeBRL     float64 `json:"feeBRL"`
	TxID       string  `json:"txID"`
	ExecutedAt int64   `json:"executedAt"`
}

// OrderResult is returned when an order is submitted.
type OrderResult struct {
	Order  Order   `json:"order"`
	Trades []Trade `json:"trades"`
}

// SubmitAsk (submitAsk) places a sell order. The credits are escrowed
// (locked) until the order is filled or cancelled.
func (m *MarketContract) SubmitAsk(
	ctx contractapi.TransactionContextInterface, seller, substation, quantity, priceBRL string,
) (*OrderResult, error) {
	return m.submit(ctx, SideAsk, seller, substation, quantity, priceBRL)
}

// SubmitBid (submitBid) places a buy order.
func (m *MarketContract) SubmitBid(
	ctx contractapi.TransactionContextInterface, buyer, substation, quantity, priceBRL string,
) (*OrderResult, error) {
	return m.submit(ctx, SideBid, buyer, substation, quantity, priceBRL)
}

// CancelOrder cancels an open order and releases any escrowed credits.
func (m *MarketContract) CancelOrder(ctx contractapi.TransactionContextInterface, orderID string) error {
	order, err := readOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if order.Status != StatusOpen && order.Status != StatusPartial {
		return fmt.Errorf("order %s is %s and cannot be cancelled", orderID, order.Status)
	}

	book := newAccountBook(ctx)
	if order.Side == SideAsk {
		w, err := book.get(order.Owner)
		if err != nil {
			return err
		}
		w.LockedKWh -= order.Remaining
		w.CreditKWh += order.Remaining
		book.note(order.Owner, fmt.Sprintf("CANCEL %s: unlock %.3f kWh", orderID, order.Remaining))
	} else {
		if _, err := book.get(order.Owner); err != nil {
			return err
		}
		book.note(order.Owner, fmt.Sprintf("CANCEL %s", orderID))
	}
	order.Status = StatusCancelled
	if err := book.flush(); err != nil {
		return err
	}
	return putJSON(ctx, OrderObjectType, []string{order.OrderID}, order)
}

// MatchOrders (matchOrders) re-runs the matching engine for a substation.
// Matching already happens automatically on every new order, so this is a
// maintenance operation (for example after a corridor change).
func (m *MarketContract) MatchOrders(ctx contractapi.TransactionContextInterface, substation string) ([]Trade, error) {
	if err := requireSandbox(ctx); err != nil {
		return nil, err
	}
	band, err := fetchPriceBand(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := loadOrders(ctx, substation, "")
	if err != nil {
		return nil, err
	}
	var bids []*Order
	for _, o := range orders {
		if o.Side == SideBid && isOpen(o) {
			bids = append(bids, o)
		}
	}
	sortBook(bids, SideBid)

	book := newAccountBook(ctx)
	var trades []Trade
	changed := map[string]*Order{}
	for _, bid := range bids {
		newTrades, touched, err := matchIncoming(ctx, book, band, bid, orders, len(trades))
		if err != nil {
			return nil, err
		}
		trades = append(trades, newTrades...)
		for _, o := range touched {
			changed[o.OrderID] = o
		}
	}
	if err := persistMatch(ctx, book, changed, trades); err != nil {
		return nil, err
	}
	if trades == nil {
		trades = []Trade{}
	}
	return trades, nil
}

// ListOrders returns orders filtered by substation and status (empty = all).
func (m *MarketContract) ListOrders(ctx contractapi.TransactionContextInterface, substation, status string) ([]Order, error) {
	orders, err := loadOrders(ctx, substation, status)
	if err != nil {
		return nil, err
	}
	result := []Order{}
	for _, o := range orders {
		result = append(result, *o)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	return result, nil
}

// ListTrades returns every executed trade.
func (m *MarketContract) ListTrades(ctx contractapi.TransactionContextInterface) ([]Trade, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(TradeObjectType, []string{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	trades := []Trade{}
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		var t Trade
		if err := json.Unmarshal(kv.Value, &t); err != nil {
			return nil, err
		}
		trades = append(trades, t)
	}
	sort.SliceStable(trades, func(i, j int) bool { return trades[i].ExecutedAt > trades[j].ExecutedAt })
	return trades, nil
}

// ##########################################
// #            Matching engine             #
// ##########################################

func (m *MarketContract) submit(
	ctx contractapi.TransactionContextInterface, side, owner, substation, quantityStr, priceStr string,
) (*OrderResult, error) {
	if err := requireSandbox(ctx); err != nil {
		return nil, err
	}
	owner = strings.TrimSpace(owner)
	substation = strings.TrimSpace(substation)
	if owner == "" || substation == "" {
		return nil, fmt.Errorf("owner and substation are required")
	}
	qty, err := strconv.ParseFloat(quantityStr, 64)
	if err != nil || qty <= 0 {
		return nil, fmt.Errorf("invalid quantity")
	}
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil || price <= 0 {
		return nil, fmt.Errorf("invalid price")
	}
	qty = round3(qty)
	price = round4(price)

	// 1. Governed Dynamic Corridor: reject any price outside the band.
	band, err := fetchPriceBand(ctx)
	if err != nil {
		return nil, err
	}
	if price < band.FloorBRL || price > band.CeilingBRL {
		return nil, fmt.Errorf("price R$ %.4f/kWh rejected: outside the governed corridor [R$ %.4f, R$ %.4f]", price, band.FloorBRL, band.CeilingBRL)
	}

	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return nil, err
	}

	book := newAccountBook(ctx)

	// 2. Sellers must own the credits, which are escrowed while the order is open.
	if side == SideAsk {
		w, err := book.get(owner)
		if err != nil {
			return nil, err
		}
		if w.CreditKWh < qty {
			return nil, fmt.Errorf("insufficient balance: %s has %.3f kWh available", owner, w.CreditKWh)
		}
		w.CreditKWh -= qty
		w.LockedKWh += qty
		book.note(owner, fmt.Sprintf("ASK %.3f kWh @ R$ %.4f (locked)", qty, price))
	} else {
		if _, err := book.get(owner); err != nil {
			return nil, err
		}
		book.note(owner, fmt.Sprintf("BID %.3f kWh @ R$ %.4f", qty, price))
	}

	order := &Order{
		OrderID:    "ORD-" + shortTxID(ctx),
		Side:       side,
		Owner:      owner,
		Substation: substation,
		Quantity:   qty,
		Remaining:  qty,
		PriceBRL:   price,
		Status:     StatusOpen,
		CreatedAt:  ts.Seconds,
	}

	// 3. Match against the opposite side of the same substation.
	resting, err := loadOrders(ctx, substation, "")
	if err != nil {
		return nil, err
	}
	trades, touched, err := matchIncoming(ctx, book, band, order, resting, 0)
	if err != nil {
		return nil, err
	}

	changed := map[string]*Order{order.OrderID: order}
	for _, o := range touched {
		changed[o.OrderID] = o
	}
	if err := persistMatch(ctx, book, changed, trades); err != nil {
		return nil, err
	}
	if trades == nil {
		trades = []Trade{}
	}
	return &OrderResult{Order: *order, Trades: trades}, nil
}

// matchIncoming matches one order against the resting orders using
// price-time priority. Trades execute at the resting (maker) price.
func matchIncoming(
	ctx contractapi.TransactionContextInterface, book *accountBook, band *PriceBand,
	incoming *Order, resting []*Order, tradeOffset int,
) ([]Trade, []*Order, error) {
	opposite := SideAsk
	if incoming.Side == SideAsk {
		opposite = SideBid
	}

	var candidates []*Order
	for _, o := range resting {
		if o.Side == opposite && isOpen(o) && o.Substation == incoming.Substation && o.Owner != incoming.Owner {
			candidates = append(candidates, o)
		}
	}
	sortBook(candidates, opposite)

	ts, err := ctx.GetStub().GetTxTimestamp()
	if err != nil {
		return nil, nil, err
	}

	var trades []Trade
	var touched []*Order
	for _, counter := range candidates {
		if incoming.Remaining <= 0 {
			break
		}
		bid, ask := incoming, counter
		if incoming.Side == SideAsk {
			bid, ask = counter, incoming
		}
		if bid.PriceBRL < ask.PriceBRL {
			break // book is sorted, no further match possible
		}

		qty := round3(math.Min(incoming.Remaining, counter.Remaining))
		price := counter.PriceBRL
		value := round4(qty * price)
		fee := round4(value * float64(band.FeeBps) / 10000)

		seller, err := book.get(ask.Owner)
		if err != nil {
			return nil, nil, err
		}
		buyer, err := book.get(bid.Owner)
		if err != nil {
			return nil, nil, err
		}
		treasury, err := book.get(TreasuryAccount)
		if err != nil {
			return nil, nil, err
		}

		// Atomic swap: credits go to the buyer, money positions are recorded
		// for the utility bill, and the brokerage fee goes to the treasury.
		seller.LockedKWh -= qty
		buyer.CreditKWh += qty
		seller.SandboxReceivableBRL += value - fee
		buyer.SandboxPayableBRL += value
		treasury.SandboxReceivableBRL += fee

		incoming.Remaining = round3(incoming.Remaining - qty)
		counter.Remaining = round3(counter.Remaining - qty)
		updateStatus(incoming)
		updateStatus(counter)
		touched = append(touched, counter)

		tradeID := fmt.Sprintf("TRD-%s-%d", shortTxID(ctx), tradeOffset+len(trades)+1)
		book.note(ask.Owner, fmt.Sprintf("SOLD %.3f kWh to %s @ R$ %.4f", qty, bid.Owner, price))
		book.note(bid.Owner, fmt.Sprintf("BOUGHT %.3f kWh from %s @ R$ %.4f", qty, ask.Owner, price))
		book.note(TreasuryAccount, fmt.Sprintf("FEE R$ %.4f (%s)", fee, tradeID))

		trades = append(trades, Trade{
			TradeID:    tradeID,
			Substation: incoming.Substation,
			Buyer:      bid.Owner,
			Seller:     ask.Owner,
			BidID:      bid.OrderID,
			AskID:      ask.OrderID,
			Quantity:   qty,
			PriceBRL:   price,
			ValueBRL:   value,
			FeeBRL:     fee,
			TxID:       ctx.GetStub().GetTxID(),
			ExecutedAt: ts.Seconds,
		})
	}
	return trades, touched, nil
}

func persistMatch(ctx contractapi.TransactionContextInterface, book *accountBook, changed map[string]*Order, trades []Trade) error {
	if err := book.flush(); err != nil {
		return err
	}
	ids := make([]string, 0, len(changed))
	for id := range changed {
		ids = append(ids, id)
	}
	sort.Strings(ids) // deterministic write order
	for _, id := range ids {
		if err := putJSON(ctx, OrderObjectType, []string{id}, changed[id]); err != nil {
			return err
		}
	}
	for _, t := range trades {
		if err := putJSON(ctx, TradeObjectType, []string{t.TradeID}, t); err != nil {
			return err
		}
	}
	if len(trades) > 0 {
		bz, _ := json.Marshal(trades)
		if err := ctx.GetStub().SetEvent("TradeExecuted", bz); err != nil {
			return err
		}
	}
	return nil
}

func loadOrders(ctx contractapi.TransactionContextInterface, substation, status string) ([]*Order, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey(OrderObjectType, []string{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var orders []*Order
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, err
		}
		var o Order
		if err := json.Unmarshal(kv.Value, &o); err != nil {
			return nil, err
		}
		if substation != "" && o.Substation != substation {
			continue
		}
		if status != "" && o.Status != status {
			continue
		}
		orders = append(orders, &o)
	}
	return orders, nil
}

func readOrder(ctx contractapi.TransactionContextInterface, orderID string) (*Order, error) {
	key, err := ctx.GetStub().CreateCompositeKey(OrderObjectType, []string{orderID})
	if err != nil {
		return nil, err
	}
	bz, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, err
	}
	if bz == nil {
		return nil, fmt.Errorf("order %s not found", orderID)
	}
	var o Order
	if err := json.Unmarshal(bz, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// sortBook orders asks by lowest price first and bids by highest price first,
// breaking ties by time and then by ID (deterministic on every peer).
func sortBook(orders []*Order, side string) {
	sort.SliceStable(orders, func(i, j int) bool {
		a, b := orders[i], orders[j]
		if a.PriceBRL != b.PriceBRL {
			if side == SideAsk {
				return a.PriceBRL < b.PriceBRL
			}
			return a.PriceBRL > b.PriceBRL
		}
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		return a.OrderID < b.OrderID
	})
}

// requireSandbox refuses market operations unless the regulatory sandbox is on.
func requireSandbox(ctx contractapi.TransactionContextInterface) error {
	cfg, err := fetchSandbox(ctx)
	if err != nil {
		return err
	}
	if !cfg.P2PEnabled {
		return fmt.Errorf("P2P trading is disabled: Lei 14.300 and REN 1.059 do not allow selling or transferring credits to another holder; it can only run inside an authorized regulatory sandbox")
	}
	return nil
}

func isOpen(o *Order) bool {
	return (o.Status == StatusOpen || o.Status == StatusPartial) && o.Remaining > 0
}

func updateStatus(o *Order) {
	switch {
	case o.Remaining <= 0:
		o.Remaining = 0
		o.Status = StatusFilled
	case o.Remaining < o.Quantity:
		o.Status = StatusPartial
	}
}
