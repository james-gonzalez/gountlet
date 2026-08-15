//go:build linux

package sysinfo

import (
	"bufio"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func fillCPU(c *CPU) {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return
	}
	defer f.Close()

	// A physical core is identified by the (physical id, core id) pair —
	// physical id alone is just the socket number, so on any single-socket
	// machine every logical CPU shares the same one.
	cores := map[string]bool{}
	var physID, coreID string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, val, ok := splitColon(line)
		if !ok {
			continue
		}
		switch key {
		case "model name":
			if c.Model == "" {
				c.Model = val
			}
		case "physical id":
			physID = val
		case "core id":
			coreID = val
			cores[physID+":"+coreID] = true
		}
	}
	c.PhysicalCores = len(cores)
	if c.PhysicalCores == 0 {
		c.PhysicalCores = c.LogicalCores
	}
}

func fillMemory(m *Memory) {
	f, err := os.Open("/proc/meminfo")
	if err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			key, val, ok := splitColon(scanner.Text())
			if !ok || key != "MemTotal" {
				continue
			}
			fields := strings.Fields(val)
			if len(fields) > 0 {
				if kb, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
					m.TotalBytes = kb * 1024
				}
			}
			break
		}
	}

	// Memory type/speed needs SMBIOS data, which typically requires root
	// via dmidecode; best-effort, silently left blank without it.
	out, err := exec.Command("dmidecode", "-t", "memory").Output()
	if err != nil {
		return
	}
	typeRe := regexp.MustCompile(`(?m)^\s*Type:\s*(DDR\w*)\s*$`)
	speedRe := regexp.MustCompile(`(?m)^\s*Speed:\s*(\d+)\s*MT/s\s*$`)
	if mm := typeRe.FindStringSubmatch(string(out)); mm != nil {
		m.Type = mm[1]
	}
	if mm := speedRe.FindStringSubmatch(string(out)); mm != nil {
		if v, err := strconv.Atoi(mm[1]); err == nil {
			m.SpeedMHz = v
		}
	}
}

func fillDisk(d *Disk, path string) {
	out, err := exec.Command("df", "-PT", path).Output()
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 6 {
		return
	}
	d.Device = fields[0]
	d.Filesystem = fields[1]
	if kb, err := strconv.ParseUint(fields[2], 10, 64); err == nil {
		d.TotalBytes = kb * 1024
	}
	if kb, err := strconv.ParseUint(fields[4], 10, 64); err == nil {
		d.FreeBytes = kb * 1024
	}

	base := baseBlockDevice(d.Device)
	if base == "" {
		return
	}
	if out, err := exec.Command("lsblk", "-no", "MODEL", base).Output(); err == nil {
		if model := strings.TrimSpace(string(out)); model != "" {
			d.Model = model
		}
	}
}

// baseBlockDevice strips a partition suffix from a device path, e.g.
// /dev/nvme0n1p2 -> /dev/nvme0n1, /dev/sda1 -> /dev/sda.
func baseBlockDevice(device string) string {
	if !strings.HasPrefix(device, "/dev/") {
		return ""
	}
	re := regexp.MustCompile(`^(/dev/(?:nvme\d+n\d+|[a-z]+))(?:p?\d+)?$`)
	if mm := re.FindStringSubmatch(device); mm != nil {
		return mm[1]
	}
	return device
}

func fillNetLinkSpeed(n *NetInterface) {
	if n.Name == "" {
		return
	}
	data, err := os.ReadFile("/sys/class/net/" + n.Name + "/speed")
	if err != nil {
		return
	}
	if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && v > 0 {
		n.LinkMbps = v
	}
}

func splitColon(line string) (key, val string, ok bool) {
	i := strings.Index(line, ":")
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}
