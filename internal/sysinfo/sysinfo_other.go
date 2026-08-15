//go:build !linux && !darwin && !windows

package sysinfo

func fillCPU(c *CPU)                   { c.PhysicalCores = c.LogicalCores }
func fillMemory(m *Memory)             {}
func fillDisk(d *Disk, _ string)       {}
func fillNetLinkSpeed(n *NetInterface) {}
