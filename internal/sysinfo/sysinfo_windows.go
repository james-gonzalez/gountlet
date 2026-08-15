//go:build windows

package sysinfo

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func psOutput(script string) string {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func fillCPU(c *CPU) {
	c.Model = psOutput("(Get-CimInstance Win32_Processor | Select-Object -First 1).Name")
	if v, err := strconv.Atoi(psOutput("(Get-CimInstance Win32_Processor | Select-Object -First 1).NumberOfCores")); err == nil {
		c.PhysicalCores = v
	} else {
		c.PhysicalCores = c.LogicalCores
	}
}

func fillMemory(m *Memory) {
	if v, err := strconv.ParseUint(psOutput("(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory"), 10, 64); err == nil {
		m.TotalBytes = v
	}
	if v, err := strconv.Atoi(psOutput("(Get-CimInstance Win32_PhysicalMemory | Select-Object -First 1).Speed")); err == nil {
		m.SpeedMHz = v
	}
	m.Type = ddrTypeFromSMBIOS(psOutput("(Get-CimInstance Win32_PhysicalMemory | Select-Object -First 1).SMBIOSMemoryType"))
}

// ddrTypeFromSMBIOS maps the documented Win32_PhysicalMemory SMBIOSMemoryType
// codes for the DDR generations gountlet is likely to see.
func ddrTypeFromSMBIOS(code string) string {
	switch code {
	case "20":
		return "DDR"
	case "21":
		return "DDR2"
	case "24":
		return "DDR3"
	case "26":
		return "DDR4"
	case "34":
		return "DDR5"
	default:
		return ""
	}
}

func fillDisk(d *Disk, path string) {
	drive := strings.TrimSuffix(filepath.VolumeName(path), ":")
	if drive == "" {
		return
	}
	d.Device = drive + ":"
	d.Filesystem = psOutput("(Get-Volume -DriveLetter " + drive + ").FileSystem")
	if v, err := strconv.ParseUint(psOutput("(Get-Volume -DriveLetter "+drive+").Size"), 10, 64); err == nil {
		d.TotalBytes = v
	}
	if v, err := strconv.ParseUint(psOutput("(Get-Volume -DriveLetter "+drive+").SizeRemaining"), 10, 64); err == nil {
		d.FreeBytes = v
	}
	d.Model = psOutput("(Get-Partition -DriveLetter " + drive + " | Get-Disk).FriendlyName")
}

func fillNetLinkSpeed(n *NetInterface) {
	speed := psOutput("(Get-NetAdapter | Where-Object Status -eq 'Up' | Select-Object -First 1).LinkSpeed")
	if speed == "" {
		return
	}
	re := regexp.MustCompile(`([\d.]+)\s*(Gbps|Mbps|Kbps)`)
	mm := re.FindStringSubmatch(speed)
	if mm == nil {
		return
	}
	val, err := strconv.ParseFloat(mm[1], 64)
	if err != nil {
		return
	}
	switch mm[2] {
	case "Gbps":
		n.LinkMbps = int(val * 1000)
	case "Mbps":
		n.LinkMbps = int(val)
	case "Kbps":
		n.LinkMbps = int(val / 1000)
	}
}
