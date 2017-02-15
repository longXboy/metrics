package main

import (
	"fmt"
	"os"

	"gopkg.in/redis.v5"
)

var redisCli *redis.Client

func InitRedis() {
	redisAddr := os.Getenv("REDIS")
	if redisAddr == "" {
		redisAddr = "192.168.1.26:6379"
	}
	redisCli = redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	pong, err := redisCli.Ping().Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("redis pong:", pong, err)
}

func testTunnelAlive(tunnel DaomonitTunnelStat) (bool, error) {
	cmd := redisCli.SIsMember(fmt.Sprintf("ngrok.%s", tunnel.Server), tunnel.PublicUrl)
	return cmd.Result()
}
