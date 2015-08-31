//Buy
//go run ib.go jReg l buy
//go run ib.go jReg nl buy
//sell
//go run ib.go all l sell

package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"strconv"
	"time"

	"github.com/gofinance/ib"
	"github.com/stocks"
)

var nextOrderTimeout = time.Second * 5

//var doExecute = true
var useArgs = false
var err error
var doExecute bool
var jReg = "U1530416"
var gReg = "U1530752"
var gIra = "U1531576"
var mIra = "U1556876"
var ticker = "AAPL"
var tradingFunds int
var buy = "buy"
var sell = "sell"
var orderType string
var tif string
var shares int64
var myEngine *ib.Engine
var theAcct string
var useLeverage bool
var argShares int64
var theAction string
var outsideRTH bool

//var argShares string
var nextOrderID int64
var rc = make(chan ib.Reply)

//var outsideRTH bool

//limit price is .5% greater than current price
var slippage = 0.003

//number of shares is 1% less than exact amount
var shareSlippage = 0.02

//ExecutionInfo don't know what this is for
type ExecutionInfo struct {
	ExecutionData ib.ExecutionData
	Commission    ib.CommissionReport
}

//IBManager set up a manager
type IBManager struct {
	nextOrderID int64
	engine      *ib.Engine
}

func getNextOrderID(mgr IBManager) chan int64 {
	res := make(chan int64)
	go func() {
		for {
			r := <-rc
			switch r.(type) {
			case (*ib.NextValidID):
				r := r.(*ib.NextValidID)
				res <- (r.OrderID)
			default:
				fmt.Println(r)
			}
		}
	}()
	return res
}

func getNextOrderIDWithTimeout(mgr IBManager) (int64, error) {
	select {
	case <-time.After(nextOrderTimeout):
		return 0, fmt.Errorf("Timeout looking for order")
	case res := <-getNextOrderID(mgr):
		return res, nil

	}
}

//NewOrder comment
func NewOrder() (ib.Order, error) {
	order, err := ib.NewOrder()
	order.OutsideRTH = false
	return order, err
}

//NewContract comment
func NewContract(symbol string) ib.Contract {
	return ib.Contract{
		Symbol:       symbol,
		SecurityType: "STK",
		Exchange:     "SMART",
		Currency:     "USD",
	}
}

//Round is simple
func Round(f float64) float64 {
	return math.Floor(f + .5)
}

func doBuy(mgr *IBManager, symbol string, quantity int64, orderType string, limitPrice float64, account string, tIF string, nextOrderID int64, outsideRTH bool) {
	request := ib.PlaceOrder{Contract: NewContract(symbol)}

	request.Order, _ = NewOrder()
	request.Order.Action = "BUY"
	request.Order.TIF = tIF
	request.Order.OrderType = orderType
	request.Order.LimitPrice = limitPrice
	request.Order.TotalQty = quantity
	request.Order.Account = account
	request.Order.OutsideRTH = outsideRTH
	request.SetID(nextOrderID)

	fmt.Printf("%s %s %d shares at $%6.2f using %s, outside: %t\n", request.Order.Account, request.Order.Action, request.Order.TotalQty, request.Order.LimitPrice, request.Order.OrderType, request.Order.OutsideRTH)
	if doExecute {
		mgr.engine.Send(&request)
	}

}

func doSell(mgr *IBManager, symbol string, shares int64, orderType string, tIF string, nextOrderID int64) {
	request := ib.PlaceOrder{Contract: NewContract(symbol)}

	request.Order, _ = NewOrder()
	request.Order.Action = "SELL"
	request.Order.TIF = tIF
	request.Order.OrderType = orderType
	request.Order.LimitPrice = 0
	request.Order.FAMethod = "PctChange"
	request.Order.FAPercentage = "-100"
	request.Order.FAGroup = "everyone"
	request.Order.FAProfile = ""
	request.Order.Account = ""

	request.SetID(nextOrderID)
	fmt.Printf("%s %s %s%% at %s, using %s\n", request.Order.FAGroup, request.Order.Action, request.Order.FAPercentage, request.Order.TIF, request.Order.OrderType)
	if doExecute {
		mgr.engine.Send(&request)
	}

}
func getShares(shares int64, tradingFunds string, thePrice float64) int64 {
	if shares == 0 {
		tradingFundsReal, _ := strconv.ParseFloat(tradingFunds, 64)
		sharesx := float64(tradingFundsReal) / thePrice
		shares = int64(sharesx - (sharesx * shareSlippage))
	}
	return shares
}

//convert the account abbreviation to the ib account name string
func acctNametoNumber(acctName string) string {
	var acctNum string
	switch {
	case acctName == "jReg":
		acctNum = jReg
	case acctName == "gReg":
		acctNum = gReg
	case acctName == "gIra":
		acctNum = gIra
	case acctName == "jReg":
		acctNum = jReg
	case acctName == "mIra":
		acctNum = mIra
	}
	return acctNum
}

func doTrades() {
	var err error
	myEngine, err = ib.NewEngine(ib.EngineOptions{})
	if err != nil {
		log.Fatalf("error creating %s Engine ", err)
	}
	defer myEngine.Stop()
	if myEngine.State() != ib.EngineReady {
		log.Fatalf("engine is not ready")
	}

	myAccountManager, err := ib.NewAdvisorAccountManager(myEngine)
	if err != nil {
		panic(err)
	}
	<-myAccountManager.Refresh()
	defer myAccountManager.Close()

	valueMap := myAccountManager.Values()
	stockFromYahoo, err := stocks.GetQuote(ticker)
	if err != nil {
		fmt.Println(err)
	}
	aQuote, err := stockFromYahoo.GetPrice()
	if err != nil {
		fmt.Println(err)
	}
	quoteSlipped := Round((aQuote+(aQuote*slippage))*100) / 100

	//check on shares based on leverage
	for aVk, aV := range valueMap {
		//availableFunds are either buyingPower or netliquadation
		correctAcct := (aVk.AccountCode == theAcct)
		correctForLever := (aVk.Key == "BuyingPower") && useLeverage
		correctForNoLever := (aVk.Key == "NetLiquidation") && !useLeverage

		if correctAcct && correctForLever {
			shares = getShares(argShares, aV.Value, quoteSlipped)
			shares = shares - int64(float64(shares)*0.6)
		}
		if correctAcct && correctForNoLever {
			shares = getShares(argShares, aV.Value, quoteSlipped)
		}
	}

	//	fmt.Println("quote", aQuote, "slipped-", quoteSlipped, "shares", shares)
	mgr := IBManager{engine: myEngine}
	mgr.engine.SubscribeAll(rc)

	mgr.engine.Send(&ib.RequestIDs{})
	nextOrderID, err = getNextOrderIDWithTimeout(mgr)
	if err != nil {
		panic(err)
	}

	//	fmt.Println("the next order ID is: ", nextOrderID)
	if theAction == "buy" {
		doBuy(&mgr,
			"AAPL",
			shares,       // number shares
			"LOC",        // mkt, moc, lmt
			quoteSlipped, // price
			theAcct,      // account
			"DAY",        // DAY OPG
			nextOrderID,
			outsideRTH) //out side regular trading hours
	} else if theAction == "sell" { //positions := ib.RequestPositions
		doSell(&mgr, "AAPL", shares, "MARKET", "OPG", nextOrderID)
	} else {
		fmt.Println("neither a buy nor a sell")
	}
	//	nextOrderID = getNextOrderID(mgr)
}

// func doTradeRepeating(numTimesLeft int) {
// 	if numTimesLeft == 0 {
// 		return
// 	}
// 	defer func() {
// 		// this will only be true if `doTrades()` panic-ed
// 		if r := recover(); r != nil {
// 			numTimesLeft--
// 			log.Printf("Retrying programm... will try %v more times", numTimesLeft)
// 			doTradeRepeating(numTimesLeft)
// 		}
// 	}()
// 	doTrades()
// }

func setGlobals() {
	acctPtr := flag.String("a", "gReg", "jReg, gReg, gIra, mIra")
	buySellPtr := flag.String("bs", "buy", "buy sell")
	leveragePtr := flag.Bool("l", false, "use leverage?")
	sharesPtr := flag.Int64("s", 0, "shares (or 0)")
	rthPtr := flag.Bool("rth", true, "rth only?")
	executePtr := flag.Bool("x", false, "execute?")
	flag.Parse()
	fmt.Println("acct     ", *acctPtr)
	fmt.Println("buysell  ", *buySellPtr)
	fmt.Println("leverage ", *leveragePtr)
	fmt.Println("shares   ", *sharesPtr)
	fmt.Println("rth      ", *rthPtr)
	fmt.Println("execute  ", *executePtr)

	theAcct = acctNametoNumber(*acctPtr)
	theAction = *buySellPtr
	useLeverage = *leveragePtr
	outsideRTH = !*rthPtr
	doExecute = *executePtr
	argShares = *sharesPtr
}

func main() {
	setGlobals()
	doTrades()
}
