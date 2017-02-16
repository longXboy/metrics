package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	dclient "github.com/docker/docker/client"
	"github.com/fsouza/go-dockerclient"
	"github.com/influxdb/influxdb/client"
	"golang.org/x/net/context"
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

func getMachineMetrics(tunnel string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()
	cli, apiVersion, err := newClient(ctx, tunnel)
	if err != nil {
		log.Printf("new docker client(%s,%s) failed!err:=%v\n", tunnel, apiVersion, err)
		return
	}
	defer cli.Close()
	containers, err := listContainers(ctx, cli)
	if err != nil {
		log.Printf("list containers(%s,%s) failed!err:=%v\n", tunnel, apiVersion, err)
		return
	}
	pts := make([]client.Point, 0)
	for _, c := range containers {
		err = getContainerMetrics(ctx, cli, c, &pts)
		if err != nil {
			log.Printf("get container metrics(%s,%s) failed!err:=%v\n", tunnel, apiVersion, err)
			break
		}
	}
	if len(pts) > 0 {
		err = writePoints(pts)
		if err != nil {
			log.Printf("write points (%d,%s,%s) failed!err:=%v", len(pts), tunnel, apiVersion, err)
			return
		}
		log.Printf("write points! pts:%d", len(pts))
	}
}

func newClient(ctx context.Context, tunnel string) (*dclient.Client, string, error) {
	var cli *dclient.Client
	var err error
	cli, err = dclient.NewClient(tunnel, "1.17", nil, nil)
	if err != nil {
		cli.Close()
		log.Printf("dclient.NewClient (%s) failed!err:=%v\n", tunnel, err)
		return nil, "", err
	}
	v, err := cli.ServerVersion(ctx)
	if err != nil {
		cli.Close()
		log.Printf("cli.ServerVersion (%s) failed!err:=%v\n", tunnel, err)
		return nil, "", err
	}
	if v.APIVersion == "" {
		cli.Close()
		log.Printf("APIVersion is empty(%s)\n", tunnel, err)
		return nil, "", fmt.Errorf("apiversion invalid")
	}
	if v.APIVersion != "1.17" {
		cli.Close()
		cli, err = dclient.NewClient(tunnel, v.APIVersion, nil, nil)
		if err != nil {
			cli.Close()
			log.Printf("dclient.NewClient (%s,%s) failed!err:=%v\n", tunnel, v.APIVersion, err)
			return nil, "", err
		}
	}
	return cli, v.APIVersion, nil
}

func listContainers(ctx context.Context, cli *dclient.Client) ([]types.Container, error) {
	arg := filters.NewArgs()
	arg.Add("label", "io.daocloud.sr.microservice-id")
	opt := types.ContainerListOptions{Filters: arg}
	containers, err := cli.ContainerList(ctx, opt)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func getContainerMetrics(ctx context.Context, cli *dclient.Client, c types.Container, pts *[]client.Point) error {
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
	cs, err := cli.ContainerStats(ctx, c.ID, true)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(cs.Body)
	for {
		s := new(docker.Stats)
		err := decoder.Decode(s)
		if err != nil {
			return err
		}
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
			*pts = append(*pts, newPoints("docker_container_cpu", tags, cpuFields))
			*pts = append(*pts, newPoints("docker_container_mem", tags, memFields))
			*pts = append(*pts, newPoints("docker_container_network", tags, netFields))
			return nil
		}
	}
}
