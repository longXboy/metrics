package main

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/stackimpact/stackimpact-go"
)

var healthCollectInterval int64 = 120
var sleepInterval int64 = 6

func main() {
	var production string = "development"
	if os.Getenv("PROD") == "true" || os.Getenv("PROD") == "1" {
		production = "production"
	}

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	agent := stackimpact.NewAgent()
	agent.Start(stackimpact.Options{
		AgentKey:       "d6ac427b372f9a5b2d93bb7ec47421a244112e65",
		AppName:        "sr_metrics",
		AppVersion:     "1.0.1",
		AppEnvironment: production,
	})

	var err error
	CollectInterval := os.Getenv("CollectInterval")
	if CollectInterval != "" {
		healthCollectInterval, err = strconv.ParseInt(CollectInterval, 10, 64)
		if err != nil {
			log.Fatalln("ParseInt CollectInterval failed!")
		}
	}
	Sleep := os.Getenv("Sleep")
	if Sleep != "" {
		sleepInterval, err = strconv.ParseInt(Sleep, 10, 64)
		if err != nil {
			log.Fatalln("ParseInt CollectInterval failed!")
		}
	}
	InitInflux()
	InitMysql()
	InitRedis()

	loopIdx := 0
	healthyIds, err := listConnectedNodes()
	if err != nil {
		panic(err)
	}
	var goRoutineIdx int64 = 0
	for {
		time.Sleep(time.Duration(int64(time.Second) * sleepInterval))
		fmt.Println("goRoutineIdx:", atomic.LoadInt64(&goRoutineIdx))
		loopIdx++
		if loopIdx == 20 {
			loopIdx = 0
			healthyIds, err = listConnectedNodes()
			if err != nil {
				log.Println("listConnectedNodes failed,err:=", err)
				continue
			}
		}
		unCollectedIds, err := listUnCollectedNodes(healthyIds, 200)
		if err != nil {
			log.Println("listUnCollectedNodes(200) failed,err:=", err)
			continue
		}
		fmt.Println("healthy:", len(healthyIds), "  uncollected:", len(unCollectedIds))
		if unCollectedIds == nil || len(unCollectedIds) == 0 {
			continue
		}
		_, err = markNodeCollected(unCollectedIds)
		if err != nil {
			log.Println("markNodeCollected failed!err:=", err.Error())
			continue
		}
		for i := range unCollectedIds {
			go testAndLogNode(unCollectedIds[i], &goRoutineIdx)
			time.Sleep(time.Millisecond * 50)
		}

	}
}

func testAndLogNode(id string, idx *int64) {
	atomic.AddInt64(idx, 1)
	defer atomic.AddInt64(idx, -1)
	dts, isok, err := getNodeTunnel(id)
	if err != nil {
		log.Println("getNodeTunnel failed!err:=", err.Error())
		return
	}
	if !isok {
		return
	}
	isok, err = testTunnelAlive(dts)
	if err != nil {
		log.Println("testTunnelAlive failed!err:=", err.Error())
		return
	}
	if isok {
		if strings.HasPrefix(dts.PublicUrl, "tcp") {
			dts.PublicUrl = strings.Replace(dts.PublicUrl, "tcp", "http", 1)
		}
		getMachineMetrics(dts.PublicUrl)
	}
}
