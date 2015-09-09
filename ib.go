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
var buy = "BUY"
var sell = "SELL"
var orderType string
var tif string
var FAMethod string
var FAPercentage string
var FAGroup string
var shares int64
var myEngine *ib.Engine
var theAcct string
var useLeverage bool
var argShares int64
var theAction string
var outsideRTH bool
var argPrice float64

//var argShares string
var nextOrderID int64

//var rc = make(chan ib.Reply)
var rc chan ib.Reply = make(chan ib.Reply)

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

func getNextOrderID(mgr IBManager) int64 {
	for {
		r := <-rc
		switch r.(type) {
		case (*ib.NextValidID):
			r := r.(*ib.NextValidID)
			return (r.OrderID)
		default:
			fmt.Println(r)
		}
	}
}
func getNextOrderIDNew(mgr IBManager) chan int64 {
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
	case res := <-getNextOrderIDNew(mgr):
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

func getEngine() *ib.Engine {

	var err error
	myEngine, err = ib.NewEngine(ib.EngineOptions{})
	if err != nil {
		log.Fatalf("error creating %s Engine ", err)
	}
	defer myEngine.Stop()
	if myEngine.State() != ib.EngineReady {
		log.Fatalf("engine is not ready")
	}
	return (myEngine)
}
func getAccountManager(*ib.Engine) *ib.AdvisorAccountManager {
	var err error

	myAccountManager, err := ib.NewAdvisorAccountManager(myEngine)
	if err != nil {
		panic(err)
	}
	<-myAccountManager.Refresh()
	defer myAccountManager.Close()
	return (myAccountManager)
}
func calculateShares(myAccountManager *ib.AdvisorAccountManager) int64 {
	var err error
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
	return (shares)
}

func doTrade(mgr IBManager, nextOrderID int64, theAction string, ticker string, shares int64, price float64, theAcct string, tif string, orderType string, FAMethod string, FAPercentage string, FAGroup string, outsideRTH bool, doExecute bool) {
	//	fmt.Println("quote", aQuote, "slipped-", quoteSlipped, "shares", shares)
	symbol := "AAPL"
	request := ib.PlaceOrder{Contract: NewContract(symbol)}
	request.Order, _ = NewOrder()
	request.Order.Action = theAction
	request.Order.TIF = tif
	request.Order.OrderType = orderType
	request.Order.LimitPrice = 0
	if theAction == "SELL" {
		request.Order.FAMethod = FAMethod
		request.Order.FAPercentage = FAPercentage
		request.Order.FAGroup = FAGroup
		request.Order.FAProfile = ""
		request.Order.Account = ""
	} else {
		request.Order.Account = theAcct
		if shares < 20 {
			doExecute = false
		}
	}

	request.SetID(nextOrderID)
	fmt.Printf("%s %t %s %s%% at %s, using %s for %f %s %s \n", request.Order.Account, doExecute, request.Order.Action, request.Order.FAPercentage, request.Order.TIF, request.Order.OrderType, request.Order.LimitPrice, request.Order.FAMethod, request.Order.FAGroup)

	if doExecute {
		mgr.engine.Send(&request)
	}
}

func setGlobals() {
	acctPtr := flag.String("a", "gReg", "jReg, gReg, gIra, mIra")
	buySellPtr := flag.String("bs", "BUY", "BUY SELL")
	pricePtr := flag.Float64("price", 0, "limit price (or 0)")
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
	argPrice = *pricePtr

	if theAction == "buy" {
		theAction = "BUY"
		orderType = "LOC"
		tif = "DAY"
	} else {
		theAction = "SELL"
		orderType = "MARKET"
		tif = "OPG"
		FAMethod = "PctChange"
		FAPercentage = "-100"
		FAGroup = "everyone"
	}
}

func main() {
	setGlobals()
	myEngine, err := ib.NewEngine(ib.EngineOptions{})
	if err != nil {
		panic(err)
	}

	myAccountManager, err := ib.NewAdvisorAccountManager(myEngine)
	if err != nil {
		panic(err)
	}
	<-myAccountManager.Refresh()
	defer myAccountManager.Close()

	//	myEngine := getEngine()
	mgr := IBManager{engine: myEngine}
	mgr.engine.SubscribeAll(rc)
	mgr.engine.Send(&ib.RequestIDs{})

	nextOrderID = getNextOrderID(mgr)
	shares := calculateShares(myAccountManager)
	doTrade(mgr, nextOrderID, theAction, ticker, shares, argPrice, theAcct, tif, orderType, FAMethod, FAPercentage, FAGroup, outsideRTH, doExecute)
}
