package main

import (
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/go-xorm/xorm"
)

type Daomonit struct {
	DaomonitId  string
	IsLogin     bool
	HeartbeatAt time.Time `xorm:"TIMESTAMP"`
}

type DaomonitTunnelStat struct {
	PublicUrl     string
	Server        string
	LocalAddr     string
	DaomonitId    string
	EstablishedAt time.Time `xorm:"TIMESTAMP"`
	IsEstablished bool
}

type Node struct {
	NodeId          string
	HealthCollectAt time.Time `xorm:"TIMESTAMP"`
}

var DaoKeeperDB *xorm.Engine
var DaoSRDB *xorm.Engine

const (
	connstr = "root:dangerous@tcp(192.168.1.25:3306)/daokeeper?charset=utf8"
)

func InitMysql() {
	DaoKeeperConnStr := os.Getenv("DAOKEEPER")
	if DaoKeeperConnStr == "" {
		DaoKeeperConnStr = "root:dangerous@tcp(192.168.1.25:3306)/daokeeper?charset=utf8"
	}
	DaoSrConnStr := os.Getenv("DAOSR")
	if DaoSrConnStr == "" {
		DaoSrConnStr = "root:dangerous@tcp(192.168.1.25:3306)/daokeeper?charset=utf8"
	}
	var err error
	DaoKeeperDB, err = xorm.NewEngine("mysql", DaoKeeperConnStr)
	if err != nil {
		panic(err)
	}
	err = DaoKeeperDB.Ping()
	if err != nil {
		panic(err)
	}
	DaoSRDB, err = xorm.NewEngine("mysql", DaoSrConnStr)
	if err != nil {
		panic(err)
	}
	err = DaoSRDB.Ping()
	if err != nil {
		panic(err)
	}
}

func getNodeTunnel(id string) (DaomonitTunnelStat, bool, error) {
	var tunnels []DaomonitTunnelStat = make([]DaomonitTunnelStat, 0)
	err := DaoKeeperDB.Where("daomonit_id=?", id).And("is_established=?", true).And("local_addr=?", "unix:///var/run/docker.sock").Desc("established_at").Limit(1).Find(&tunnels)
	if err != nil {
		return DaomonitTunnelStat{}, false, nil
	}
	if tunnels == nil || len(tunnels) == 0 {
		return DaomonitTunnelStat{}, false, nil
	}
	return tunnels[0], true, nil
}

func markNodeCollected(ids []string) (int64, error) {
	var node Node
	node.HealthCollectAt = time.Now()
	return DaoSRDB.In("node_id", ids).Cols("health_collect_at").Update(&node)
}

func listUnCollectedNodes(ids []string, limit int) ([]string, error) {
	if ids == nil || len(ids) == 0 {
		return []string{}, nil
	}
	var nodes []Node
	err := DaoSRDB.In("node_id", ids).And(fmt.Sprintf("(health_collect_at<(NOW() - INTERVAL %d SECOND) or health_collect_at is null)", healthCollectInterval)).Asc("health_collect_at").Limit(limit).Find(&nodes)
	if err != nil {
		return []string{}, err
	}
	var idsFilters []string = make([]string, 0)
	for i := range nodes {
		idsFilters = append(idsFilters, nodes[i].NodeId)
	}
	return idsFilters, nil
}

func listConnectedNodes() ([]string, error) {
	var daomonits []Daomonit = make([]Daomonit, 0)
	err := DaoKeeperDB.Where("is_login=?", true).And("heartbeat_at>(NOW() - INTERVAL 25 SECOND)").Find(&daomonits)
	if err != nil {
		return []string{}, err
	}
	var ids []string = make([]string, 0)
	for i := range daomonits {
		ids = append(ids, daomonits[i].DaomonitId)
	}
	return ids, nil
}
