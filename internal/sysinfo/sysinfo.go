// Package sysinfo reports best-effort hardware details (CPU model, memory
// type, disk model, network interface) used to annotate benchmark results.
// Every field is populated on a best-effort basis: platforms or permission
// levels that can't provide a value leave it empty/zero rather than
// failing the whole benchmark over missing context.
package sysinfo

import (
	"net"
	"runtime"
)

// CPU describes the host processor.
type CPU struct {
	Model         string
	PhysicalCores int
	LogicalCores  int
}

// Memory describes installed RAM.
type Memory struct {
	TotalBytes uint64
	Type       string // e.g. "DDR4"; empty if unknown
	SpeedMHz   int    // 0 if unknown
}

// Disk describes the storage backing a path.
type Disk struct {
	Device     string
	Model      string
	Filesystem string
	TotalBytes uint64
	FreeBytes  uint64
}

// NetInterface describes the primary outbound network interface.
type NetInterface struct {
	Name     string
	MAC      string
	IPs      []string
	LinkMbps int // 0 if unknown
}

// GetCPU reports the host processor's model and core counts.
func GetCPU() CPU {
	c := CPU{LogicalCores: runtime.NumCPU()}
	fillCPU(&c)
	return c
}

// GetMemory reports installed RAM.
func GetMemory() Memory {
	var m Memory
	fillMemory(&m)
	return m
}

// GetDisk reports the storage device backing path.
func GetDisk(path string) Disk {
	var d Disk
	fillDisk(&d, path)
	return d
}

// GetNetInterface reports the primary (first up, non-loopback) network
// interface, regardless of which interface any particular benchmark run
// actually used.
func GetNetInterface() NetInterface {
	var n NetInterface
	if iface := primaryInterface(); iface != nil {
		n.Name = iface.Name
		if len(iface.HardwareAddr) > 0 {
			n.MAC = iface.HardwareAddr.String()
		}
		if addrs, err := iface.Addrs(); err == nil {
			for _, a := range addrs {
				if ipNet, ok := a.(*net.IPNet); ok {
					n.IPs = append(n.IPs, ipNet.IP.String())
				}
			}
		}
	}
	fillNetLinkSpeed(&n)
	return n
}

func primaryInterface() *net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for i := range ifaces {
		f := ifaces[i].Flags
		if f&net.FlagUp == 0 || f&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifaces[i].Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		return &ifaces[i]
	}
	return nil
}
