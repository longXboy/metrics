package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/influxdb/influxdb/client"
)

func calculateCpuPercent(previousCpu uint64, previousSystem uint64, currentCpu uint64, currentSystem uint64, cpuSize int) float64 {
	cpuPercent := 0.0
	cpuDelta := float64(currentCpu - previousCpu)
	systemDelta := float64(currentSystem - previousSystem)

	if cpuDelta > 0.0 && systemDelta > 0.0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(cpuSize)
	}
	return cpuPercent
}

func getContainerLogs(waitGroup *sync.WaitGroup, dockerCli *docker.Client, c docker.APIContainers, lock *sync.Mutex, pts *[]client.Point) {
	stat := make(chan *docker.Stats)
	done := make(chan bool)
	cname := ""
	if c.Names != nil && len(c.Names) > 0 && len(c.Names[0]) > 0 {
		if []byte(c.Names[0])[0] == byte('/') {
			cname = c.Names[0][1:]
		} else {
			cname = c.Names[0]
		}
	}
	tags := map[string]string{
		"micro_service_id": c.Labels["io.daocloud.sr.microservice-id"],
		"container_name":   cname,
		"container_id":     c.ID,
	}
	var preNetRx uint64
	var preNetTx uint64
	var preSet bool
	go func() {
		for s := range stat {
			var rxBytes uint64
			var txBytes uint64
			if s.Networks != nil {
				for _, v := range s.Networks {
					rxBytes += v.RxBytes
					txBytes += v.TxBytes
				}
			}
			if !preSet {
				preNetRx = rxBytes
				preNetTx = txBytes
				preSet = true
			} else {
				preSystemCpuUsage := s.PreCPUStats.SystemCPUUsage
				systemCpuUsage := s.CPUStats.SystemCPUUsage
				preCpuUsage := s.PreCPUStats.CPUUsage
				cpuUsage := s.CPUStats.CPUUsage
				cpuSize := 0
				if cpuUsage.PercpuUsage != nil {
					cpuSize = len(cpuUsage.PercpuUsage)
				}
				cpuFields := map[string]interface{}{
					"usermode_percent":    calculateCpuPercent(preCpuUsage.UsageInUsermode, preSystemCpuUsage, cpuUsage.UsageInUsermode, systemCpuUsage, cpuSize),
					"kernelmode_percent":  calculateCpuPercent(preCpuUsage.UsageInKernelmode, preSystemCpuUsage, cpuUsage.UsageInKernelmode, systemCpuUsage, cpuSize),
					"total_percent":       calculateCpuPercent(preCpuUsage.TotalUsage, preSystemCpuUsage, cpuUsage.TotalUsage, systemCpuUsage, cpuSize),
					"total_usage":         cpuUsage.TotalUsage,
					"usage_in_usermode":   cpuUsage.UsageInUsermode,
					"usage_in_kernelmode": cpuUsage.UsageInKernelmode,
					"system_cpu_usage":    systemCpuUsage,
					"cpu_size":            cpuSize,
				}
				memFields := map[string]interface{}{
					"usage":     s.MemoryStats.Usage,
					"limit":     s.MemoryStats.Limit,
					"max_usage": s.MemoryStats.MaxUsage,
				}
				netFields := map[string]interface{}{
					"rx_bytes":     rxBytes,
					"tx_bytes":     txBytes,
					"rx_bandwidth": rxBytes - preNetRx,
					"tx_bandwidth": txBytes - preNetTx,
				}
				lock.Lock()
				*pts = append(*pts, newPoints("docker_container_cpu", tags, cpuFields))
				*pts = append(*pts, newPoints("docker_container_mem", tags, memFields))
				*pts = append(*pts, newPoints("docker_container_network", tags, netFields))
				lock.Unlock()
				done <- false
				break
			}
		}
	}()
	dockerCli.Stats(docker.StatsOptions{ID: c.ID, Stats: stat, Timeout: time.Second * 10, Done: done, Stream: true})
	waitGroup.Done()
}

func getNodeMetrics(tunnel string) {
	dockerCli, err := docker.NewClient(tunnel)
	if err != nil {
		log.Printf("new docker client(%s) failed!err:=%s", tunnel, err.Error())
		return
	}
	version, err := dockerCli.Version()
	if err != nil {
		log.Printf("docker version(%s) failed!err:=%s", tunnel, err.Error())
		return
	}
	containers, err := dockerCli.ListContainers(docker.ListContainersOptions{Filters: map[string][]string{"label": []string{"io.daocloud.sr.microservice-id"}}})
	if err != nil {
		log.Printf("list containers(%v,%v) failed!err:=%s", version.Get("Version"), version.Get("ApiVersion"), err.Error())
		return
	}
	pts := make([]client.Point, 0)
	var waitGroup sync.WaitGroup
	var lock sync.Mutex
	for _, c := range containers {
		waitGroup.Add(1)
		go getContainerLogs(&waitGroup, dockerCli, c, &lock, &pts)
		time.Sleep(time.Millisecond * 50)
	}
	waitGroup.Wait()
	if pts != nil && len(pts) > 0 {
		fmt.Printf("done!version:%v  points:%v\n", version.Get("Version"), len(pts))
	} else {
		fmt.Printf("done!tunnel:%s version:%v\n", tunnel, version.Get("Version"))
	}
	writePoints(pts)
}
