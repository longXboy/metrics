package main

import (
	"log"
	"net/url"
	"os"
	"time"

	"github.com/influxdb/influxdb/client"
)

var influxCli *client.Client

const (
	InfluxUser = "root"
	InfluxPwd  = "root"
)

var InfluxDb string

func InitInflux() {
	InfluxAddr := os.Getenv("InfluxAddr")
	if InfluxAddr == "" {
		InfluxAddr = "http://192.168.1.28:8086"
	}
	InfluxDb = os.Getenv("InfluxDB")
	if InfluxDb == "" {
		InfluxDb = "daosr"
	}
	u, err := url.Parse(InfluxAddr)
	if err != nil {
		panic(err)
	}

	conf := client.Config{
		URL:      *u,
		Username: InfluxUser,
		Password: InfluxPwd,
	}

	influxCli, err = client.NewClient(conf)
	if err != nil {
		panic(err)
	}

	dur, ver, err := influxCli.Ping()
	if err != nil {
		panic(err)
	}
	log.Printf("ping influx success! %v, %s", dur, ver)

}
func writePoints(pts []client.Point) error {
	bps := client.BatchPoints{
		Points:          pts,
		Database:        InfluxDb,
		RetentionPolicy: "default",
	}
	_, err := influxCli.Write(bps)
	if err != nil {
		return err
	}
	return nil
}

func newPoints(measurement string, tags map[string]string, fields map[string]interface{}) client.Point {
	pt := client.Point{Name: measurement,
		Tags:      tags,
		Fields:    fields,
		Time:      time.Now(),
		Precision: "n"}
	return pt
}
