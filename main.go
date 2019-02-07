package main

import (
	"container/ring"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/Mandala/go-log"
	"github.com/iotaledger/iota.go/account"
	"github.com/iotaledger/iota.go/account/builder"
	"github.com/iotaledger/iota.go/account/event"
	"github.com/iotaledger/iota.go/account/event/listener"
	"github.com/iotaledger/iota.go/account/plugins/promoter"
	"github.com/iotaledger/iota.go/account/plugins/transfer/poller"
	"github.com/iotaledger/iota.go/account/store/inmemory"
	"github.com/iotaledger/iota.go/account/timesrc"
	"github.com/iotaledger/iota.go/api"
	"github.com/iotaledger/iota.go/checksum"
	"github.com/iotaledger/iota.go/consts"
	"github.com/iotaledger/iota.go/converter"
	"github.com/iotaledger/iota.go/pow"
	. "github.com/iotaledger/iota.go/trinary"
	"github.com/labstack/echo"
	"github.com/luca-moser/confbox/quorum"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

const configFile = "config.json"

var logger *log.Logger

func main() {
	var acc account.Account

	conf := readConfig()

	logger = log.New(os.Stdout)
	if conf.Debug {
		logger = logger.WithDebug()
	}

	// compose API
	httpClient := &http.Client{Timeout: time.Duration(conf.Quorum.Timeout) * time.Second}
	apiSettings := quorum.QuorumHTTPClientSettings{
		PrimaryNode:                &conf.Quorum.PrimaryNode,
		Threshold:                  conf.Quorum.Threshold,
		NoResponseTolerance:        conf.Quorum.NoResponseTolerance,
		Client:                     httpClient,
		Nodes:                      conf.Quorum.Nodes,
		MaxSubtangleMilestoneDelta: conf.Quorum.MaxSubtangleMilestoneDelta,
		ForceQuorumSend: map[api.IRICommand]struct{}{
			api.BroadcastTransactionsCmd: {},
		},
	}
	if conf.LocalPow {
		_, powFunc := pow.GetFastestProofOfWorkImpl()
		apiSettings.LocalProofOfWorkFunc = powFunc
	}
	iotaAPI, err := api.ComposeAPI(apiSettings, quorum.NewQuorumHTTPClient)
	must(err)

	// init account
	em := event.NewEventMachine()

	dataStore := inmemory.NewInMemoryStore()

	// create a poller which will check for incoming transfers
	receiveEventFilter := poller.NewPerTailReceiveEventFilter(true)
	transferPoller := poller.NewTransferPoller(
		iotaAPI, dataStore, em, account.NewInMemorySeedProvider(strings.Repeat("9", 81)),
		receiveEventFilter, time.Duration(conf.TransferPolling.Interval)*time.Second,
	)

	// build the account object
	b := builder.NewBuilder().
		WithAPI(iotaAPI).
		WithStore(dataStore).
		WithMWM(conf.MWM).
		WithDepth(conf.GTTADepth).
		WithPlugins(transferPoller)

	// create a promoter/reattacher which takes care of trying to get
	// pending transfers to confirm.
	if conf.PromoteReattach.Enabled {
		b.WithPlugins(promoter.NewPromoter(
			iotaAPI, dataStore, em, &timesrc.SystemClock{},
			time.Duration(conf.PromoteReattach.Interval)*time.Second,
			conf.GTTADepth, conf.MWM))
	}
	acc, err = b.WithEvents(em).Build()
	must(err)
	must(acc.Start())
	defer acc.Shutdown()

	getResult := make(chan struct{})
	backResult := make(chan ConfRate)
	go measure(em, getResult, backResult)

	// send off a bundle each minute
	addr := randAddr()
	logger.Infof("will use %s as destination address", addr)
	var counter int
	go func() {
		ticker := time.NewTicker(time.Duration(1) * time.Minute)
		for {
			msg, _ := converter.ASCIIToTrytes(fmt.Sprintf("conf box tx: %d", counter))
			for i := 0; i < txPerPoint; i++ {
				acc.Send(account.Recipient{Address: addr, Tag: "CONFBOX", Message: msg})
			}
			logger.Debugf("sent off %d txs", txPerPoint)
			<-ticker.C
		}
	}()

	go func() {
		ticker := time.NewTicker(time.Duration(conf.ResultLogInterval) * time.Minute)
		for {
			getResult <- struct{}{}
			result := <-backResult
			logger.Infof("5: %.2f, 10: %.2f, 15: %.2f, 30: %.2f (points: %d)", result.Avg5, result.Avg10, result.Avg15, result.Avg30, pointsFilled)
			<-ticker.C
		}
	}()

	e := echo.New()
	e.HideBanner = true
	e.GET("/", func(c echo.Context) error {
		getResult <- struct{}{}
		result := <-backResult
		res := Response{Config: conf.info, Results: result}
		return c.JSON(http.StatusOK, res)
	})
	must(e.Start(conf.Listen))
}

const txPerPoint = 5
const retentionPolicy = 31

var sizes = [4]int{5, 5, 5, 15}

var points = ring.New(retentionPolicy)
var pointsFilled = 0

type ConfRate struct {
	Avg5  float64 `json:"avg_5"`
	Avg10 float64 `json:"avg_10"`
	Avg15 float64 `json:"avg_15"`
	Avg30 float64 `json:"avg_30"`
}

type Response struct {
	Results ConfRate `json:"results"`
	Config  info     `json:"config"`
}

type bucket struct {
	ok        bool
	size      float64
	confirmed float64
}

func (b *bucket) rate() float64 {
	if !b.ok {
		return -1
	}
	return math.Floor((b.confirmed/b.size)*100) / 100
}

func measure(em event.EventMachine, getResult chan struct{}, backResult chan ConfRate) {
	lis := listener.NewChannelEventListener(em).RegConfirmedTransfers().RegSentTransfers()

	gathered := 0

	for {
		select {
		case e := <-lis.SentTransfer:
			logger.Debugf("got sent transfer event %s", e[0].Hash)
			m, ok := points.Value.(map[Hash]bool)
			// either never used or we have looped in the ring buffer
			if !ok || points.Value == nil || (len(m) > 0 && gathered == 0) {
				m = map[Hash]bool{}
			}
			m[e[0].Hash] = false
			points.Value = m
			gathered++
			// gathered all tx for this minute, lets forward to the next
			if gathered == txPerPoint {
				pointsFilled++
				logger.Debugf("filled point with %d txs (points filled: %d)", txPerPoint, pointsFilled)
				gathered = 0
				points = points.Next()
			}
		case e := <-lis.TransferConfirmed:
			logger.Debugf("got transfer confirmed event %s", e[0].Hash)
			hash := e[0].Hash
			// traverse the ring buffer and set the confirmed flag accordingly
			r := points
			for i := 0; i < retentionPolicy; i++ {
				m, ok := r.Value.(map[Hash]bool)
				if !ok || m == nil {
					r = r.Prev()
					continue
				}
				if _, has := m[hash]; has {
					logger.Debugf("set tx to be confirmed")
					m[hash] = true
					break
				}
				r = r.Prev()
			}
		case <-getResult:
			r := points.Prev()

			computeBucket := func(size int, b bucket) bucket {
				for i := 0; i < size; i++ {
					m, ok := r.Value.(map[Hash]bool)
					if !ok {
						return b
					}
					for _, v := range m {
						b.size++
						if v {
							b.confirmed++
						}
					}
					r = r.Prev()
				}
				b.ok = true
				return b
			}

			buckets := make([]bucket, len(sizes))
			for i, size := range sizes {
				buckets[i] = computeBucket(size, buckets[i])
				if !buckets[i].ok {
					break
				}
				if i != len(sizes)-1 {
					cpy := buckets[i]
					cpy.ok = false
					buckets[i+1] = cpy
				}
			}
			cr := ConfRate{}
			cr.Avg5 = buckets[0].rate()
			cr.Avg10 = buckets[1].rate()
			cr.Avg15 = buckets[2].rate()
			cr.Avg30 = buckets[3].rate()
			backResult <- cr
		}
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func randAddr() Hash {
	var hash string
	num := make([]byte, 81)
	_, err := rand.Read(num)
	must(err)
	for i := 0; i < 81; i++ {
		hash += string(consts.TryteAlphabet[num[i]%byte(len(consts.TryteAlphabet))])
	}
	hash, _ = checksum.AddChecksum(hash, true, consts.AddressChecksumTrytesSize)
	return hash
}

type info struct {
	MWM             uint64 `json:"mwm"`
	GTTADepth       uint64 `json:"gtta_depth"`
	TransferPolling struct {
		Interval uint64 `json:"interval"`
	} `json:"transfer_polling"`
	PromoteReattach struct {
		Enabled  bool   `json:"enabled"`
		Interval uint64 `json:"interval"`
	} `json:"promote_reattach"`
}

type config struct {
	info
	Listen            string `json:"listen"`
	LocalPow          bool   `json:"local_pow"`
	Debug             bool   `json:"debug"`
	ResultLogInterval uint64 `json:"result_log_interval"`
	Quorum            struct {
		PrimaryNode                string   `json:"primary_node"`
		Nodes                      []string `json:"nodes"`
		Threshold                  float64  `json:"threshold"`
		NoResponseTolerance        float64  `json:"no_response_tolerance"`
		MaxSubtangleMilestoneDelta uint64   `json:"max_subtangle_milestone_delta"`
		Timeout                    uint64   `json:"timeout"`
	} `json:"quorum"`
}

func readConfig() *config {
	configBytes, err := ioutil.ReadFile(configFile)
	must(err)

	config := &config{}
	must(json.Unmarshal(configBytes, config))
	return config
}
