package orderbook

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// New returns newly initialized OrderBook
func New(targetSize int) *OrderBook {
	return &OrderBook{
		targetSize: targetSize,
		orderIDs:   map[string]OrderState{},
	}
}

func (ob *OrderBook) Process(scanner *bufio.Scanner) error {

	var previousBuyResult, previousSellResult float64
	for scanner.Scan() {
		inputString := scanner.Text()

		result, err := ob.parse(inputString)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		if result.OrderCode == "B" && floatEqual(previousBuyResult, result.Total) {
			continue
		}
		if result.OrderCode == "S" && floatEqual(previousSellResult, result.Total) {
			continue
		}

		output := formatResult(result)

		if output != "" {
			if result.OrderCode == "B" {
				previousBuyResult = result.Total
			} else {
				previousSellResult = result.Total

			}
			fmt.Println(output)
		}
	}

	return nil
}

func (ob *OrderBook) parse(inputString string) (*OrderResult, error) {
	in := strings.Split(inputString, " ")

	switch len(in) {
	case 6:
		// format: "timestamp operation id order_type price shares"
		price, _ := strconv.ParseFloat(in[4], 32) // ignoring error for now
		shares, _ := strconv.Atoi(in[5])
		orderType := getOrderType(in[3])
		newOrder := &Order{
			Timestamp: in[0],
			ID:        in[2],
			Type:      orderType,
			Price:     floatToFixedSign(price, 2),
		}

		income, err := ob.addOrder(shares, newOrder)
		if err != nil {
			return nil, err
		}

		timestamp := in[0]
		id := in[2]
		orderCode := getOrderCode(ob.orderIDs[id].Type)

		return &OrderResult{
			Timestamp: timestamp,
			OrderCode: orderCode,
			Total:     income,
		}, nil

	case 4:
		// format: "timestamp operation id shares"
		shares, _ := strconv.Atoi(in[3])

		expense := ob.removeSharesFromOrder(in[2], shares)

		timestamp := in[0]
		id := in[2]
		orderCode := getOrderCode(ob.orderIDs[id].Type)

		return &OrderResult{
			Timestamp: timestamp,
			OrderCode: orderCode,
			Total:     expense,
		}, nil

	}

	return nil, errors.New(fmt.Sprintf("failed to parse input %s", inputString))
}

// there are no orders adding new shares to existing
func (ob *OrderBook) addOrder(shares int, order *Order) (float64, error) {
	if order.Type == Undefined {
		return 0, errors.New("unknown order type")
	}

	orderState := OrderState{
		IsActive: true,
		Type:     order.Type,
		Shares:   shares,
	}
	ob.orderIDs[order.ID] = orderState

	if order.Type == BidOrder {
		return ob.addBidOrder(shares, order), nil
	}

	return ob.addAskOrder(shares, order), nil
}

func (ob *OrderBook) addBidOrder(shares int, order *Order) float64 {
	ob.bidShareSum += shares
	ob.bids = ob.addSortedOrder(order, ob.bids)

	return ob.executeOrder(order.ID)
}

func (ob *OrderBook) executeOrder(id string) float64 {
	var total float64
	{
		if ob.orderIDs[id].Type == BidOrder && ob.bidShareSum >= ob.targetSize {
			total = ob.sellShares()
		}
		if ob.orderIDs[id].Type == AskOrder && ob.askShareSum >= ob.targetSize {
			total = ob.buyShares()
		}
	}

	return total
}

func (ob *OrderBook) sellShares() float64 {
	// running while before selling target-size shares
	soldShares := 0
	maxPriceOrderIndex := len(ob.bids) - 1 // take bid with highest price
	var income float64
	for soldShares < ob.targetSize {
		order := ob.bids[maxPriceOrderIndex]
		orderState := ob.orderIDs[order.ID]
		currentSharesOnSale := orderState.Shares
		maxPriceOrderIndex -= 1

		// skip removed orders
		if !orderState.IsActive {
			continue
		}

		if soldShares+orderState.Shares > ob.targetSize {
			currentSharesOnSale = ob.targetSize - soldShares
		}

		income += float64(currentSharesOnSale) * order.Price
		soldShares += currentSharesOnSale
	}

	return income
}

func (ob *OrderBook) addAskOrder(shares int, order *Order) float64 {
	ob.askShareSum += shares
	ob.asks = ob.addSortedOrder(order, ob.asks)

	return ob.executeOrder(order.ID)
}

func (ob *OrderBook) buyShares() float64 {
	gainedShares := 0
	minPriceOrderIndex := 0 // take ask with lowest price
	var expense float64
	for gainedShares < ob.targetSize {
		order := ob.asks[minPriceOrderIndex]
		orderState := ob.orderIDs[order.ID]
		currentSharesOnBuy := orderState.Shares
		minPriceOrderIndex += 1

		// skip removed orders
		// todo: cleanup removed orders with some frequency
		if !orderState.IsActive {
			continue
		}

		if gainedShares+orderState.Shares > ob.targetSize {
			currentSharesOnBuy = ob.targetSize - gainedShares
		}

		expense += float64(currentSharesOnBuy) * order.Price
		gainedShares += currentSharesOnBuy
	}

	return expense
}

func (ob *OrderBook) removeSharesFromOrder(id string, shares int) float64 {
	orderState := ob.orderIDs[id]
	orderState.Shares -= shares
	// mark order as inactive as it has <= shares
	if orderState.Shares <= 0 {
		orderState.IsActive = false
	}
	ob.orderIDs[id] = orderState

	if orderState.Type == BidOrder {
		previousBidShareSum := ob.bidShareSum
		ob.bidShareSum -= shares
		if previousBidShareSum >= ob.targetSize && ob.bidShareSum < ob.targetSize {
			return -1
		}

	} else {
		previousAskShareSum := ob.askShareSum
		ob.askShareSum -= shares
		if previousAskShareSum >= ob.targetSize && ob.askShareSum < ob.targetSize {
			return -1
		}
	}

	return ob.executeOrder(id)
}

func getOrderCode(orderType OrderType) string {
	switch orderType {
	case BidOrder:
		return "S"
	case AskOrder:
		return "B"
	default:
		return ""
	}
}

func getOrderType(str string) OrderType {
	switch str {
	case "B":
		return BidOrder
	case "S":
		return AskOrder
	default:
		return Undefined
	}
}

func (ob *OrderBook) addSortedOrder(order *Order, data []*Order) []*Order {
	i := sort.Search(len(data), func(i int) bool {
		if data[i].Price == order.Price {
			return ob.orderIDs[data[i].ID].Shares <= ob.orderIDs[order.ID].Shares
		}
		return data[i].Price > order.Price
	})

	if i == len(data) {
		return append(data, order)
	}

	// make space for the inserted element by shifting values
	data = append(data[:i+1], data[i:]...)
	data[i] = order
	return data
}
