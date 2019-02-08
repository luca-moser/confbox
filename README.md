# ConfBox [![Build Status](https://travis-ci.org/luca-moser/confbox.svg?branch=master)](https://travis-ci.org/luca-moser/confbox)

ConfBox monitors the overall confirmation rate of the IOTA Tangle network.
It provides a single HTTP endpoint from which the currently measured confirmation rate can be retrieved.

You can access currently online ConfBoxes under: 
* mainnet: http://88.99.60.78:15265, http://159.69.9.6:15265.
* devnet: http://88.99.60.78:15266, http://159.69.9.6:15266.

The HTTP response looks like this:
```json
{
    "results": {
        "avg_5": 0.57,
        "avg_10": 0.79,
        "avg_15": 0.83,
        "avg_30": 0.76
    },
    "config": {
        "mwm": 14,
        "gtta_depth": 3,
        "transfer_polling": {
            "interval": 10
        },
        "promote_reattach": {
            "enabled": false,
            "interval": 30
        }
    }
}
```

If ConfBox did not gather enough data yet, some `results` will show `-1`.

## Install your own ConfBox using docker
Assuming we are running on a linux box.

1. install [docker](https://docs.docker.com/install/linux/docker-ce/ubuntu/) and [docker-compose](https://docs.docker.com/compose/install/).
2. create a new dir for confbox
3. copy the `docker-compose.yml`, `config.json` and `confbox` from the repository to your folder.
4. `chmod +x confbox`  
5. replace the `REPLACE_ME` placeholders inside `config.json` with your own nodes.
you can pick multiple nodes from [iota.dance](http://iota.dance) to put in the `nodes` array.
6. start the confbox with `./confbox start`

Your ConfBox is now up and running under `http://your-address:15265`.

## How it works
- A buffer with space for 30 minutes worth of measurement data is allocated.
- Each minute a batch of 5 zero value transactions is issued.
The transactions are broadcasted to each defined node in the config to increase the chance of propagation.
- A transfer poller checks which transactions got confirmed and marks them. 
- Up on request, ConfBox computes the avg. 5min/10min/15min/30min conf. rate given the measurement data. 

## Config
- `listen`: the address and port to listen to
- `debug`: enable debug log
- `local_pow`: whether to do PoW locally
- `result_log_interval`: interval (minutes) to use to log the current measurements onto the console
- `mwm`: minimum weight magnitude used for PoW
- `gtta_depth`: `getTransactionsToApprove` depth
- `transfer_polling.interval`: interval (seconds) to use to check for confirmed transactions
- `promote_reattach.enabled`: whether to promote/reattach transactions
- `promote_reattach.interval`: interval (seconds) to use to promote/reattach pending transactions
- `quorum.primary_node`: primary node to use for IRI API calls
- `quorum.nodes`: nodes to use for quorum IRI API calls (mainly used to check whether a transactions got confirmed)
- `quorum.nodes`: nodes to use for quorum IRI API calls (mainly used to check whether a transactions got confirmed)
- `quorum.max_subtangle_milestone_delta`: max. allowed delta between the defined nodes' latest solid subtangle milestone
- `quorum.timeout`: timeout (seconds) for IRI API calls
- `quorum.threshold`: threshold for the quorums; 0.66 means 2/3 of nodes must have the same response
- `quorum.no_response_tolerance`: how many nodes are tolerated to not give a response

Sample config:
```
{
  "listen": "127.0.0.1:9090",
  "debug": false,
  "local_pow": true,
  "result_log_interval": 5,
  "mwm": 14,
  "gtta_depth": 3,
  "transfer_polling": {
    "interval": 10
  },
  "promote_reattach": {
    "enabled": false,
    "interval": 30
  },
  "quorum": {
    "primary_node": "https://<primary-node>:14265",
    "nodes": [
      "https://<primary-node>:14265",
      "https://<secondary-node>:14265",
      ...
    ],
    "max_subtangle_milestone_delta": 1,
    "timeout": 15,
    "threshold": 0.66,
    "no_response_tolerance": 0.2
  }
}
```

## Usage with the account package

Create a new `SendOracle` and use a `ConfBoxDecider` as an `OracleSource`:
```go
func main() {
    ...
    
    timeDecider := oracle_time.NewTimeDecider(ntpClock, time.Duration(1)*time.Hour)
    confRateDecider := oracle_confbox.NewConfBoxDecider("<confbox-url>", ntpClock, 0.70, oracle_confbox.AvgMode10Min)
    sendOracle := oracle.New(timeDecider, confRateDecider)
    ...
}
```

You can instantiate multiple `ConfBoxDecider`s pointing to different ConfBoxes, to gain an even higher confidence.