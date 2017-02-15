package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var healthCollectInterval int64 = 120
var sleepInterval int64 = 6

func main() {
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
		fmt.Println("goRoutineIdx:", goRoutineIdx)
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
		for i := 0; true; i++ {
			begin := i * 10
			end := (i + 1) * 10
			if begin >= len(unCollectedIds) {
				break
			}
			if end >= len(unCollectedIds) {
				end = len(unCollectedIds)
			}
			ids := unCollectedIds[begin:end]
			atomic.AddInt64(&goRoutineIdx, 1)
			go func() {
				defer atomic.AddInt64(&goRoutineIdx, -1)
				_, err2 := markNodeCollected(ids)
				if err2 != nil {
					log.Println("markNodeCollected failed!err:=", err.Error())
					return
				}
				for _, data := range ids {
					dts, isok, err2 := getNodeTunnel(data)
					if err2 != nil {
						log.Println("getNodeTunnel failed!err:=", err.Error())
						continue
					}
					if !isok {
						continue
					}
					isok, err := testTunnelAlive(dts)
					if err != nil {
						log.Println("testTunnelAlive failed!err:=", err.Error())
						continue
					}
					if isok {
						if strings.HasPrefix(dts.PublicUrl, "tcp") {
							dts.PublicUrl = strings.Replace(dts.PublicUrl, "tcp", "http", 1)
						}
						getNodeMetrics(dts.PublicUrl)
					}
				}
			}()
		}
	}
}
