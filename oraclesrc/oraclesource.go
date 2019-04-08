package oraclesrc

import (
	"encoding/json"
	"fmt"
	"github.com/iotaledger/iota.go/account/deposit"
	"github.com/iotaledger/iota.go/account/timesrc"
	"github.com/luca-moser/confbox/models"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
)

// ErrNonOkHttpStatusCode is returned when the connected ConfBox sends a non ok HTTP status code.
var ErrNonOkHttpStatusCode = errors.New("non ok http status code from ConfBox")
// ErrConfBoxNotReady is returned when the connected ConfBox did not yet compute the conf. rate for
// the selected avg. mode.
var ErrConfBoxNotReady = errors.New("connected ConfBox isn't ready yet")

// The avg result to use for the ConfBoxDecider.
type AvgMode byte

// available avg. modes on a ConfBox.
const (
	AvgMode5Min AvgMode = iota
	AvgMode10Min
	AvgMode15Min
	AvgMode30Min
)

// DefaultConfBoxDecider creates a new ConfBoxDecider with an average confirmation rate threshold
// of 65% in the last 10 minutes.
func DefaultConfBoxDecider(confBoxURL string, timeSource timesrc.TimeSource) *ConfBoxDecider {
	return &ConfBoxDecider{confBoxURL, timeSource, 0.65, AvgMode10Min}
}

// NewConfBoxDecider creates a new ConfBox decider with the given threshold and mode.
func NewConfBoxDecider(confBoxURL string, timeSource timesrc.TimeSource, threshold float64, mode AvgMode) *ConfBoxDecider {
	return &ConfBoxDecider{confBoxURL, timeSource, threshold, mode}
}

// ConfBoxDecider is an OracleSource which given the current average confirmation rate
// of the network, decides whether a transaction should be sent or not (given the set threshold).
// ConfBoxDecider connects to a ConfBox to retrieve the current average confirmation rates.
// If the delta between the current time and the CDR's timeout is below the selected avg. mode,
// then the ConfBoxDecider will decide to not send the transaction either.
type ConfBoxDecider struct {
	url        string
	timeSource timesrc.TimeSource
	threshold  float64
	mode       AvgMode
}

const dateFormat = "2006-02-01 15:04:05"
const notAvailMsg = "avg. conf. rate %d min not available yet"
const belowThreshold = "current conf. rate of %.2f (avg. %d min) is below set threshold of %.2f"
const timeDeltaBelowAvg = "time delta between now (%s) and the CDR timeout (%s) is below the selected avg. conf. rate mode (%d min). (delta %s)"

var modeToMinMap = map[AvgMode]int{
	AvgMode5Min:  5,
	AvgMode10Min: 10,
	AvgMode15Min: 15,
	AvgMode30Min: 30,
}

func (cbd *ConfBoxDecider) Ok(conds *deposit.CDA) (bool, string, error) {
	res, err := http.Get(cbd.url)
	if err != nil {
		return false, "", errors.Wrapf(err, "unable to query ConfBox at %s", cbd.url)
	}

	if res.StatusCode != http.StatusOK {
		return false, "", ErrNonOkHttpStatusCode
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return false, "", errors.Wrapf(err, "unable to read body")
	}

	confBoxRes := &models.Response{}
	if err := json.Unmarshal(body, confBoxRes); err != nil {
		return false, "", errors.Wrap(err, "unable to parse JSON body")
	}

	now, err := cbd.timeSource.Time()
	if err != nil {
		return false, "", errors.Wrap(err, "unable to retrieve time from time source")
	}

	timeout := *conds.TimeoutAt
	timeDelta := timeout.Sub(now)

	var currentConfRate float64
	switch (cbd.mode) {
	case AvgMode5Min:
		currentConfRate = confBoxRes.Results.Avg5
	case AvgMode10Min:
		currentConfRate = confBoxRes.Results.Avg10
	case AvgMode15Min:
		currentConfRate = confBoxRes.Results.Avg15
	case AvgMode30Min:
		currentConfRate = confBoxRes.Results.Avg30
	}

	if currentConfRate == -1 {
		return false, "", errors.Wrap(ErrConfBoxNotReady, fmt.Sprintf(notAvailMsg, modeToMinMap[cbd.mode]))
	}
	if currentConfRate < cbd.threshold {
		return false, fmt.Sprintf(belowThreshold, currentConfRate, modeToMinMap[cbd.mode], cbd.threshold), nil
	}
	if timeDelta.Minutes() < float64(modeToMinMap[cbd.mode]) {
		return false, fmt.Sprintf(timeDeltaBelowAvg, now.Format(dateFormat), timeout.Format(dateFormat), modeToMinMap[cbd.mode], timeDelta.String()), nil
	}

	return true, "", nil
}
